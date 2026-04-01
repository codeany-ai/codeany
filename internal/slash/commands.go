package slash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/codeany-ai/codeany/internal/config"
	"github.com/codeany-ai/codeany/internal/plugins"
	"github.com/codeany-ai/codeany/internal/session"
	"github.com/codeany-ai/codeany/internal/skills"
	"github.com/codeany-ai/open-agent-sdk-go/mcp"
)

// ─── /init ────────────────────────────────────────

func (h *Handler) init(args []string) Result {
	cwd, _ := os.Getwd()

	// Check if CODEANY.md or CLAUDE.md already exists
	for _, name := range []string{"CODEANY.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(cwd, name)); err == nil {
			return Result{Message: fmt.Sprintf("%s already exists in this project.\nEdit it directly or delete and re-run /init.", name)}
		}
	}

	// Generate a basic CODEANY.md by analyzing the project
	content := generateProjectMD(cwd)
	path := filepath.Join(cwd, "CODEANY.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return Result{Message: fmt.Sprintf("Failed to create CODEANY.md: %v", err)}
	}

	return Result{Message: fmt.Sprintf("Created %s\nEdit it to customize instructions for your project.", path)}
}

func generateProjectMD(cwd string) string {
	var b strings.Builder
	b.WriteString("# CODEANY.md\n\n")
	b.WriteString("## Project Overview\n\n")
	b.WriteString("<!-- Describe your project here -->\n\n")

	// Detect language/framework
	files := detectProjectFiles(cwd)
	if len(files) > 0 {
		b.WriteString("## Tech Stack\n\n")
		for _, f := range files {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Commands\n\n")
	b.WriteString("```bash\n")
	b.WriteString("# Install dependencies\n# <add command>\n\n")
	b.WriteString("# Run dev server\n# <add command>\n\n")
	b.WriteString("# Run tests\n# <add command>\n\n")
	b.WriteString("# Build\n# <add command>\n")
	b.WriteString("```\n\n")

	b.WriteString("## Code Style\n\n")
	b.WriteString("<!-- Add any code style guidelines -->\n\n")

	return b.String()
}

func detectProjectFiles(cwd string) []string {
	var detected []string
	checks := map[string]string{
		"go.mod":         "Go",
		"package.json":   "Node.js / JavaScript",
		"Cargo.toml":     "Rust",
		"pyproject.toml": "Python",
		"requirements.txt": "Python",
		"pom.xml":        "Java (Maven)",
		"build.gradle":   "Java (Gradle)",
		"Gemfile":        "Ruby",
		"composer.json":  "PHP",
		"Makefile":       "Make",
		"Dockerfile":     "Docker",
		"docker-compose.yml": "Docker Compose",
		".github/workflows": "GitHub Actions",
		"tsconfig.json":  "TypeScript",
	}
	for file, tech := range checks {
		path := filepath.Join(cwd, file)
		if _, err := os.Stat(path); err == nil {
			detected = append(detected, tech)
		}
	}
	return detected
}

// ─── /doctor ──────────────────────────────────────

func (h *Handler) doctor() Result {
	var b strings.Builder
	b.WriteString("Environment check:\n\n")

	// OS
	b.WriteString(fmt.Sprintf("  OS:       %s/%s\n", runtime.GOOS, runtime.GOARCH))

	// Shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "(unknown)"
	}
	b.WriteString(fmt.Sprintf("  Shell:    %s\n", shell))

	// Git
	if out, err := exec.Command("git", "--version").Output(); err == nil {
		b.WriteString(fmt.Sprintf("  Git:      %s", strings.TrimSpace(string(out))))
	} else {
		b.WriteString("  Git:      ✗ not found\n")
	}

	// API key
	cfg := h.app.GetConfig()
	if cfg.APIKey != "" {
		b.WriteString(fmt.Sprintf("  API Key:  ✓ set (%s...)\n", cfg.APIKey[:min(8, len(cfg.APIKey))]))
	} else {
		keyEnvs := []string{"CODEANY_API_KEY", "ANTHROPIC_API_KEY"}
		found := false
		for _, env := range keyEnvs {
			if v := os.Getenv(env); v != "" {
				b.WriteString(fmt.Sprintf("  API Key:  ✓ from %s\n", env))
				found = true
				break
			}
		}
		if !found {
			b.WriteString("  API Key:  ✗ not set (set ANTHROPIC_API_KEY or CODEANY_API_KEY)\n")
		}
	}

	// Base URL
	if cfg.BaseURL != "" {
		b.WriteString(fmt.Sprintf("  Base URL: %s\n", cfg.BaseURL))
	} else if u := os.Getenv("CODEANY_BASE_URL"); u != "" {
		b.WriteString(fmt.Sprintf("  Base URL: %s (from env)\n", u))
	}

	// Model
	b.WriteString(fmt.Sprintf("  Model:    %s\n", cfg.Model))

	// Config dir
	b.WriteString(fmt.Sprintf("  Config:   %s\n", config.GlobalConfigDir()))

	// CODEANY.md / CLAUDE.md
	cwd, _ := os.Getwd()
	for _, name := range []string{"CODEANY.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(cwd, name)); err == nil {
			b.WriteString(fmt.Sprintf("  %s:  ✓ found\n", name))
		}
	}

	// MCP servers
	if len(cfg.MCPServers) > 0 {
		b.WriteString(fmt.Sprintf("  MCP:      %d servers configured\n", len(cfg.MCPServers)))
	}

	// Permissions
	perms := config.LoadPermissionRules()
	if len(perms.AlwaysAllow) > 0 {
		b.WriteString(fmt.Sprintf("  Perms:    %d always-allow rules\n", len(perms.AlwaysAllow)))
	}

	return Result{Message: b.String()}
}

// ─── /mcp ─────────────────────────────────────────

func (h *Handler) mcpCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "Agent not initialized."}
	}

	client := a.MCPClient()
	if client == nil {
		return Result{Message: "MCP client not available."}
	}

	if len(args) == 0 {
		return h.mcpList(client)
	}

	switch args[0] {
	case "list":
		return h.mcpList(client)
	case "tools":
		return h.mcpTools(client, args[1:])
	case "reconnect":
		if len(args) < 2 {
			return Result{Message: "Usage: /mcp reconnect <server-name>"}
		}
		return h.mcpReconnect(client, args[1])
	default:
		return Result{Message: fmt.Sprintf("Unknown /mcp subcommand: %s\nUsage: /mcp [list|tools|reconnect <name>]", args[0])}
	}
}

func (h *Handler) mcpList(client *mcp.Client) Result {
	conns := client.AllConnections()
	if len(conns) == 0 {
		return Result{Message: "No MCP servers configured.\nAdd servers in ~/.codeany/settings.json under \"mcpServers\"."}
	}

	var b strings.Builder
	b.WriteString("MCP Servers:\n\n")
	for _, conn := range conns {
		status := "?"
		switch conn.Status {
		case "connected":
			status = "✓"
		case "error":
			status = "✗"
		case "disconnected":
			status = "○"
		default:
			status = "…"
		}

		tools := ""
		if conn.Tools != nil {
			tools = fmt.Sprintf(" (%d tools)", len(conn.Tools))
		}

		b.WriteString(fmt.Sprintf("  %s %s%s\n", status, conn.Name, tools))
		if conn.Error != "" {
			b.WriteString(fmt.Sprintf("    Error: %s\n", conn.Error))
		}
	}
	b.WriteString("\nUse /mcp tools [server] to list tools, /mcp reconnect <server> to reconnect.")
	return Result{Message: b.String()}
}

func (h *Handler) mcpTools(client *mcp.Client, args []string) Result {
	tools := client.AllTools()
	if len(tools) == 0 {
		return Result{Message: "No MCP tools available."}
	}

	var b strings.Builder
	b.WriteString("MCP Tools:\n\n")

	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}

	count := 0
	for _, t := range tools {
		if filter != "" && !strings.Contains(strings.ToLower(t.Name), strings.ToLower(filter)) {
			continue
		}
		b.WriteString(fmt.Sprintf("  %s\n", t.Name))
		if t.Description != "" {
			desc := t.Description
			if len(desc) > 80 {
				desc = desc[:80] + "…"
			}
			b.WriteString(fmt.Sprintf("    %s\n", desc))
		}
		count++
		if count >= 50 {
			b.WriteString(fmt.Sprintf("\n  ... and %d more\n", len(tools)-50))
			break
		}
	}

	b.WriteString(fmt.Sprintf("\nTotal: %d tools", len(tools)))
	return Result{Message: b.String()}
}

func (h *Handler) mcpReconnect(client *mcp.Client, name string) Result {
	conn := client.GetConnection(name)
	if conn == nil {
		return Result{Message: fmt.Sprintf("Server %q not found.", name)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := h.app.GetConfig()
	serverCfg, ok := cfg.MCPServers[name]
	if !ok {
		return Result{Message: fmt.Sprintf("No config found for server %q.", name)}
	}

	_, err := client.ConnectServer(ctx, name, serverCfg)
	if err != nil {
		return Result{Message: fmt.Sprintf("Failed to reconnect %q: %v", name, err)}
	}
	return Result{Message: fmt.Sprintf("Reconnected to %q.", name)}
}

// ─── /skills ──────────────────────────────────────

func (h *Handler) skillsCmd(args []string) Result {
	allSkills := skills.LoadAll()
	return Result{Message: skills.FormatSkillList(allSkills)}
}

// handleSkillInvocation checks if a slash command is a skill name and invokes it
func (h *Handler) HandleSkillInvocation(cmd string, args []string) (Result, bool) {
	allSkills := skills.LoadAll()
	name := strings.TrimPrefix(cmd, "/")
	skill := skills.FindByName(allSkills, name)
	if skill == nil {
		return Result{}, false
	}

	// Build the skill prompt
	arguments := strings.Join(args, " ")
	prompt := skill.Content
	if arguments != "" {
		prompt = strings.ReplaceAll(prompt, "$ARGUMENTS", arguments)
		if !strings.Contains(skill.Content, "$ARGUMENTS") {
			prompt = prompt + "\n\nUser request: " + arguments
		}
	}

	return Result{
		Message: fmt.Sprintf("Running skill: %s\n(Sending to agent as prompt)", skill.Name),
		SkillPrompt: prompt,
	}, true
}

// ─── /compact ─────────────────────────────────────

func (h *Handler) compactCmd(args []string) Result {
	instruction := strings.Join(args, " ")
	if instruction == "" {
		instruction = "Summarize the conversation so far, keeping key decisions and context."
	}

	return Result{
		Message:       "Conversation compacted. Context summary will be provided to the agent.",
		ClearMessages: true,
		SkillPrompt:   fmt.Sprintf("[System: Previous conversation was compacted. Summary instruction: %s]", instruction),
	}
}

// ─── /plan ────────────────────────────────────────

func (h *Handler) planCmd(args []string) Result {
	if len(args) == 0 {
		// Toggle plan mode
		return h.planToggle()
	}

	task := strings.Join(args, " ")
	return Result{
		SkillPrompt: fmt.Sprintf("Create a detailed implementation plan for the following task. Do NOT execute anything, only plan.\n\nTask: %s\n\nProvide:\n1. Step-by-step breakdown\n2. Files that need to be modified\n3. Potential risks or considerations\n4. Estimated complexity", task),
	}
}

// ─── /review ──────────────────────────────────────

func (h *Handler) reviewCmd(args []string) Result {
	target := strings.Join(args, " ")
	if target == "" {
		target = "the recent changes (git diff)"
	}

	return Result{
		SkillPrompt: fmt.Sprintf("Review %s. Check for:\n1. Bugs and logic errors\n2. Security issues\n3. Performance concerns\n4. Code style and best practices\n5. Missing error handling\n\nProvide specific, actionable feedback.", target),
	}
}

// ─── /commit ──────────────────────────────────────

func (h *Handler) commitCmd(args []string) Result {
	msg := strings.Join(args, " ")
	prompt := "Review the current git diff (staged and unstaged changes), then create an appropriate git commit."
	if msg != "" {
		prompt += fmt.Sprintf("\n\nUse this as the commit message guidance: %s", msg)
	}
	prompt += "\n\nFollow conventional commit format. Stage relevant files and create the commit."

	return Result{SkillPrompt: prompt}
}

// ─── /diff ────────────────────────────────────────

func (h *Handler) diffCmd(args []string) Result {
	return Result{
		SkillPrompt: "Show me the current git diff (both staged and unstaged). Summarize what changed and why.",
	}
}

// ─── /export ──────────────────────────────────────

func (h *Handler) exportCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No conversation to export."}
	}

	// Export conversation to a file
	home, _ := os.UserHomeDir()
	filename := fmt.Sprintf("codeany-export-%s.md", time.Now().Format("20060102-150405"))
	path := filepath.Join(home, filename)

	var b strings.Builder
	b.WriteString("# Codeany Conversation Export\n\n")
	b.WriteString(fmt.Sprintf("Date: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("Model: %s\n\n---\n\n", h.app.GetConfig().Model))

	for _, msg := range a.GetMessages() {
		switch msg.Role {
		case "user":
			b.WriteString("## User\n\n")
			for _, block := range msg.Content {
				if block.Text != "" {
					b.WriteString(block.Text + "\n\n")
				}
			}
		case "assistant":
			b.WriteString("## Assistant\n\n")
			for _, block := range msg.Content {
				if block.Text != "" {
					b.WriteString(block.Text + "\n\n")
				}
			}
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return Result{Message: fmt.Sprintf("Failed to export: %v", err)}
	}
	return Result{Message: fmt.Sprintf("Exported conversation to %s", path)}
}

// ─── /resume ──────────────────────────────────────

func (h *Handler) resumeCmd(args []string) Result {
	cwd, _ := os.Getwd()
	sessDir := config.SessionPath()

	sessions := session.ListRecent(sessDir, 10, cwd)
	return Result{Message: session.FormatSessionList(sessions)}
}

// ─── /fast ────────────────────────────────────────

func (h *Handler) fastCmd(args []string) Result {
	cfg := h.app.GetConfig()
	current := cfg.Model

	// Toggle between current model and a faster variant
	if strings.Contains(current, "opus") {
		h.app.SetModel(strings.ReplaceAll(current, "opus", "sonnet"))
		return Result{Message: fmt.Sprintf("Switched to fast mode: %s → %s", current, strings.ReplaceAll(current, "opus", "sonnet"))}
	} else if strings.Contains(current, "sonnet") {
		h.app.SetModel(strings.ReplaceAll(current, "sonnet", "haiku"))
		return Result{Message: fmt.Sprintf("Switched to fast mode: %s → %s", current, strings.ReplaceAll(current, "sonnet", "haiku"))}
	}

	return Result{Message: fmt.Sprintf("Current model: %s (already using fastest available)", current)}
}

// ─── /bug ─────────────────────────────────────────

func (h *Handler) bugCmd(args []string) Result {
	desc := strings.Join(args, " ")
	if desc == "" {
		return Result{Message: "Usage: /bug <description>\n\nDescribe the bug and the agent will investigate."}
	}
	return Result{
		SkillPrompt: fmt.Sprintf("There is a bug: %s\n\nInvestigate this bug:\n1. Find the relevant code\n2. Identify the root cause\n3. Propose a fix\n4. Implement the fix if straightforward", desc),
	}
}

// ─── /test ────────────────────────────────────────

func (h *Handler) testCmd(args []string) Result {
	target := strings.Join(args, " ")
	if target == "" {
		return Result{
			SkillPrompt: "Find and run the test suite for this project. Report the results.",
		}
	}
	return Result{
		SkillPrompt: fmt.Sprintf("Run tests for: %s\n\nReport which tests pass and fail. If there are failures, investigate the cause.", target),
	}
}

// ─── /plugin ──────────────────────────────────────

func (h *Handler) pluginCmd(args []string) Result {
	allPlugins := plugins.LoadAll(config.GlobalConfigDir())
	return Result{Message: plugins.FormatPluginList(allPlugins)}
}

// ─── /hooks ───────────────────────────────────────

func (h *Handler) hooksCmd(args []string) Result {
	cfg := h.app.GetConfig()
	if cfg.Hooks == nil || (len(cfg.Hooks.PreToolUse) == 0 && len(cfg.Hooks.PostToolUse) == 0) {
		return Result{Message: "No hooks configured.\n\nAdd hooks in ~/.codeany/settings.json:\n```json\n{\n  \"hooks\": {\n    \"preToolUse\": [{\"matcher\": \"Bash\", \"command\": \"echo checking...\"}],\n    \"postToolUse\": []\n  }\n}\n```"}
	}

	var b strings.Builder
	b.WriteString("Configured hooks:\n\n")
	if len(cfg.Hooks.PreToolUse) > 0 {
		b.WriteString("  Pre-tool-use:\n")
		for _, h := range cfg.Hooks.PreToolUse {
			b.WriteString(fmt.Sprintf("    %s → %s\n", h.Matcher, h.Command))
		}
	}
	if len(cfg.Hooks.PostToolUse) > 0 {
		b.WriteString("  Post-tool-use:\n")
		for _, h := range cfg.Hooks.PostToolUse {
			b.WriteString(fmt.Sprintf("    %s → %s\n", h.Matcher, h.Command))
		}
	}
	return Result{Message: b.String()}
}

// ─── /context ─────────────────────────────────────

func (h *Handler) contextCmd(args []string) Result {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	var b strings.Builder
	b.WriteString("Context sources:\n\n")

	// Check for config files
	configFiles := []struct {
		path string
		name string
	}{
		{filepath.Join(cwd, "CODEANY.md"), "CODEANY.md (project)"},
		{filepath.Join(cwd, "CLAUDE.md"), "CLAUDE.md (project)"},
		{filepath.Join(cwd, "CODEANY.local.md"), "CODEANY.local.md (personal)"},
		{filepath.Join(cwd, "CLAUDE.local.md"), "CLAUDE.local.md (personal)"},
		{filepath.Join(cwd, ".codeany", "CODEANY.md"), ".codeany/CODEANY.md"},
		{filepath.Join(cwd, ".claude", "CLAUDE.md"), ".claude/CLAUDE.md"},
		{filepath.Join(home, ".codeany", "CODEANY.md"), "~/.codeany/CODEANY.md (global)"},
		{filepath.Join(home, ".claude", "CLAUDE.md"), "~/.claude/CLAUDE.md (global)"},
	}

	for _, cf := range configFiles {
		if info, err := os.Stat(cf.path); err == nil {
			b.WriteString(fmt.Sprintf("  ✓ %s (%d bytes)\n", cf.name, info.Size()))
		}
	}

	// Rules
	for _, dir := range []string{
		filepath.Join(cwd, ".codeany", "rules"),
		filepath.Join(cwd, ".claude", "rules"),
	} {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					b.WriteString(fmt.Sprintf("  ✓ rules/%s\n", e.Name()))
				}
			}
		}
	}

	// Memory
	memDir := config.MemoryPath()
	if entries, err := os.ReadDir(memDir); err == nil {
		count := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				count++
			}
		}
		if count > 0 {
			b.WriteString(fmt.Sprintf("  ✓ memory/ (%d files)\n", count))
		}
	}

	// Skills
	allSkills := skills.LoadAll()
	if len(allSkills) > 0 {
		b.WriteString(fmt.Sprintf("  ✓ skills (%d loaded)\n", len(allSkills)))
	}

	// Plugins
	allPlugins := plugins.LoadAll(config.GlobalConfigDir())
	if len(allPlugins) > 0 {
		b.WriteString(fmt.Sprintf("  ✓ plugins (%d loaded)\n", len(allPlugins)))
	}

	// MCP
	cfg := h.app.GetConfig()
	if len(cfg.MCPServers) > 0 {
		b.WriteString(fmt.Sprintf("  ✓ MCP servers (%d configured)\n", len(cfg.MCPServers)))
	}

	return Result{Message: b.String()}
}

// ─── /session ─────────────────────────────────────

func (h *Handler) sessionCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No active session."}
	}

	cfg := h.app.GetConfig()
	var b strings.Builder
	b.WriteString("Session info:\n\n")
	b.WriteString(fmt.Sprintf("  ID:         %s\n", a.SessionID()))
	b.WriteString(fmt.Sprintf("  Model:      %s\n", cfg.Model))
	b.WriteString(fmt.Sprintf("  Cost:       $%.4f\n", h.app.GetCost()))
	b.WriteString(fmt.Sprintf("  Tokens in:  %d\n", h.app.GetTokensIn()))
	b.WriteString(fmt.Sprintf("  Tokens out: %d\n", h.app.GetTokensOut()))
	b.WriteString(fmt.Sprintf("  Messages:   %d\n", len(a.GetMessages())))

	cwd, _ := os.Getwd()
	b.WriteString(fmt.Sprintf("  CWD:        %s\n", cwd))
	b.WriteString(fmt.Sprintf("  Permission: %s\n", cfg.PermissionMode))

	return Result{Message: b.String()}
}

// ─── /files ───────────────────────────────────────

func (h *Handler) filesCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No active session."}
	}

	// Extract file paths from tool calls in conversation
	files := make(map[string]bool)
	for _, msg := range a.GetMessages() {
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				if fp, ok := block.Input["file_path"].(string); ok {
					files[fp] = true
				}
				if p, ok := block.Input["path"].(string); ok && p != "" {
					files[p] = true
				}
			}
		}
	}

	if len(files) == 0 {
		return Result{Message: "No files accessed in this session."}
	}

	var b strings.Builder
	b.WriteString("Files accessed this session:\n\n")
	for f := range files {
		b.WriteString(fmt.Sprintf("  %s\n", shortenPathStr(f)))
	}
	return Result{Message: b.String()}
}

func shortenPathStr(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// ─── /planToggle ──────────────────────────────────

func (h *Handler) planToggle() Result {
	// This is handled specially - the TUI model checks for planMode
	return Result{
		Message:   "Plan mode toggled. (Agent will plan but not execute tools.)",
		PlanToggle: true,
	}
}

// ─── helpers ──────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
