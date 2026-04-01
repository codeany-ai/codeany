package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codeany-ai/codeany/internal/config"
	"github.com/codeany-ai/codeany/internal/memory"
	"github.com/codeany-ai/codeany/internal/session"
	"github.com/codeany-ai/codeany/internal/skills"
	"github.com/codeany-ai/codeany/internal/slash"
	"github.com/codeany-ai/codeany/internal/theme"
	"github.com/codeany-ai/open-agent-sdk-go/agent"
	"github.com/codeany-ai/open-agent-sdk-go/types"
)

// UI states
type state int

const (
	stateInit       state = iota
	stateInput
	stateQuerying
	statePermission
)

// Tea messages
type agentEventMsg struct{ event types.SDKMessage }
type agentDoneMsg struct{ err error }
type agentInitDoneMsg struct{ err error }
type tickMsg time.Time
type permissionRequestMsg struct {
	toolName string
	input    map[string]interface{}
	respCh   chan *types.PermissionDecision
}
type shellResultMsg struct {
	output string
	err    error
}

// ToolState tracks a single tool call's lifecycle
type ToolState struct {
	ID        string
	Name      string
	Input     map[string]interface{}
	InputStr  string
	Status    string // "running", "done", "error"
	Output    string
	Error     string
	StartTime time.Time
	EndTime   time.Time
	Expanded  bool
}

// Model is the main Bubble Tea model
type Model struct {
	cfg           *config.Config
	agent         *agent.Agent
	program       *tea.Program
	ctx           context.Context
	cancel        context.CancelFunc
	initialPrompt string
	resumeSession bool

	// UI state
	state    state
	width    int
	height   int
	viewport viewport.Model
	spinner  spinner.Model
	input    *InputModel
	ready    bool

	// Scrolling
	userScrolled bool // true when user manually scrolled up

	// Conversation display
	blocks []DisplayBlock

	// Active query tracking
	queryActive    bool
	queryStartTime time.Time
	streamingText  strings.Builder
	activeTools    []*ToolState
	thinkingText   string
	thinkingStart  time.Time

	// Stats
	currentCost    float64
	totalTokensIn  int
	totalTokensOut int
	sessionStart   time.Time

	// Permissions - persisted to disk
	permRules *config.PermissionRules
	mu        sync.Mutex

	// Pending permission
	pendingPerm *permissionRequestMsg

	// Session
	session *session.Session

	// Slash command handler
	slashHandler *slash.Handler

	// Slash autocomplete
	slashMatches []slash.CommandDef
	showSlash    bool

	// Status
	errMsg string

	// Toggle states
	transcriptMode bool
	planMode       bool   // no-execution mode
	spinnerVerb    string // current fun spinner verb
}

// DisplayBlock represents one visual block in the conversation
type DisplayBlock struct {
	Type      string // "user", "assistant", "tool", "system"
	Content   string
	Tools     []*ToolState
	Thinking  string
	Timestamp time.Time
}

func NewModel(cfg *config.Config, initialPrompt string, resume bool) *Model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(theme.Primary)

	ctx, cancel := context.WithCancel(context.Background())

	m := &Model{
		cfg:           cfg,
		ctx:           ctx,
		cancel:        cancel,
		initialPrompt: initialPrompt,
		resumeSession: resume,
		state:         stateInit,
		spinner:       s,
		input:         NewInputModel(),
		blocks:        make([]DisplayBlock, 0),
		activeTools:   make([]*ToolState, 0),
		permRules:     config.LoadPermissionRules(),
		sessionStart:  time.Now(),
	}

	m.slashHandler = slash.NewHandler(m)
	return m
}

func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.initAgent(),
		tea.WindowSize(),
		m.tickCmd(),
	)
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) initAgent() tea.Cmd {
	return func() tea.Msg {
		cwd, _ := os.Getwd()

		// Build extra context from skills and memory
		extraContext := buildExtraContext(cwd)
		appendPrompt := m.cfg.AppendSystemPrompt
		if extraContext != "" {
			if appendPrompt != "" {
				appendPrompt += "\n\n"
			}
			appendPrompt += extraContext
		}

		opts := agent.Options{
			Model:              m.cfg.Model,
			APIKey:             m.cfg.APIKey,
			BaseURL:            m.cfg.BaseURL,
			CWD:                cwd,
			MaxTurns:           m.cfg.MaxTurns,
			MaxBudgetUSD:       m.cfg.MaxBudgetUSD,
			MCPServers:         m.cfg.MCPServers,
			SystemPrompt:       m.cfg.SystemPrompt,
			AppendSystemPrompt: appendPrompt,
			CustomHeaders:      m.cfg.CustomHeaders,
			ProxyURL:           m.cfg.ProxyURL,
			AllowedTools:       m.cfg.AllowedTools,
		}

		pm := m.cfg.GetPermissionMode()
		if pm == types.PermissionModeBypassPermissions {
			opts.PermissionMode = types.PermissionModeBypassPermissions
		} else {
			opts.CanUseTool = m.createPermissionCallback()
		}

		a := agent.New(opts)
		m.agent = a

		if err := a.Init(m.ctx); err != nil {
			return agentInitDoneMsg{err: err}
		}

		if m.resumeSession {
			m.session = session.Resume(config.SessionPath(), cwd)
		} else {
			m.session = session.New(config.SessionPath(), cwd)
		}

		return agentInitDoneMsg{err: nil}
	}
}

func (m *Model) createPermissionCallback() types.CanUseToolFn {
	return func(tool types.Tool, input map[string]interface{}) (*types.PermissionDecision, error) {
		// Check persisted rules first
		if m.permRules.IsAllowed(tool.Name()) {
			return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
		}

		// Read-only auto-approve in acceptEdits mode
		if m.cfg.GetPermissionMode() == types.PermissionModeAcceptEdits && tool.IsReadOnly(input) {
			return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
		}

		// Ask user via TUI
		respCh := make(chan *types.PermissionDecision, 1)
		if m.program != nil {
			m.program.Send(permissionRequestMsg{
				toolName: tool.Name(),
				input:    input,
				respCh:   respCh,
			})
		} else {
			return &types.PermissionDecision{Behavior: types.PermissionAllow}, nil
		}

		select {
		case decision := <-respCh:
			return decision, nil
		case <-m.ctx.Done():
			return &types.PermissionDecision{Behavior: types.PermissionDeny, Reason: "Cancelled"}, nil
		}
	}
}

// ─── Update ───────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		// Forward mouse events to viewport for wheel scrolling
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// If user scrolled up, stop auto-scroll-to-bottom
		if msg.Button == tea.MouseButtonWheelUp {
			m.userScrolled = true
		}
		if msg.Button == tea.MouseButtonWheelDown {
			if m.viewport.AtBottom() {
				m.userScrolled = false
			}
		}
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.ready = true
			m.viewport = viewport.New(m.width, m.contentHeight())
			m.viewport.MouseWheelEnabled = true
			m.viewport.SetContent(m.renderContent())
			m.input.SetWidth(m.width)
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = m.contentHeight()
			m.input.SetWidth(m.width)
		}
		m.refreshViewport()
		if m.state == stateInput && m.initialPrompt != "" {
			prompt := m.initialPrompt
			m.initialPrompt = ""
			return m, m.sendQuery(prompt)
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
		if m.queryActive {
			m.refreshViewport()
		}

	case tickMsg:
		if m.queryActive {
			m.refreshViewport()
		}
		cmds = append(cmds, m.tickCmd())

	case agentInitDoneMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Failed to initialize: %v", msg.err)
		}
		m.state = stateInput
		m.input.Focus()
		m.refreshViewport()
		if m.initialPrompt != "" && m.ready {
			prompt := m.initialPrompt
			m.initialPrompt = ""
			return m, m.sendQuery(prompt)
		}
		return m, nil

	case agentEventMsg:
		m.handleAgentEvent(msg.event)
		m.refreshViewport()
		return m, nil

	case agentDoneMsg:
		m.queryActive = false
		m.state = stateInput
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		}
		if m.agent != nil {
			m.syncFinalState()
		}
		// Update session metadata
		if m.session != nil {
			msgs := 0
			if m.agent != nil {
				msgs = len(m.agent.GetMessages())
			}
			m.session.UpdateMeta(m.cfg.Model, msgs, m.currentCost, "")
		}
		m.streamingText.Reset()
		m.activeTools = nil
		m.thinkingText = ""
		m.refreshViewport()
		m.input.Focus()
		// Ring terminal bell to notify user
		fmt.Print("\a")
		return m, nil

	case permissionRequestMsg:
		m.pendingPerm = &msg
		m.state = statePermission
		m.refreshViewport()
		return m, nil

	case shellResultMsg:
		output := strings.TrimRight(msg.output, "\n")
		if msg.err != nil {
			output += "\n" + msg.err.Error()
		}
		m.blocks = append(m.blocks, DisplayBlock{
			Type: "system", Content: output, Timestamp: time.Now(),
		})
		m.refreshViewport()
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

// ─── Key handling ────────────────────────────────

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.state == stateQuerying || m.state == statePermission {
			m.cancel()
			m.queryActive = false
			m.state = stateInput
			m.pendingPerm = nil
			m.activeTools = nil
			m.ctx, m.cancel = context.WithCancel(context.Background())
			m.input.Focus()
			m.refreshViewport()
			return m, nil
		}
		return m, tea.Quit

	case "ctrl+d":
		if m.state == stateInput && m.input.Value() == "" {
			return m, tea.Quit
		}

	case "ctrl+l":
		m.blocks = nil
		if m.agent != nil {
			m.agent.Clear()
		}
		m.refreshViewport()
		return m, nil

	case "ctrl+o":
		m.transcriptMode = !m.transcriptMode
		m.refreshViewport()
		return m, nil
	}

	switch m.state {
	case stateInput:
		return m.handleInputKey(msg)
	case statePermission:
		return m.handlePermissionKey(msg)
	case stateQuerying:
		// Allow scrolling while querying
		return m.handleScrollKey(msg)
	}

	return m, nil
}

func (m *Model) handleScrollKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.viewport.LineUp(1)
		m.userScrolled = true
		return m, nil
	case "down", "j":
		m.viewport.LineDown(1)
		if m.viewport.AtBottom() {
			m.userScrolled = false
		}
		return m, nil
	case "pgup", "ctrl+b":
		m.viewport.HalfViewUp()
		m.userScrolled = true
		return m, nil
	case "pgdown", "ctrl+f":
		m.viewport.HalfViewDown()
		if m.viewport.AtBottom() {
			m.userScrolled = false
		}
		return m, nil
	case "home", "g":
		m.viewport.GotoTop()
		m.userScrolled = true
		return m, nil
	case "end", "G":
		m.viewport.GotoBottom()
		m.userScrolled = false
		return m, nil
	}
	return m, nil
}

func (m *Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Scrolling keys work even in input mode
	switch msg.String() {
	case "pgup":
		m.viewport.HalfViewUp()
		m.userScrolled = true
		return m, nil
	case "pgdown":
		m.viewport.HalfViewDown()
		if m.viewport.AtBottom() {
			m.userScrolled = false
		}
		return m, nil
	case "tab":
		// Tab completion for slash commands
		if m.showSlash && len(m.slashMatches) > 0 {
			// Complete with first match
			m.input.ta.Reset()
			m.input.ta.InsertString(m.slashMatches[0].Name + " ")
			m.showSlash = false
			m.slashMatches = nil
			return m, nil
		}
	case "esc":
		if m.showSlash {
			m.showSlash = false
			m.slashMatches = nil
			return m, nil
		}
		m.input.Reset()
		return m, nil
	}

	// Check for enter (submit)
	submitted, cmd := m.input.Update(msg)
	if submitted {
		text := m.input.Value()
		m.input.Reset()
		m.showSlash = false
		m.slashMatches = nil

		if text == "" {
			return m, nil
		}

		// Shell escape
		if strings.HasPrefix(text, "! ") {
			shellCmd := strings.TrimPrefix(text, "! ")
			m.blocks = append(m.blocks, DisplayBlock{
				Type: "system", Content: "$ " + shellCmd, Timestamp: time.Now(),
			})
			m.refreshViewport()
			return m, func() tea.Msg {
				out, err := exec.Command("bash", "-c", shellCmd).CombinedOutput()
				return shellResultMsg{output: string(out), err: err}
			}
		}

		// Slash commands
		if strings.HasPrefix(text, "/") {
			result := m.slashHandler.Handle(text)
			if result.Quit {
				return m, tea.Quit
			}
			if result.ClearMessages {
				m.blocks = nil
				if m.agent != nil {
					m.agent.Clear()
				}
			}
			if result.Message != "" {
				m.blocks = append(m.blocks, DisplayBlock{
					Type: "system", Content: result.Message, Timestamp: time.Now(),
				})
			}
			if result.PlanToggle {
				m.planMode = !m.planMode
				modeStr := "OFF"
				if m.planMode {
					modeStr = "ON"
				}
				m.blocks = append(m.blocks, DisplayBlock{
					Type: "system", Content: fmt.Sprintf("Plan mode: %s", modeStr), Timestamp: time.Now(),
				})
			}
			if result.SessionTitle != "" && m.session != nil {
				m.session.UpdateMeta(m.cfg.Model, 0, m.currentCost, result.SessionTitle)
			}
			if result.VimToggle {
				// Vim mode is tracked as a visual indicator
				m.blocks = append(m.blocks, DisplayBlock{
					Type: "system", Content: "Vim mode toggled (visual indicator only in this version)", Timestamp: time.Now(),
				})
			}
			m.refreshViewport()
			// If the command produced a prompt for the agent, send it
			if result.SkillPrompt != "" {
				return m, m.sendQuery(result.SkillPrompt)
			}
			return m, nil
		}

		return m, m.sendQuery(text)
	}

	// Update slash autocomplete state based on current input
	m.updateSlashAutocomplete()

	return m, cmd
}

func (m *Model) updateSlashAutocomplete() {
	val := m.input.Value()
	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") && len(val) >= 1 {
		m.slashMatches = slash.MatchCommands(val)
		m.showSlash = len(m.slashMatches) > 0
	} else {
		m.showSlash = false
		m.slashMatches = nil
	}
}

func (m *Model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingPerm == nil {
		return m, nil
	}

	switch msg.String() {
	case "y", "Y", "enter":
		m.pendingPerm.respCh <- &types.PermissionDecision{Behavior: types.PermissionAllow}
		m.pendingPerm = nil
		m.state = stateQuerying
		m.refreshViewport()
	case "n", "N":
		m.pendingPerm.respCh <- &types.PermissionDecision{Behavior: types.PermissionDeny, Reason: "User denied"}
		m.pendingPerm = nil
		m.state = stateQuerying
		m.refreshViewport()
	case "a", "A":
		// Persist "always allow" to disk
		m.permRules.SetAlwaysAllow(m.pendingPerm.toolName)
		m.pendingPerm.respCh <- &types.PermissionDecision{Behavior: types.PermissionAllow}
		m.pendingPerm = nil
		m.state = stateQuerying
		m.refreshViewport()
	}
	return m, nil
}

// ─── Query ──────────────────────────────────────

func (m *Model) sendQuery(prompt string) tea.Cmd {
	m.state = stateQuerying
	m.errMsg = ""
	m.queryActive = true
	m.queryStartTime = time.Now()
	m.activeTools = nil
	m.thinkingText = ""
	m.thinkingStart = time.Time{}
	m.streamingText.Reset()
	m.userScrolled = false
	m.spinnerVerb = theme.RandomVerb()
	m.input.Blur()

	m.blocks = append(m.blocks, DisplayBlock{
		Type: "user", Content: prompt, Timestamp: time.Now(),
	})
	m.refreshViewport()

	return func() tea.Msg {
		if m.agent == nil {
			return agentDoneMsg{err: fmt.Errorf("agent not initialized")}
		}

		events, errCh := m.agent.Query(m.ctx, prompt)
		for event := range events {
			if m.program != nil {
				m.program.Send(agentEventMsg{event: event})
			}
		}
		return agentDoneMsg{err: <-errCh}
	}
}

func (m *Model) handleAgentEvent(event types.SDKMessage) {
	switch event.Type {
	case types.MessageTypeAssistant:
		if event.Message != nil {
			m.processAssistantMessage(event.Message)
		}
	case types.MessageTypeProgress:
		if event.Text != "" {
			m.streamingText.Reset()
			m.streamingText.WriteString(event.Text)
		}
	case types.MessageTypeResult:
		m.currentCost = event.Cost
		if event.Usage != nil {
			m.totalTokensIn = event.Usage.InputTokens
			m.totalTokensOut = event.Usage.OutputTokens
		}
	}
}

func (m *Model) processAssistantMessage(msg *types.Message) {
	var textParts []string
	var tools []*ToolState
	var thinking string

	for _, block := range msg.Content {
		switch block.Type {
		case types.ContentBlockText:
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case types.ContentBlockToolUse:
			ts := &ToolState{
				ID:        block.ID,
				Name:      block.Name,
				Input:     block.Input,
				InputStr:  formatToolInput(block.Name, block.Input),
				Status:    "running",
				StartTime: time.Now(),
			}
			tools = append(tools, ts)
			m.activeTools = append(m.activeTools, ts)
		case types.ContentBlockToolResult:
			resultText := ""
			for _, cb := range block.Content {
				if cb.Text != "" {
					resultText += cb.Text
				}
			}
			for _, t := range m.activeTools {
				if t.ID == block.ToolUseID || (t.Status == "running") {
					if block.IsError {
						t.Error = resultText
						t.Status = "error"
					} else {
						t.Output = resultText
						t.Status = "done"
					}
					t.EndTime = time.Now()
					break
				}
			}
		case types.ContentBlockThinking:
			thinking = block.Thinking
			if m.thinkingStart.IsZero() {
				m.thinkingStart = time.Now()
			}
		}
	}

	if thinking != "" {
		m.thinkingText = thinking
	}

	if len(tools) > 0 {
		m.blocks = append(m.blocks, DisplayBlock{
			Type: "tool", Tools: tools, Timestamp: time.Now(),
		})
	}

	content := strings.Join(textParts, "\n")
	if content != "" {
		m.blocks = append(m.blocks, DisplayBlock{
			Type: "assistant", Content: content, Timestamp: time.Now(),
		})
	}
}

func (m *Model) syncFinalState() {
	if m.agent.CostTracker() != nil {
		m.currentCost = m.agent.CostTracker().TotalCost()
		in, out := m.agent.CostTracker().TotalTokens()
		m.totalTokensIn = in
		m.totalTokensOut = out
	}
}

// ─── Viewport ────────────────────────────────────

func (m *Model) refreshViewport() {
	if !m.ready {
		return
	}
	content := m.renderContent()
	m.viewport.SetContent(content)
	// Only auto-scroll if user hasn't manually scrolled up
	if !m.userScrolled {
		m.viewport.GotoBottom()
	}
}

func (m *Model) contentHeight() int {
	h := m.height - 6
	if h < 5 {
		h = 5
	}
	return h
}

// ─── View ───────────────────────────────────────

func (m *Model) View() string {
	if !m.ready {
		return fmt.Sprintf("\n  %s Initializing codeany...\n", m.spinner.View())
	}

	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Viewport
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Bottom area
	switch m.state {
	case statePermission:
		b.WriteString(m.renderPermissionPrompt())
	case stateQuerying:
		b.WriteString(m.renderActivityLine())
	case stateInit:
		b.WriteString(fmt.Sprintf("  %s Initializing...", m.spinner.View()))
	default:
		// Slash autocomplete dropdown (above input)
		if m.showSlash && len(m.slashMatches) > 0 {
			b.WriteString(m.renderSlashSuggestions())
		}
		b.WriteString(m.renderInputArea())
	}

	// Status bar
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())

	return b.String()
}

func (m *Model) renderHeader() string {
	left := theme.PrimaryText.Render(" ● codeany")
	model := theme.MutedStyle.Render(fmt.Sprintf(" %s", m.cfg.Model))

	// Right side: session time
	dur := time.Since(m.sessionStart)
	right := theme.DimText.Render(fmt.Sprintf("%s ", formatDuration(dur)))

	// Fill middle with spaces
	leftLen := lipgloss.Width(left + model)
	rightLen := lipgloss.Width(right)
	gap := m.width - leftLen - rightLen
	if gap < 1 {
		gap = 1
	}

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1b26")).
		Width(m.width)

	return headerStyle.Render(left + model + strings.Repeat(" ", gap) + right)
}

func (m *Model) renderActivityLine() string {
	elapsed := time.Since(m.queryStartTime)

	var parts []string
	parts = append(parts, formatDuration(elapsed))

	if m.totalTokensOut > 0 {
		parts = append(parts, fmt.Sprintf("↓ %d tokens", m.totalTokensOut))
	}

	if !m.thinkingStart.IsZero() && m.queryActive {
		thinkDur := time.Since(m.thinkingStart)
		parts = append(parts, fmt.Sprintf("thought for %s", formatDuration(thinkDur)))
	}

	info := strings.Join(parts, " · ")
	spinnerStr := m.spinner.View()
	label := m.spinnerVerb + "..."
	if label == "..." {
		label = "Thinking..."
	}

	for _, t := range m.activeTools {
		if t.Status == "running" {
			label = fmt.Sprintf("%s %s...", theme.ToolVerb(t.Name), t.Name)
			break
		}
	}

	return fmt.Sprintf("  %s %s (%s)",
		spinnerStr,
		theme.PrimaryText.Render(label),
		theme.MutedStyle.Render(info),
	)
}

func (m *Model) renderInputArea() string {
	if m.errMsg != "" {
		errLine := theme.ErrorStyle.Render("  ✗ " + m.errMsg)
		return errLine + "\n" + m.input.View()
	}
	return m.input.View()
}

func (m *Model) renderSlashSuggestions() string {
	var b strings.Builder
	maxShow := 8
	if len(m.slashMatches) < maxShow {
		maxShow = len(m.slashMatches)
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Dim).
		Padding(0, 1).
		MarginLeft(2)

	var inner strings.Builder
	for i := 0; i < maxShow; i++ {
		cmd := m.slashMatches[i]
		name := lipgloss.NewStyle().Bold(true).Foreground(theme.Secondary).Render(cmd.Name)
		desc := theme.MutedStyle.Render("  " + cmd.Description)
		inner.WriteString(name + desc)
		if i < maxShow-1 {
			inner.WriteString("\n")
		}
	}
	if len(m.slashMatches) > maxShow {
		inner.WriteString(fmt.Sprintf("\n%s", theme.DimText.Render(fmt.Sprintf("  +%d more", len(m.slashMatches)-maxShow))))
	}

	b.WriteString(boxStyle.Render(inner.String()))
	b.WriteString("\n")
	return b.String()
}

func (m *Model) renderStatusBar() string {
	var leftParts []string

	// Plan mode indicator
	if m.planMode {
		leftParts = append(leftParts, theme.SecondaryText.Render("📋 PLAN"))
	}

	// Permission mode indicator
	mode := m.cfg.PermissionMode
	if mode == "" {
		mode = "default"
	}
	switch mode {
	case "bypassPermissions":
		leftParts = append(leftParts, theme.WarningText.Render("⚡ bypass"))
	case "acceptEdits":
		leftParts = append(leftParts, theme.SuccessText.Render("✎ auto"))
	case "plan":
		leftParts = append(leftParts, theme.SecondaryText.Render("📋 plan"))
	default:
		leftParts = append(leftParts, theme.MutedStyle.Render("🔒"))
	}

	// Cost
	if m.currentCost > 0 {
		leftParts = append(leftParts, theme.MutedStyle.Render(fmt.Sprintf("$%.4f", m.currentCost)))
	}

	// Tokens
	if m.totalTokensIn > 0 || m.totalTokensOut > 0 {
		leftParts = append(leftParts, theme.MutedStyle.Render(fmt.Sprintf("↑%d ↓%d", m.totalTokensIn, m.totalTokensOut)))
	}

	// MCP connections
	if m.agent != nil {
		client := m.agent.MCPClient()
		if client != nil {
			conns := client.AllConnections()
			if len(conns) > 0 {
				connected := 0
				for _, c := range conns {
					if c.Status == "connected" {
						connected++
					}
				}
				leftParts = append(leftParts, theme.MutedStyle.Render(fmt.Sprintf("MCP %d/%d", connected, len(conns))))
			}
		}
	}

	left := strings.Join(leftParts, theme.DimText.Render(" │ "))

	// Right side: scroll position
	right := ""
	if m.ready {
		pct := m.viewport.ScrollPercent()
		if pct < 1.0 {
			right = theme.DimText.Render(fmt.Sprintf(" %d%% ", int(pct*100)))
		} else {
			right = theme.DimText.Render(" end ")
		}
	}

	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1b26")).
		Width(m.width).
		PaddingLeft(1)

	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	gap := m.width - leftLen - rightLen - 1
	if gap < 1 {
		gap = 1
	}

	return barStyle.Render(left + strings.Repeat(" ", gap) + right)
}

func (m *Model) renderPermissionPrompt() string {
	if m.pendingPerm == nil {
		return ""
	}

	var b strings.Builder
	toolStyle := theme.ToolName.Render(m.pendingPerm.toolName)
	b.WriteString(fmt.Sprintf("  %s wants to execute:\n", toolStyle))

	input := formatToolInput(m.pendingPerm.toolName, m.pendingPerm.input)
	if len(input) > 400 {
		input = input[:400] + "..."
	}
	b.WriteString(fmt.Sprintf("  %s\n\n", theme.DimText.Render(input)))

	allow := lipgloss.NewStyle().Foreground(theme.Success).Bold(true).Render("[Y]es")
	deny := lipgloss.NewStyle().Foreground(theme.Error).Bold(true).Render("[N]o")
	always := lipgloss.NewStyle().Foreground(theme.Warning).Bold(true).Render("[A]lways allow")
	b.WriteString(fmt.Sprintf("  %s  %s  %s", allow, deny, always))

	return theme.PermissionBox.Render(b.String())
}

func (m *Model) renderContent() string {
	if len(m.blocks) == 0 && !m.queryActive {
		return m.renderWelcome()
	}

	var b strings.Builder
	for i, block := range m.blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderBlock(block))
	}

	// Show active streaming/tool state
	if m.queryActive {
		if m.thinkingText != "" {
			thinkStyle := lipgloss.NewStyle().Foreground(theme.Dim).Italic(true)
			text := m.thinkingText
			if len(text) > 300 {
				text = text[:300] + "..."
			}
			b.WriteString("\n  " + thinkStyle.Render("💭 "+text) + "\n")
		}

		for _, t := range m.activeTools {
			if t.Status == "running" {
				b.WriteString(m.renderToolState(t, true))
			}
		}

		if m.streamingText.Len() > 0 {
			b.WriteString("\n")
			rendered := renderMarkdown(m.streamingText.String(), m.width-4)
			for _, line := range strings.Split(rendered, "\n") {
				b.WriteString("  " + line + "\n")
			}
		}
	}

	return b.String()
}

func (m *Model) renderBlock(block DisplayBlock) string {
	switch block.Type {
	case "user":
		return renderUserBlock(block, m.width)
	case "assistant":
		return renderAssistantBlock(block, m.width)
	case "tool":
		return m.renderToolBlock(block)
	case "system":
		return renderSystemBlock(block)
	default:
		return ""
	}
}

func (m *Model) renderToolBlock(block DisplayBlock) string {
	var b strings.Builder
	for _, tool := range block.Tools {
		b.WriteString(m.renderToolState(tool, false))
	}
	return b.String()
}

func (m *Model) renderToolState(tool *ToolState, isActive bool) string {
	var b strings.Builder
	w := m.width

	// Status indicator
	var dot string
	var nameStyle lipgloss.Style
	switch tool.Status {
	case "running":
		dot = theme.PrimaryText.Render("⏺")
		nameStyle = lipgloss.NewStyle().Bold(true)
	case "done":
		dot = theme.SuccessText.Render("✓")
		nameStyle = lipgloss.NewStyle().Bold(true)
	case "error":
		dot = theme.ErrorStyle.Render("✗")
		nameStyle = theme.ErrorStyle.Bold(true)
	default:
		dot = theme.MutedStyle.Render("⏺")
		nameStyle = lipgloss.NewStyle().Bold(true)
	}

	// Header line: ⏺ ToolName(args)
	name := nameStyle.Render(tool.Name)
	args := ""
	if tool.InputStr != "" {
		args = theme.MutedStyle.Render("(" + truncate(tool.InputStr, 80) + ")")
	}
	b.WriteString(fmt.Sprintf("  %s %s%s\n", dot, name, args))

	// Running indicator
	if tool.Status == "running" && isActive {
		b.WriteString("    " + theme.MutedStyle.Render("Running...") + "\n")
	}

	// Output section — tool-specific rendering
	if tool.Output != "" {
		expanded := m.transcriptMode || tool.Expanded
		b.WriteString(m.renderToolOutput(tool, expanded, w))
	}

	// Error
	if tool.Error != "" {
		errLines := strings.Split(tool.Error, "\n")
		for i, line := range errLines {
			if i >= 5 {
				b.WriteString("    " + theme.ErrorStyle.Render(fmt.Sprintf("... +%d more error lines", len(errLines)-5)) + "\n")
				break
			}
			b.WriteString("    " + theme.ErrorStyle.Render(truncate(line, w-8)) + "\n")
		}
	}

	return b.String()
}

func (m *Model) renderToolOutput(tool *ToolState, expanded bool, w int) string {
	var b strings.Builder
	output := tool.Output
	lines := strings.Split(output, "\n")
	totalLines := len(lines)

	// How many lines to show when collapsed
	collapsedMax := 3
	expandedMax := 30

	switch tool.Name {
	case "Read":
		// For Read tool: show line count summary
		if !expanded {
			b.WriteString(fmt.Sprintf("    ⎿ %d lines\n", totalLines))
		} else {
			maxShow := expandedMax
			if totalLines < maxShow {
				maxShow = totalLines
			}
			for i := 0; i < maxShow; i++ {
				b.WriteString("    " + theme.DimText.Render(truncate(lines[i], w-8)) + "\n")
			}
			if totalLines > maxShow {
				b.WriteString("    " + theme.DimText.Render(fmt.Sprintf("... +%d lines", totalLines-maxShow)) + "\n")
			}
		}

	case "Bash":
		// For Bash: show output, stderr-aware
		maxShow := collapsedMax
		if expanded {
			maxShow = expandedMax
		}
		if totalLines <= maxShow {
			for _, line := range lines {
				b.WriteString("    " + theme.DimText.Render(truncate(line, w-8)) + "\n")
			}
		} else {
			for i := 0; i < maxShow; i++ {
				b.WriteString("    " + theme.DimText.Render(truncate(lines[i], w-8)) + "\n")
			}
			b.WriteString("    " + theme.DimText.Render(fmt.Sprintf("... +%d lines (ctrl+o to expand)", totalLines-maxShow)) + "\n")
		}

	case "Edit":
		// For Edit: show what was changed
		if !expanded {
			b.WriteString("    ⎿ " + theme.SuccessText.Render("file updated") + "\n")
		} else {
			maxShow := expandedMax
			if totalLines < maxShow {
				maxShow = totalLines
			}
			for i := 0; i < maxShow; i++ {
				line := lines[i]
				if strings.HasPrefix(line, "+") {
					b.WriteString("    " + theme.SuccessText.Render(truncate(line, w-8)) + "\n")
				} else if strings.HasPrefix(line, "-") {
					b.WriteString("    " + theme.ErrorStyle.Render(truncate(line, w-8)) + "\n")
				} else {
					b.WriteString("    " + theme.DimText.Render(truncate(line, w-8)) + "\n")
				}
			}
			if totalLines > maxShow {
				b.WriteString("    " + theme.DimText.Render(fmt.Sprintf("... +%d lines", totalLines-maxShow)) + "\n")
			}
		}

	case "Write":
		// For Write: show file created/written
		if !expanded {
			b.WriteString("    ⎿ " + theme.SuccessText.Render("file written") + "\n")
		} else {
			maxShow := expandedMax
			if totalLines < maxShow {
				maxShow = totalLines
			}
			for i := 0; i < maxShow; i++ {
				b.WriteString("    " + theme.DimText.Render(truncate(lines[i], w-8)) + "\n")
			}
			if totalLines > maxShow {
				b.WriteString("    " + theme.DimText.Render(fmt.Sprintf("... +%d lines", totalLines-maxShow)) + "\n")
			}
		}

	case "Glob":
		// Show file list
		maxShow := 8
		if expanded {
			maxShow = 30
		}
		if totalLines <= maxShow {
			for _, line := range lines {
				b.WriteString("    " + theme.DimText.Render(truncate(line, w-8)) + "\n")
			}
		} else {
			for i := 0; i < maxShow; i++ {
				b.WriteString("    " + theme.DimText.Render(truncate(lines[i], w-8)) + "\n")
			}
			b.WriteString("    " + theme.DimText.Render(fmt.Sprintf("... +%d files", totalLines-maxShow)) + "\n")
		}

	case "Grep":
		// Show search results
		maxShow := 5
		if expanded {
			maxShow = 30
		}
		if totalLines <= maxShow {
			for _, line := range lines {
				b.WriteString("    " + theme.DimText.Render(truncate(line, w-8)) + "\n")
			}
		} else {
			for i := 0; i < maxShow; i++ {
				b.WriteString("    " + theme.DimText.Render(truncate(lines[i], w-8)) + "\n")
			}
			b.WriteString("    " + theme.DimText.Render(fmt.Sprintf("... +%d matches", totalLines-maxShow)) + "\n")
		}

	default:
		// Generic: show first few lines
		maxShow := collapsedMax
		if expanded {
			maxShow = expandedMax
		}
		if totalLines <= maxShow {
			for _, line := range lines {
				if line != "" {
					b.WriteString("    ⎿ " + theme.DimText.Render(truncate(line, w-10)) + "\n")
				}
			}
		} else {
			firstLine := lines[0]
			b.WriteString("    ⎿ " + theme.DimText.Render(truncate(firstLine, w-10)) + "\n")
			if totalLines > 1 {
				b.WriteString("    " + theme.DimText.Render(fmt.Sprintf("  ... +%d lines (ctrl+o to expand)", totalLines-1)) + "\n")
			}
		}
	}

	return b.String()
}

func (m *Model) renderWelcome() string {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}

	p := theme.PrimaryText
	d := theme.DimText
	mt := theme.MutedStyle

	var b strings.Builder
	b.WriteString("\n")

	// ASCII art logo
	b.WriteString(p.Render("     ██████╗ ██████╗ ██████╗ ███████╗ █████╗ ███╗   ██╗██╗   ██╗") + "\n")
	b.WriteString(p.Render("    ██╔════╝██╔═══██╗██╔══██╗██╔════╝██╔══██╗████╗  ██║╚██╗ ██╔╝") + "\n")
	b.WriteString(p.Render("    ██║     ██║   ██║██║  ██║█████╗  ███████║██╔██╗ ██║ ╚████╔╝ ") + "\n")
	b.WriteString(p.Render("    ██║     ██║   ██║██║  ██║██╔══╝  ██╔══██║██║╚██╗██║  ╚██╔╝  ") + "\n")
	b.WriteString(p.Render("    ╚██████╗╚██████╔╝██████╔╝███████╗██║  ██║██║ ╚████║   ██║   ") + "\n")
	b.WriteString(p.Render("     ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝╚═╝  ╚═╝╚═╝  ╚═══╝   ╚═╝   ") + "\n")
	b.WriteString("\n")
	b.WriteString(mt.Render("    AI-powered terminal agent") + "  " + d.Render("v0.1.0") + "\n")
	b.WriteString("\n")

	// Session info
	b.WriteString(mt.Render(fmt.Sprintf("    cwd    %s", cwd)) + "\n")
	b.WriteString(mt.Render(fmt.Sprintf("    model  %s", m.cfg.Model)) + "\n")
	mode := m.cfg.PermissionMode
	if mode == "" {
		mode = "default"
	}
	b.WriteString(mt.Render(fmt.Sprintf("    mode   %s", mode)) + "\n")

	// MCP servers
	if len(m.cfg.MCPServers) > 0 {
		b.WriteString(mt.Render(fmt.Sprintf("    mcp    %d server(s)", len(m.cfg.MCPServers))) + "\n")
	}

	b.WriteString("\n")

	// Tips in a subtle box
	tipBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Dim).
		Padding(0, 1).
		MarginLeft(3)

	tips := d.Render("Enter") + mt.Render(" send") +
		d.Render("  Shift+Enter") + mt.Render(" newline") +
		d.Render("  /help") + mt.Render(" commands") +
		"\n" +
		d.Render("Ctrl+C") + mt.Render(" cancel") +
		d.Render("  Ctrl+O") + mt.Render(" expand") +
		d.Render("  Ctrl+L") + mt.Render(" clear") +
		d.Render("  !cmd") + mt.Render(" shell")

	b.WriteString(tipBorder.Render(tips))
	b.WriteString("\n\n")

	return b.String()
}

// ─── slash.App interface ────────────────────────

func (m *Model) GetConfig() *config.Config { return m.cfg }
func (m *Model) GetAgent() *agent.Agent    { return m.agent }
func (m *Model) GetCost() float64          { return m.currentCost }
func (m *Model) GetTokensIn() int          { return m.totalTokensIn }
func (m *Model) GetTokensOut() int         { return m.totalTokensOut }
func (m *Model) SetModel(model string)     { m.cfg.Model = model }
func (m *Model) SetPermissionMode(mode string) {
	m.cfg.PermissionMode = mode
}
func (m *Model) SendPrompt(prompt string) {
	// Handled via SkillPrompt in handleInputKey
}

// ─── Context Builder ────────────────────────────

func buildExtraContext(cwd string) string {
	var parts []string

	// Skills context
	allSkills := skills.LoadAll()
	if skillCtx := skills.FormatForPrompt(allSkills); skillCtx != "" {
		parts = append(parts, skillCtx)
	}

	// Memory context
	memDir := config.MemoryPath()
	if memCtx := memory.FormatForPrompt(memDir); memCtx != "" {
		parts = append(parts, memCtx)
	}

	// Project-specific memory
	projMemDir := config.ProjectMemoryPath()
	if projMemCtx := memory.FormatForPrompt(projMemDir); projMemCtx != "" {
		parts = append(parts, projMemCtx)
	}

	return strings.Join(parts, "\n\n")
}

// ─── Helpers ────────────────────────────────────

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 40
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
