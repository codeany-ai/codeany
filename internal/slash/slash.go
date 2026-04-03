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
	BriefToggle   bool   // Toggle brief mode
	SessionTitle  string // Rename current session
	VimToggle     bool   // Toggle vim mode
	StartLogin    bool   // Start interactive login wizard
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
		// Auth
		{Name: "/login", Description: "Set API key", HasArgs: true},
		{Name: "/logout", Description: "Remove stored API key"},
		// Theme
		{Name: "/theme", Description: "Switch color theme", HasArgs: true},
		// Utilities
		{Name: "/copy", Description: "Copy last response to clipboard"},
		{Name: "/stats", Description: "Detailed session statistics"},
		{Name: "/retry", Description: "Retry last message"},
		{Name: "/usage", Description: "Detailed API usage breakdown"},
		// More Git
		{Name: "/branch", Description: "Git branch management", HasArgs: true},
		{Name: "/pr", Description: "Create pull request", HasArgs: true},
		{Name: "/stash", Description: "Git stash management", HasArgs: true},
		// Analysis
		{Name: "/security-review", Aliases: []string{"/sec"}, Description: "Security vulnerability scan", HasArgs: true},
		{Name: "/refactor", Description: "Refactor code", HasArgs: true},
		{Name: "/summary", Description: "Summarize project/codebase", HasArgs: true},
		{Name: "/ask", Description: "Quick Q&A without tools", HasArgs: true},
		// Session
		{Name: "/rename", Description: "Rename current session", HasArgs: true},
		// Misc
		{Name: "/vim", Description: "Toggle vim mode"},
		{Name: "/feedback", Description: "Report bugs or feedback"},
		{Name: "/tips", Description: "Show a random tip"},
		// Multi-agent
		{Name: "/team", Description: "Team management (create, add, send, inbox)", HasArgs: true},
		{Name: "/worktree", Description: "Git worktree isolation (enter, exit)", HasArgs: true},
		// Mid-query
		{Name: "/btw", Description: "Side question (queued during execution)", HasArgs: true},
		// Reasoning
		{Name: "/effort", Description: "Set reasoning effort (low/medium/high/max)", HasArgs: true},
		{Name: "/passes", Description: "Multi-pass task execution", HasArgs: true},
		// Agents
		{Name: "/agents", Description: "Show available agent types"},
		{Name: "/tasks", Description: "Show task list"},
		// Navigation
		{Name: "/rewind", Description: "Undo last conversation turn"},
		{Name: "/brief", Description: "Toggle brief output mode"},
		{Name: "/output-style", Description: "Change output format", HasArgs: true},
		{Name: "/color", Description: "Color settings"},
		// Sharing
		{Name: "/share", Description: "Export conversation for sharing"},
		{Name: "/tag", Description: "Tag current session", HasArgs: true},
		// Context
		{Name: "/env", Description: "Show environment variables"},
		{Name: "/add-dir", Description: "Add directory to context", HasArgs: true},
		{Name: "/onboarding", Description: "First-time project setup"},
		// Analysis
		{Name: "/insights", Description: "Code insights and analysis", HasArgs: true},
		// More Git
		{Name: "/bughunter", Description: "Deep bug investigation", HasArgs: true},
		{Name: "/commit-push-pr", Description: "Commit + push + create PR", HasArgs: true},
		{Name: "/pr-comments", Description: "Review PR comments", HasArgs: true},
		// Info
		{Name: "/keybindings", Description: "Show keyboard shortcuts"},
		{Name: "/release-notes", Description: "Show release notes"},
		{Name: "/reload-plugins", Description: "Reload all plugins"},
		{Name: "/thinkback", Description: "Reflect on completed work"},
		{Name: "/statusline", Description: "Status line info"},
		{Name: "/privacy", Description: "Privacy information"},
		{Name: "/issue", Description: "Report a bug/issue", HasArgs: true},
		{Name: "/upgrade", Description: "Alias for update"},
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
	case "/login":
		return h.loginCmd(args)
	case "/logout":
		return h.logoutCmd(args)
	case "/theme":
		return h.themeCmd(args)
	case "/copy":
		return h.copyCmd(args)
	case "/stats":
		return h.statsCmd(args)
	case "/retry":
		return h.retryCmd(args)
	case "/usage":
		return h.usageCmd(args)
	case "/branch":
		return h.branchCmd(args)
	case "/pr":
		return h.prCmd(args)
	case "/stash":
		return h.stashCmd(args)
	case "/security-review", "/sec":
		return h.securityReviewCmd(args)
	case "/refactor":
		return h.refactorCmd(args)
	case "/summary":
		return h.summaryCmd(args)
	case "/ask":
		return h.askCmd(args)
	case "/rename":
		return h.renameCmd(args)
	case "/vim":
		return h.vimCmd(args)
	case "/feedback":
		return h.feedbackCmd(args)
	case "/tips":
		return h.tipsCmd(args)
	case "/team":
		return h.teamCmd(args)
	case "/worktree":
		return h.worktreeCmd(args)
	case "/btw":
		return h.btwCmd(args)
	case "/effort":
		return h.effortCmd(args)
	case "/passes":
		return h.passesCmd(args)
	case "/agents":
		return h.agentsCmd(args)
	case "/tasks":
		return h.tasksCmd(args)
	case "/rewind":
		return h.rewindCmd(args)
	case "/brief":
		return h.briefCmd(args)
	case "/output-style":
		return h.outputStyleCmd(args)
	case "/color":
		return h.colorCmd(args)
	case "/share":
		return h.shareCmd(args)
	case "/tag":
		return h.tagCmd(args)
	case "/env":
		return h.envCmd(args)
	case "/add-dir":
		return h.addDirCmd(args)
	case "/onboarding":
		return h.onboardingCmd(args)
	case "/insights":
		return h.insightsCmd(args)
	case "/bughunter":
		return h.bughunterCmd(args)
	case "/commit-push-pr":
		return h.commitPushPrCmd(args)
	case "/pr-comments":
		return h.prCommentsCmd(args)
	case "/keybindings":
		return h.keybindingsCmd(args)
	case "/release-notes":
		return h.releaseNotesCmd(args)
	case "/reload-plugins":
		return h.reloadPluginsCmd(args)
	case "/thinkback":
		return h.thinkbackCmd(args)
	case "/statusline":
		return h.statuslineCmd(args)
	case "/privacy", "/privacy-settings":
		return h.privacyCmd(args)
	case "/issue":
		return h.issueCmd(args)
	case "/upgrade":
		return Result{Message: "Run `codeany update` from the command line to upgrade."}
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

	b.WriteString("Core:\n")
	b.WriteString("  /help          Show this help\n")
	b.WriteString("  /clear         Clear conversation\n")
	b.WriteString("  /compact       Compact with instructions\n")
	b.WriteString("  /model [name]  Show or change model\n")
	b.WriteString("  /fast          Toggle faster model\n")
	b.WriteString("  /effort [lvl]  Set reasoning depth (low/med/high/max)\n")
	b.WriteString("  /plan [task]   Plan mode / plan a task\n")
	b.WriteString("  /brief         Toggle concise output\n")
	b.WriteString("  /quit          Exit\n")

	b.WriteString("\nGit & Code:\n")
	b.WriteString("  /commit [msg]  Git commit\n")
	b.WriteString("  /review        Code review\n")
	b.WriteString("  /diff          Git diff summary\n")
	b.WriteString("  /branch        Branch management\n")
	b.WriteString("  /pr [desc]     Create pull request\n")
	b.WriteString("  /stash         Stash management\n")
	b.WriteString("  /bug <desc>    Investigate a bug\n")
	b.WriteString("  /test          Run tests\n")
	b.WriteString("  /sec           Security review\n")
	b.WriteString("  /refactor      Refactor code\n")

	b.WriteString("\nAnalysis:\n")
	b.WriteString("  /summary       Summarize codebase\n")
	b.WriteString("  /insights      Code insights & analysis\n")
	b.WriteString("  /passes <task> Multi-pass execution\n")
	b.WriteString("  /ask <q>       Quick Q&A (no tools)\n")
	b.WriteString("  /btw <q>       Side question during query\n")

	b.WriteString("\nTools & Context:\n")
	b.WriteString("  /mcp           MCP servers (list/tools/reconnect)\n")
	b.WriteString("  /skills        List skills\n")
	b.WriteString("  /plugin        Plugins (list/create/reload)\n")
	b.WriteString("  /hooks         Show hooks\n")
	b.WriteString("  /context       Show context sources\n")
	b.WriteString("  /add-dir <p>   Add directory to context\n")
	b.WriteString("  /init          Initialize project\n")
	b.WriteString("  /onboarding    First-time setup wizard\n")
	b.WriteString("  /env           Show environment\n")

	b.WriteString("\nSession:\n")
	b.WriteString("  /cost          Cost and tokens\n")
	b.WriteString("  /usage         Detailed API usage\n")
	b.WriteString("  /stats         Session statistics\n")
	b.WriteString("  /session       Session details\n")
	b.WriteString("  /files         Files accessed\n")
	b.WriteString("  /resume        Recent sessions\n")
	b.WriteString("  /export        Export conversation\n")
	b.WriteString("  /share         Share conversation\n")
	b.WriteString("  /copy          Copy last response\n")
	b.WriteString("  /retry         Retry last message\n")
	b.WriteString("  /rewind        Undo last turn\n")
	b.WriteString("  /rename <t>    Name session\n")
	b.WriteString("  /tag <t>       Tag session\n")

	b.WriteString("\nMulti-agent:\n")
	b.WriteString("  /agents        Available agent types\n")
	b.WriteString("  /tasks         Task list\n")
	b.WriteString("  /team          Team management\n")
	b.WriteString("  /worktree      Git worktree isolation\n")

	b.WriteString("\nConfig:\n")
	b.WriteString("  /config        Show configuration\n")
	b.WriteString("  /permissions   Permission mode + rules\n")
	b.WriteString("  /login         Setup API key (interactive)\n")
	b.WriteString("  /logout        Remove API key\n")
	b.WriteString("  /doctor        Environment check\n")
	b.WriteString("  /theme         Color theme\n")

	b.WriteString("\nKeys: Enter send · Shift+Enter newline · Ctrl+C cancel · Ctrl+L clear")
	b.WriteString("\n      Ctrl+O expand · PgUp/Down scroll · !cmd shell · Tab complete")

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
		mode := cfg.PermissionMode
		if mode == "" {
			mode = "default"
		}
		indicator := map[string]string{
			"default":           "🔒 Asks for write operations",
			"acceptEdits":       "✎  Auto-approves all file edits + bash",
			"bypassPermissions": "⚡ Allows everything (no prompts)",
			"plan":              "📋 Plan only, no execution",
		}
		desc := indicator[mode]
		return Result{
			Message: fmt.Sprintf("Current mode: %s — %s\n\nSwitch with:\n  /permissions bypass     Skip all prompts\n  /permissions default    Ask for write ops\n  /permissions auto       Auto-approve edits\n  /permissions plan       Plan only", mode, desc),
		}
	}

	mode := args[0]
	// Aliases for convenience
	switch strings.ToLower(mode) {
	case "bypass", "bypasspermissions", "off", "y", "yes":
		mode = "bypassPermissions"
	case "auto", "acceptedits", "accept":
		mode = "acceptEdits"
	case "default", "on", "ask":
		mode = "default"
	case "plan":
		mode = "plan"
	}

	switch mode {
	case "default", "acceptEdits", "bypassPermissions", "plan":
		h.app.SetPermissionMode(mode)
		indicator := ""
		switch mode {
		case "bypassPermissions":
			indicator = " ⚡ All tools auto-approved"
		case "acceptEdits":
			indicator = " ✎ File edits auto-approved"
		case "plan":
			indicator = " 📋 Plan only mode"
		case "default":
			indicator = " 🔒 Will ask for write operations"
		}
		return Result{Message: fmt.Sprintf("Permission mode: %s%s", mode, indicator)}
	case "rules":
		return Result{Message: config.LoadPermissionRules().FormatRules()}
	case "allow":
		if len(args) < 2 {
			return Result{Message: "Usage: /permissions allow <tool> [pattern]\nExample: /permissions allow Bash git*"}
		}
		tool := args[1]
		pattern := ""
		if len(args) > 2 {
			pattern = strings.Join(args[2:], " ")
		}
		config.LoadPermissionRules().AddAllowRule(tool, pattern)
		return Result{Message: fmt.Sprintf("✓ Allow rule added: %s %s", tool, pattern)}
	case "deny":
		if len(args) < 2 {
			return Result{Message: "Usage: /permissions deny <tool> [pattern]\nExample: /permissions deny Bash rm*"}
		}
		tool := args[1]
		pattern := ""
		if len(args) > 2 {
			pattern = strings.Join(args[2:], " ")
		}
		config.LoadPermissionRules().AddDenyRule(tool, pattern)
		return Result{Message: fmt.Sprintf("✓ Deny rule added: %s %s", tool, pattern)}
	default:
		return Result{Message: fmt.Sprintf("Unknown: %s\nUse: bypass, auto, default, plan, rules, allow <tool>, deny <tool>", mode)}
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
