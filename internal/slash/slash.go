package slash

import (
	"fmt"
	"sort"
	"strings"

	"github.com/codeany-ai/codeany/internal/config"
	"github.com/codeany-ai/open-agent-sdk-go/agent"
)

// App is the interface that the TUI model implements
type App interface {
	GetConfig() *config.Config
	GetAgent() *agent.Agent
	GetCost() float64
	GetTokensIn() int
	GetTokensOut() int
	SetModel(model string)
	SetPermissionMode(mode string)
	SendPrompt(prompt string) // Send a prompt to the agent
}

// Result is returned by slash command handlers
type Result struct {
	Message       string
	Quit          bool
	ClearMessages bool
	SkillPrompt   string // If set, send this as a prompt to the agent
	PlanToggle    bool   // Toggle plan mode
}

// CommandDef defines a slash command with metadata
type CommandDef struct {
	Name        string
	Aliases     []string
	Description string
	HasArgs     bool
}

// AllCommands returns all available slash commands
func AllCommands() []CommandDef {
	return []CommandDef{
		// Core
		{Name: "/help", Aliases: []string{"/h"}, Description: "Show available commands"},
		{Name: "/quit", Aliases: []string{"/q", "/exit"}, Description: "Exit codeany"},
		{Name: "/clear", Description: "Clear conversation history"},
		{Name: "/compact", Description: "Compact conversation with optional instructions", HasArgs: true},
		{Name: "/model", Description: "Show or change model", HasArgs: true},
		{Name: "/cost", Description: "Show cost and token usage"},
		{Name: "/config", Description: "Show current configuration"},
		{Name: "/permissions", Aliases: []string{"/perm"}, Description: "Show or change permission mode", HasArgs: true},
		{Name: "/status", Description: "Show session status"},
		{Name: "/version", Description: "Show version"},
		{Name: "/memory", Description: "Show memory info"},
		// Setup & diagnostics
		{Name: "/init", Description: "Initialize project (create CODEANY.md)"},
		{Name: "/doctor", Description: "Check environment and configuration"},
		// MCP
		{Name: "/mcp", Description: "Manage MCP servers (list, tools, reconnect)", HasArgs: true},
		// Skills
		{Name: "/skills", Description: "List available skills"},
		// Git & code
		{Name: "/commit", Description: "Create a git commit", HasArgs: true},
		{Name: "/review", Description: "Review code changes", HasArgs: true},
		{Name: "/diff", Description: "Show and summarize git diff"},
		// Planning
		{Name: "/plan", Description: "Plan a task without executing", HasArgs: true},
		// Export
		{Name: "/export", Description: "Export conversation to file"},
		// Session
		{Name: "/resume", Description: "List recent sessions"},
		// Quick actions
		{Name: "/fast", Description: "Switch to faster model"},
		{Name: "/bug", Description: "Report and investigate a bug", HasArgs: true},
		{Name: "/test", Description: "Run tests", HasArgs: true},
		// Plugins & context
		{Name: "/plugin", Aliases: []string{"/plugins"}, Description: "List installed plugins"},
		{Name: "/hooks", Description: "Show configured hooks"},
		{Name: "/context", Description: "Show all context sources"},
		{Name: "/session", Description: "Show session details"},
		{Name: "/files", Description: "List files accessed this session"},
	}
}

// MatchCommands returns commands that fuzzy-match a prefix (for autocomplete)
func MatchCommands(prefix string) []CommandDef {
	prefix = strings.ToLower(prefix)
	var matches []CommandDef

	for _, cmd := range AllCommands() {
		// Match against name
		if strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
			matches = append(matches, cmd)
			continue
		}
		// Match against aliases
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(strings.ToLower(alias), prefix) {
				matches = append(matches, cmd)
				break
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})
	return matches
}

// Handler processes slash commands
type Handler struct {
	app App
}

func NewHandler(app App) *Handler {
	return &Handler{app: app}
}

func (h *Handler) Handle(input string) Result {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return Result{}
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/help", "/h":
		return h.help()
	case "/quit", "/q", "/exit":
		return Result{Quit: true}
	case "/clear":
		return Result{Message: "Conversation cleared.", ClearMessages: true}
	case "/compact":
		return h.compactCmd(args)
	case "/model":
		return h.model(args)
	case "/cost":
		return h.cost()
	case "/config":
		return h.showConfig()
	case "/permissions", "/perm":
		return h.permissions(args)
	case "/status":
		return h.status()
	case "/version":
		return Result{Message: "codeany v0.1.0"}
	case "/memory":
		return h.memory()
	// New commands
	case "/init":
		return h.init(args)
	case "/doctor":
		return h.doctor()
	case "/mcp":
		return h.mcpCmd(args)
	case "/skills":
		return h.skillsCmd(args)
	case "/commit":
		return h.commitCmd(args)
	case "/review":
		return h.reviewCmd(args)
	case "/diff":
		return h.diffCmd(args)
	case "/plan":
		return h.planCmd(args)
	case "/export":
		return h.exportCmd(args)
	case "/resume":
		return h.resumeCmd(args)
	case "/fast":
		return h.fastCmd(args)
	case "/bug":
		return h.bugCmd(args)
	case "/test":
		return h.testCmd(args)
	case "/plugin", "/plugins":
		return h.pluginCmd(args)
	case "/hooks":
		return h.hooksCmd(args)
	case "/context":
		return h.contextCmd(args)
	case "/session":
		return h.sessionCmd(args)
	case "/files":
		return h.filesCmd(args)
	default:
		// Try skill invocation
		if result, ok := h.HandleSkillInvocation(cmd, args); ok {
			return result
		}
		return Result{Message: fmt.Sprintf("Unknown command: %s. Type /help for available commands.", cmd)}
	}
}

func (h *Handler) help() Result {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range AllCommands() {
		aliases := ""
		if len(cmd.Aliases) > 0 {
			aliases = " (" + strings.Join(cmd.Aliases, ", ") + ")"
		}
		b.WriteString(fmt.Sprintf("  %-16s %s%s\n", cmd.Name, cmd.Description, aliases))
	}
	b.WriteString("\nKeyboard shortcuts:\n")
	b.WriteString("  Enter            Send message\n")
	b.WriteString("  Shift+Enter      New line\n")
	b.WriteString("  Ctrl+C           Cancel query / Exit\n")
	b.WriteString("  Ctrl+D           Exit (when input empty)\n")
	b.WriteString("  Ctrl+L           Clear conversation\n")
	b.WriteString("  Ctrl+O           Toggle expand tool output\n")
	b.WriteString("  Up/Down          Input history / Scroll\n")
	b.WriteString("  PgUp/PgDown      Scroll messages\n")
	b.WriteString("  Esc              Clear input\n")
	b.WriteString("  ! <cmd>          Run shell command\n")
	return Result{Message: b.String()}
}

func (h *Handler) model(args []string) Result {
	if len(args) == 0 {
		cfg := h.app.GetConfig()
		return Result{Message: fmt.Sprintf("Current model: %s\n\nAvailable models:\n  opus-4-6     Most capable\n  sonnet-4-6   Balanced speed/capability\n  haiku-4-5    Fastest", cfg.Model)}
	}

	model := args[0]
	switch strings.ToLower(model) {
	case "opus", "opus-4-6", "claude-opus-4-6":
		model = "opus-4-6"
	case "sonnet", "sonnet-4-6", "claude-sonnet-4-6":
		model = "sonnet-4-6"
	case "haiku", "haiku-4-5", "claude-haiku-4-5":
		model = "haiku-4-5"
	}

	h.app.SetModel(model)
	return Result{Message: fmt.Sprintf("Model changed to: %s", model)}
}

func (h *Handler) cost() Result {
	cost := h.app.GetCost()
	tokIn := h.app.GetTokensIn()
	tokOut := h.app.GetTokensOut()
	return Result{
		Message: fmt.Sprintf("Session cost: $%.4f\nInput tokens:  %d\nOutput tokens: %d\nTotal tokens:  %d",
			cost, tokIn, tokOut, tokIn+tokOut),
	}
}

func (h *Handler) showConfig() Result {
	cfg := h.app.GetConfig()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Model:           %s\n", cfg.Model))
	b.WriteString(fmt.Sprintf("Permission Mode: %s\n", cfg.PermissionMode))
	b.WriteString(fmt.Sprintf("Max Turns:       %d\n", cfg.MaxTurns))
	b.WriteString(fmt.Sprintf("Config Dir:      %s\n", config.GlobalConfigDir()))
	if len(cfg.MCPServers) > 0 {
		b.WriteString(fmt.Sprintf("MCP Servers:     %d configured\n", len(cfg.MCPServers)))
		for name := range cfg.MCPServers {
			b.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}
	if cfg.BaseURL != "" {
		b.WriteString(fmt.Sprintf("Base URL:        %s\n", cfg.BaseURL))
	}
	return Result{Message: b.String()}
}

func (h *Handler) permissions(args []string) Result {
	if len(args) == 0 {
		cfg := h.app.GetConfig()
		return Result{
			Message: fmt.Sprintf("Current mode: %s\n\nAvailable modes:\n  default           Ask for each tool\n  acceptEdits       Auto-approve read-only tools\n  bypassPermissions Skip all prompts\n  plan              Plan mode (no execution)", cfg.PermissionMode),
		}
	}

	mode := args[0]
	switch mode {
	case "default", "acceptEdits", "bypassPermissions", "plan":
		h.app.SetPermissionMode(mode)
		return Result{Message: fmt.Sprintf("Permission mode changed to: %s", mode)}
	default:
		return Result{Message: fmt.Sprintf("Unknown permission mode: %s", mode)}
	}
}

func (h *Handler) status() Result {
	cfg := h.app.GetConfig()
	a := h.app.GetAgent()

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Model:      %s\n", cfg.Model))
	b.WriteString(fmt.Sprintf("Cost:       $%.4f\n", h.app.GetCost()))
	b.WriteString(fmt.Sprintf("Tokens:     %d in / %d out\n", h.app.GetTokensIn(), h.app.GetTokensOut()))
	b.WriteString(fmt.Sprintf("Permission: %s\n", cfg.PermissionMode))
	if a != nil {
		b.WriteString(fmt.Sprintf("Session:    %s\n", a.SessionID()))
	}
	return Result{Message: b.String()}
}

func (h *Handler) memory() Result {
	memPath := config.MemoryPath()
	return Result{Message: fmt.Sprintf("Memory directory: %s\nUse the agent to manage memories.", memPath)}
}
