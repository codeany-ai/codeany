package slash

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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
	"github.com/codeany-ai/codeany/internal/team"
	"github.com/codeany-ai/codeany/internal/worktree"
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
	configDir := config.GlobalConfigDir()

	if len(args) == 0 {
		allPlugins := plugins.LoadAll(configDir)
		return Result{Message: plugins.FormatPluginList(allPlugins)}
	}

	switch args[0] {
	case "list":
		allPlugins := plugins.LoadAll(configDir)
		return Result{Message: plugins.FormatPluginList(allPlugins)}
	case "reload":
		allPlugins := plugins.LoadAll(configDir)
		return Result{Message: fmt.Sprintf("✓ Reloaded %d plugins", len(allPlugins))}
	case "create":
		if len(args) < 2 {
			return Result{Message: "Usage: /plugin create <name>"}
		}
		name := args[1]
		dir := filepath.Join(configDir, "plugins", name)
		os.MkdirAll(dir, 0755)
		manifest := fmt.Sprintf(`{"name": "%s", "description": "", "version": "0.1.0"}`, name)
		os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0644)
		return Result{Message: fmt.Sprintf("✓ Plugin %q created at %s\n\nAdd skills in %s/skills/<name>/SKILL.md", name, dir, dir)}
	default:
		return Result{Message: "Usage: /plugin [list|reload|create <name>]"}
	}
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

// ─── /login ───────────────────────────────────────

func (h *Handler) loginCmd(args []string) Result {
	if len(args) == 0 {
		// Start interactive wizard
		return Result{StartLogin: true}
	}

	// Quick mode: /login <api-key>
	apiKey := args[0]
	return saveAPIKey(apiKey)
}

// saveAPIKey saves an API key and auto-detects provider
func saveAPIKey(apiKey string) Result {
	provider := "anthropic"
	baseURL := ""
	if strings.HasPrefix(apiKey, "sk-or-") {
		provider = "openrouter"
		baseURL = "https://openrouter.ai/api"
	} else if strings.HasPrefix(apiKey, "sk-") && !strings.HasPrefix(apiKey, "sk-ant-") {
		provider = "openai"
		baseURL = "https://api.openai.com/v1"
	}

	return SaveProviderConfig(provider, apiKey, baseURL, "")
}

// SaveProviderConfig writes provider config to settings.json
func SaveProviderConfig(provider, apiKey, baseURL, model string) Result {
	settingsPath := config.GlobalConfigPath()
	var settings map[string]interface{}

	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	settings["apiKey"] = apiKey
	if provider == "openai" || provider == "openrouter" || provider == "custom" {
		settings["provider"] = "openai"
	} else {
		settings["provider"] = "anthropic"
	}
	if baseURL != "" {
		settings["baseURL"] = baseURL
	}
	if model != "" {
		settings["model"] = model
	}

	data, _ := json.MarshalIndent(settings, "", "  ")
	os.MkdirAll(filepath.Dir(settingsPath), 0755)
	if err := os.WriteFile(settingsPath, data, 0600); err != nil {
		return Result{Message: fmt.Sprintf("Failed to save: %v", err)}
	}

	return Result{Message: fmt.Sprintf("✓ Logged in (%s)\n  Stored in %s\n  Restart codeany to apply changes.", provider, settingsPath)}
}

// ─── /logout ──────────────────────────────────────

func (h *Handler) logoutCmd(args []string) Result {
	settingsPath := config.GlobalConfigPath()
	var settings map[string]interface{}

	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		return Result{Message: "No stored API key found."}
	}

	delete(settings, "apiKey")
	data, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsPath, data, 0600)

	return Result{Message: "✓ API key removed from settings.\nSet ANTHROPIC_API_KEY or CODEANY_API_KEY env var to authenticate."}
}

// ─── /theme ───────────────────────────────────────

func (h *Handler) themeCmd(args []string) Result {
	if len(args) == 0 {
		return Result{Message: "Usage: /theme <dark|light>\n\nSwitch the color theme. Currently only supports dark (default)."}
	}

	t := strings.ToLower(args[0])
	switch t {
	case "dark", "light":
		return Result{Message: fmt.Sprintf("Theme set to: %s\n(Theme switching will take effect after restart)", t)}
	default:
		return Result{Message: fmt.Sprintf("Unknown theme: %s. Available: dark, light", t)}
	}
}

// ─── /copy ────────────────────────────────────────

func (h *Handler) copyCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No conversation to copy from."}
	}

	msgs := a.GetMessages()
	// Find last assistant message
	var lastText string
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			for _, block := range msgs[i].Content {
				if block.Text != "" {
					lastText = block.Text
					break
				}
			}
			if lastText != "" {
				break
			}
		}
	}

	if lastText == "" {
		return Result{Message: "No assistant response to copy."}
	}

	// Copy to clipboard
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard")
	default:
		return Result{Message: "Clipboard not supported on this platform."}
	}

	cmd.Stdin = strings.NewReader(lastText)
	if err := cmd.Run(); err != nil {
		return Result{Message: fmt.Sprintf("Failed to copy: %v", err)}
	}

	preview := lastText
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	return Result{Message: fmt.Sprintf("✓ Copied to clipboard (%d chars)\n  %s", len(lastText), preview)}
}

// ─── /stats ───────────────────────────────────────

func (h *Handler) statsCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No active session."}
	}

	var b strings.Builder
	b.WriteString("Session statistics:\n\n")

	msgs := a.GetMessages()
	userMsgs := 0
	assistantMsgs := 0
	toolCalls := 0
	toolTypes := make(map[string]int)

	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			userMsgs++
		case "assistant":
			assistantMsgs++
			for _, block := range msg.Content {
				if block.Type == "tool_use" {
					toolCalls++
					toolTypes[block.Name]++
				}
			}
		}
	}

	b.WriteString(fmt.Sprintf("  Messages:    %d user, %d assistant\n", userMsgs, assistantMsgs))
	b.WriteString(fmt.Sprintf("  Tool calls:  %d total\n", toolCalls))

	if len(toolTypes) > 0 {
		b.WriteString("  By tool:\n")
		for name, count := range toolTypes {
			b.WriteString(fmt.Sprintf("    %-12s %d\n", name, count))
		}
	}

	b.WriteString(fmt.Sprintf("\n  Cost:        $%.4f\n", h.app.GetCost()))
	b.WriteString(fmt.Sprintf("  Tokens in:   %d\n", h.app.GetTokensIn()))
	b.WriteString(fmt.Sprintf("  Tokens out:  %d\n", h.app.GetTokensOut()))

	return Result{Message: b.String()}
}

// ─── /retry ───────────────────────────────────────

func (h *Handler) retryCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No conversation."}
	}

	msgs := a.GetMessages()
	// Find last user message
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			for _, block := range msgs[i].Content {
				if block.Text != "" {
					return Result{
						Message:     "Retrying last message...",
						SkillPrompt: block.Text,
					}
				}
			}
		}
	}

	return Result{Message: "No previous user message to retry."}
}

// ─── /branch ──────────────────────────────────────

func (h *Handler) branchCmd(args []string) Result {
	if len(args) == 0 {
		return Result{
			SkillPrompt: "Show the current git branch and list recent branches. For each branch, show its last commit.",
		}
	}
	action := args[0]
	switch action {
	case "new", "create":
		if len(args) < 2 {
			return Result{Message: "Usage: /branch new <name>"}
		}
		return Result{
			SkillPrompt: fmt.Sprintf("Create a new git branch named %q from the current branch and switch to it.", args[1]),
		}
	case "switch", "checkout":
		if len(args) < 2 {
			return Result{Message: "Usage: /branch switch <name>"}
		}
		return Result{
			SkillPrompt: fmt.Sprintf("Switch to git branch %q.", args[1]),
		}
	default:
		return Result{
			SkillPrompt: fmt.Sprintf("Git branch operation: %s", strings.Join(args, " ")),
		}
	}
}

// ─── /pr ──────────────────────────────────────────

func (h *Handler) prCmd(args []string) Result {
	desc := strings.Join(args, " ")
	prompt := "Create a pull request for the current branch."
	if desc != "" {
		prompt += fmt.Sprintf("\n\nDescription: %s", desc)
	}
	prompt += "\n\nSteps:\n1. Check current branch and diff against main\n2. Push the branch if needed\n3. Create the PR with a good title and description using `gh pr create`"
	return Result{SkillPrompt: prompt}
}

// ─── /stash ───────────────────────────────────────

func (h *Handler) stashCmd(args []string) Result {
	if len(args) == 0 {
		return Result{
			SkillPrompt: "Show the current git stash list. If there are stashed changes, show what each stash contains.",
		}
	}
	switch args[0] {
	case "save", "push":
		msg := strings.Join(args[1:], " ")
		if msg == "" {
			msg = "WIP"
		}
		return Result{
			SkillPrompt: fmt.Sprintf("Stash current changes with message: %q", msg),
		}
	case "pop", "apply":
		return Result{
			SkillPrompt: "Apply the most recent git stash (pop).",
		}
	default:
		return Result{
			SkillPrompt: fmt.Sprintf("Git stash operation: %s", strings.Join(args, " ")),
		}
	}
}

// ─── /usage ───────────────────────────────────────

func (h *Handler) usageCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No active session."}
	}

	var b strings.Builder
	b.WriteString("API usage:\n\n")

	tracker := a.CostTracker()
	if tracker != nil {
		b.WriteString(fmt.Sprintf("  Total cost:    %s\n", tracker.FormatCost()))
		in, out := tracker.TotalTokens()
		b.WriteString(fmt.Sprintf("  Input tokens:  %d\n", in))
		b.WriteString(fmt.Sprintf("  Output tokens: %d\n", out))
		b.WriteString(fmt.Sprintf("  Total tokens:  %d\n", in+out))

		// Per-model breakdown
		allUsage := tracker.AllModelUsage()
		if len(allUsage) > 0 {
			b.WriteString("\n  By model:\n")
			for model, usage := range allUsage {
				b.WriteString(fmt.Sprintf("    %s: $%.4f (%d in / %d out)\n",
					model, usage.CostUSD, usage.InputTokens, usage.OutputTokens))
			}
		}

		stats := tracker.Stats()
		if dur, ok := stats["totalAPIDuration"]; ok {
			b.WriteString(fmt.Sprintf("\n  API time:      %v\n", dur))
		}
		if dur, ok := stats["totalToolDuration"]; ok {
			b.WriteString(fmt.Sprintf("  Tool time:     %v\n", dur))
		}
	}

	return Result{Message: b.String()}
}

// ─── /security-review ─────────────────────────────

func (h *Handler) securityReviewCmd(args []string) Result {
	target := strings.Join(args, " ")
	if target == "" {
		target = "the current codebase"
	}
	return Result{
		SkillPrompt: fmt.Sprintf("Perform a security review of %s. Check for:\n1. OWASP Top 10 vulnerabilities\n2. Injection attacks (SQL, command, XSS)\n3. Authentication/authorization flaws\n4. Sensitive data exposure\n5. Insecure dependencies\n6. Hardcoded secrets or credentials\n\nProvide a severity-ranked list of findings with remediation steps.", target),
	}
}

// ─── /refactor ────────────────────────────────────

func (h *Handler) refactorCmd(args []string) Result {
	target := strings.Join(args, " ")
	if target == "" {
		return Result{Message: "Usage: /refactor <file or description>\n\nAsk the agent to refactor code with best practices."}
	}
	return Result{
		SkillPrompt: fmt.Sprintf("Refactor %s. Focus on:\n1. Code clarity and readability\n2. DRY principle (remove duplication)\n3. Single responsibility\n4. Better naming\n5. Simplify complex logic\n\nMake the changes, keeping functionality identical.", target),
	}
}

// ─── /summary ─────────────────────────────────────

func (h *Handler) summaryCmd(args []string) Result {
	target := strings.Join(args, " ")
	if target == "" {
		return Result{
			SkillPrompt: "Summarize this project/codebase. Provide:\n1. What the project does\n2. Tech stack and main dependencies\n3. Directory structure overview\n4. Key entry points\n5. How to build/run/test",
		}
	}
	return Result{
		SkillPrompt: fmt.Sprintf("Summarize: %s", target),
	}
}

// ─── /ask ─────────────────────────────────────────

func (h *Handler) askCmd(args []string) Result {
	question := strings.Join(args, " ")
	if question == "" {
		return Result{Message: "Usage: /ask <question>\n\nAsk a question. The agent will answer without using tools."}
	}
	return Result{
		SkillPrompt: fmt.Sprintf("[Answer this question directly without using any tools, just from your knowledge]: %s", question),
	}
}

// ─── /rename ──────────────────────────────────────

func (h *Handler) renameCmd(args []string) Result {
	if len(args) == 0 {
		return Result{Message: "Usage: /rename <title>\n\nGive this session a name for easy identification in /resume."}
	}
	title := strings.Join(args, " ")
	// Store in session - will be saved on next session update
	return Result{
		Message:      fmt.Sprintf("Session renamed to: %s", title),
		SessionTitle: title,
	}
}

// ─── /vim ─────────────────────────────────────────

func (h *Handler) vimCmd(args []string) Result {
	return Result{
		Message:   "Vim mode toggled.",
		VimToggle: true,
	}
}

// ─── /feedback ────────────────────────────────────

func (h *Handler) feedbackCmd(args []string) Result {
	return Result{
		Message: "Report issues at: https://github.com/codeany-ai/codeany/issues\n\nPlease include:\n1. What you expected\n2. What happened\n3. Steps to reproduce\n4. codeany version (run: codeany version)",
	}
}

// ─── /tips ────────────────────────────────────────

func (h *Handler) tipsCmd(args []string) Result {
	tips := []string{
		"Use /fast to quickly switch to a cheaper, faster model",
		"Use /plan to think through complex tasks before executing",
		"Use /commit to let the agent create git commits with good messages",
		"Use Ctrl+O to expand/collapse tool output",
		"Use ! <cmd> to run shell commands inline",
		"Create skills in .codeany/skills/ to teach the agent new abilities",
		"Use /sec for a quick security review of your code",
		"Use /ask for quick questions without tool overhead",
		"Use /copy to copy the last response to your clipboard",
		"Use /export to save the entire conversation to a file",
		"Configure MCP servers in ~/.codeany/settings.json for extra tools",
		"Use /context to see what files and rules the agent is reading",
	}
	// Pick a random tip
	tip := tips[rng.Intn(len(tips))]
	return Result{Message: fmt.Sprintf("Tip: %s\n\nType /tips again for another tip.", tip)}
}

var rng = newRNG()

func newRNG() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// ─── /team ────────────────────────────────────────

func (h *Handler) teamCmd(args []string) Result {
	configDir := config.GlobalConfigDir()

	if len(args) == 0 {
		teams := team.ListTeams(configDir)
		return Result{Message: team.FormatTeamList(teams)}
	}

	switch args[0] {
	case "create":
		if len(args) < 2 {
			return Result{Message: "Usage: /team create <name> [description]"}
		}
		name := args[1]
		desc := strings.Join(args[2:], " ")
		t, err := team.Create(configDir, name, desc)
		if err != nil {
			return Result{Message: fmt.Sprintf("Failed to create team: %v", err)}
		}
		return Result{Message: fmt.Sprintf("✓ Team %q created with lead agent\n  Dir: %s", t.Name, filepath.Join(team.TeamsDir(configDir), name))}

	case "add":
		if len(args) < 3 {
			return Result{Message: "Usage: /team add <team> <agent-name> [type]"}
		}
		t, err := team.Load(configDir, args[1])
		if err != nil {
			return Result{Message: fmt.Sprintf("Team %q not found.", args[1])}
		}
		agentType := "general-purpose"
		if len(args) > 3 {
			agentType = args[3]
		}
		t.AddMember(args[2], agentType, "")
		return Result{Message: fmt.Sprintf("✓ Added %s to team %s", args[2], t.Name)}

	case "delete", "remove":
		if len(args) < 2 {
			return Result{Message: "Usage: /team delete <name>"}
		}
		if err := team.Delete(configDir, args[1]); err != nil {
			return Result{Message: fmt.Sprintf("Failed to delete team: %v", err)}
		}
		return Result{Message: fmt.Sprintf("✓ Team %q deleted", args[1])}

	case "send":
		if len(args) < 4 {
			return Result{Message: "Usage: /team send <team> <agent> <message>"}
		}
		teamName := args[1]
		agentName := args[2]
		msg := strings.Join(args[3:], " ")
		if err := team.SendMsg(configDir, teamName, "user", agentName, msg); err != nil {
			return Result{Message: fmt.Sprintf("Failed to send: %v", err)}
		}
		return Result{Message: fmt.Sprintf("✓ Message sent to %s in team %s", agentName, teamName)}

	case "inbox":
		if len(args) < 3 {
			return Result{Message: "Usage: /team inbox <team> <agent>"}
		}
		messages := team.ReadInbox(configDir, args[1], args[2])
		if len(messages) == 0 {
			return Result{Message: "No unread messages."}
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Inbox for %s (%d messages):\n\n", args[2], len(messages)))
		for _, m := range messages {
			b.WriteString(fmt.Sprintf("  [%s] %s: %s\n", m.Timestamp.Format("15:04"), m.From, m.Text))
		}
		return Result{Message: b.String()}

	default:
		return Result{Message: "Usage: /team [create|add|delete|send|inbox]\n\n/team              List teams\n/team create <n>   Create team\n/team add <t> <a>  Add agent\n/team delete <n>   Delete team\n/team send <t> <a> Send message\n/team inbox <t> <a> Read inbox"}
	}
}

// ─── /worktree ────────────────────────────────────

func (h *Handler) worktreeCmd(args []string) Result {
	configDir := config.GlobalConfigDir()

	if len(args) == 0 {
		wts := worktree.ListAll(configDir)
		if len(wts) == 0 {
			return Result{Message: "No worktrees.\n\nCreate one with: /worktree enter <name>"}
		}
		var b strings.Builder
		b.WriteString("Worktrees:\n\n")
		for _, wt := range wts {
			b.WriteString(fmt.Sprintf("  %s → %s (branch: %s)\n", wt.Name, wt.Path, wt.Branch))
		}
		return Result{Message: b.String()}
	}

	switch args[0] {
	case "enter", "create":
		name := "work"
		if len(args) > 1 {
			name = args[1]
		}
		a := h.app.GetAgent()
		sessionID := "unknown"
		if a != nil {
			sessionID = a.SessionID()
		}
		wt, err := worktree.Create(configDir, name, sessionID)
		if err != nil {
			return Result{Message: fmt.Sprintf("Failed to create worktree: %v", err)}
		}
		if err := wt.Enter(); err != nil {
			return Result{Message: fmt.Sprintf("Failed to enter worktree: %v", err)}
		}
		return Result{Message: fmt.Sprintf("✓ Entered worktree %q\n  Branch: %s\n  Path: %s\n\nUse /worktree exit to return.", name, wt.Branch, wt.Path)}

	case "exit", "leave":
		a := h.app.GetAgent()
		sessionID := "unknown"
		if a != nil {
			sessionID = a.SessionID()
		}
		wt := worktree.LoadActive(configDir, sessionID)
		if wt == nil {
			return Result{Message: "Not in a worktree."}
		}
		remove := false
		if len(args) > 1 && (args[1] == "--remove" || args[1] == "-r") {
			remove = true
		}
		if err := wt.Exit(remove); err != nil {
			return Result{Message: fmt.Sprintf("Failed to exit worktree: %v", err)}
		}
		msg := fmt.Sprintf("✓ Returned to %s", wt.OriginalCWD)
		if remove {
			msg += " (worktree removed)"
		}
		return Result{Message: msg}

	default:
		return Result{Message: "Usage: /worktree [enter|exit]\n\n/worktree             List worktrees\n/worktree enter <n>   Create & enter worktree\n/worktree exit [-r]   Exit (--remove to delete)"}
	}
}

// ─── /effort ──────────────────────────────────────

func (h *Handler) effortCmd(args []string) Result {
	if len(args) == 0 {
		cfg := h.app.GetConfig()
		current := cfg.Effort
		if current == "" {
			current = "(default)"
		}
		return Result{Message: fmt.Sprintf("Current effort: %s\n\nAvailable levels:\n  /effort low      Fast, concise responses\n  /effort medium   Balanced (default)\n  /effort high     Thorough analysis\n  /effort max      Maximum reasoning depth", current)}
	}

	level := strings.ToLower(args[0])
	switch level {
	case "low", "medium", "high", "max":
		cfg := h.app.GetConfig()
		cfg.Effort = level
		return Result{Message: fmt.Sprintf("Effort level set to: %s\n(Takes effect on next query)", level)}
	default:
		return Result{Message: fmt.Sprintf("Unknown effort level: %s\nUse: low, medium, high, max", level)}
	}
}

// ─── /btw ─────────────────────────────────────────

func (h *Handler) btwCmd(args []string) Result {
	if len(args) == 0 {
		return Result{Message: "Usage: /btw <question>\n\nAsk a quick side question. If the agent is busy, it will be queued and answered next."}
	}
	question := strings.Join(args, " ")
	return Result{
		SkillPrompt: fmt.Sprintf("[Side question from user — answer briefly then continue previous task]: %s", question),
	}
}

// ─── /agents ──────────────────────────────────────

func (h *Handler) agentsCmd(args []string) Result {
	return Result{Message: "Available agent types:\n\n  general-purpose  General task handling\n  explore          Fast codebase search\n  plan             Implementation planning\n\nThe agent will automatically spawn subagents as needed via the Agent tool.\nUse /team to manage multi-agent teams."}
}

// ─── /tasks ───────────────────────────────────────

func (h *Handler) tasksCmd(args []string) Result {
	return Result{
		SkillPrompt: "Show the current task list. If no tasks exist, say so. Use TaskList tool.",
	}
}

// ─── /rewind ──────────────────────────────────────

func (h *Handler) rewindCmd(args []string) Result {
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No conversation to rewind."}
	}
	msgs := a.GetMessages()
	if len(msgs) < 2 {
		return Result{Message: "Nothing to rewind."}
	}
	// Clear and note
	a.Clear()
	return Result{
		Message:       fmt.Sprintf("✓ Rewound conversation (removed %d messages). Start fresh.", len(msgs)),
		ClearMessages: true,
	}
}

// ─── /brief ───────────────────────────────────────

func (h *Handler) briefCmd(args []string) Result {
	return Result{
		Message:     "Brief mode toggled.",
		BriefToggle: true,
	}
}

// ─── /share ───────────────────────────────────────

func (h *Handler) shareCmd(args []string) Result {
	// Export to a shareable format
	a := h.app.GetAgent()
	if a == nil {
		return Result{Message: "No conversation to share."}
	}

	home, _ := os.UserHomeDir()
	filename := fmt.Sprintf("codeany-share-%s.md", time.Now().Format("20060102-150405"))
	path := filepath.Join(home, filename)

	var b strings.Builder
	b.WriteString("# Codeany Conversation\n\n")
	b.WriteString(fmt.Sprintf("*Shared on %s*\n\n---\n\n", time.Now().Format("2006-01-02 15:04")))

	for _, msg := range a.GetMessages() {
		switch msg.Role {
		case "user":
			b.WriteString("**User:**\n\n")
		case "assistant":
			b.WriteString("**Assistant:**\n\n")
		}
		for _, block := range msg.Content {
			if block.Text != "" {
				b.WriteString(block.Text + "\n\n")
			}
		}
		b.WriteString("---\n\n")
	}

	os.WriteFile(path, []byte(b.String()), 0644)
	return Result{Message: fmt.Sprintf("✓ Shared to %s", path)}
}

// ─── /env ─────────────────────────────────────────

func (h *Handler) envCmd(args []string) Result {
	var b strings.Builder
	b.WriteString("Environment:\n\n")

	envVars := []string{
		"CODEANY_API_KEY", "ANTHROPIC_API_KEY",
		"CODEANY_BASE_URL", "ANTHROPIC_BASE_URL",
		"CODEANY_MODEL", "ANTHROPIC_MODEL",
		"CODEANY_PROVIDER",
		"CODEANY_CUSTOM_HEADERS",
		"HTTPS_PROXY", "HTTP_PROXY",
		"SHELL", "TERM", "EDITOR",
		"HOME", "USER", "PWD",
	}

	for _, key := range envVars {
		val := os.Getenv(key)
		if val == "" {
			continue
		}
		// Mask API keys
		if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") {
			if len(val) > 12 {
				val = val[:8] + "..." + val[len(val)-4:]
			} else {
				val = "****"
			}
		}
		b.WriteString(fmt.Sprintf("  %-25s %s\n", key, val))
	}

	return Result{Message: b.String()}
}

// ─── /tag ─────────────────────────────────────────

func (h *Handler) tagCmd(args []string) Result {
	if len(args) == 0 {
		return Result{Message: "Usage: /tag <label>\n\nTag the current session for easy finding later."}
	}
	tag := strings.Join(args, " ")
	return Result{
		Message:      fmt.Sprintf("✓ Session tagged: %s", tag),
		SessionTitle: tag,
	}
}

// ─── /add-dir ─────────────────────────────────────

func (h *Handler) addDirCmd(args []string) Result {
	if len(args) == 0 {
		return Result{Message: "Usage: /add-dir <path>\n\nAdd a directory to the agent's working context."}
	}
	dir := args[0]
	return Result{
		SkillPrompt: fmt.Sprintf("The user wants you to also work with files in the directory: %s\nAcknowledge and list its contents.", dir),
	}
}

// ─── /onboarding ──────────────────────────────────

func (h *Handler) onboardingCmd(args []string) Result {
	return Result{
		SkillPrompt: "Walk me through setting up codeany for this project. Check:\n1. Does CODEANY.md or CLAUDE.md exist? If not, help create one.\n2. What's the tech stack? (check for package.json, go.mod, Cargo.toml, etc.)\n3. How to build/test/lint?\n4. Any existing .codeany/ or .claude/ configuration?\n5. Suggest skills or hooks that would be useful.\n\nBe concise and actionable.",
	}
}

// ─── /output-style ────────────────────────────────

func (h *Handler) outputStyleCmd(args []string) Result {
	if len(args) == 0 {
		return Result{Message: "Usage: /output-style <concise|detailed|markdown|plain>\n\nChange how the agent formats responses."}
	}
	style := args[0]
	instructions := ""
	switch strings.ToLower(style) {
	case "concise":
		instructions = "From now on, be extremely concise. Use short sentences. Skip explanations unless asked."
	case "detailed":
		instructions = "From now on, provide detailed explanations with examples and reasoning."
	case "markdown":
		instructions = "From now on, format all responses with rich markdown: headers, code blocks, lists, tables."
	case "plain":
		instructions = "From now on, use plain text only. No markdown formatting, no emojis."
	default:
		return Result{Message: fmt.Sprintf("Unknown style: %s\nUse: concise, detailed, markdown, plain", style)}
	}
	return Result{
		Message:     fmt.Sprintf("Output style: %s", style),
		SkillPrompt: fmt.Sprintf("[System instruction: %s]", instructions),
	}
}

// ─── /passes ──────────────────────────────────────

func (h *Handler) passesCmd(args []string) Result {
	if len(args) == 0 {
		return Result{Message: "Usage: /passes <task>\n\nMulti-pass mode: the agent will make multiple passes over the task,\nrefining its work each time. Good for complex refactoring or review."}
	}
	task := strings.Join(args, " ")
	return Result{
		SkillPrompt: fmt.Sprintf("Perform this task in multiple passes:\n\nTask: %s\n\nPass 1: Initial implementation/analysis\nPass 2: Review and fix issues from pass 1\nPass 3: Polish and verify\n\nClearly label each pass.", task),
	}
}

// ─── /insights ────────────────────────────────────

func (h *Handler) insightsCmd(args []string) Result {
	target := strings.Join(args, " ")
	if target == "" {
		target = "this codebase"
	}
	return Result{
		SkillPrompt: fmt.Sprintf("Analyze %s and provide insights:\n1. Architecture patterns used\n2. Code quality observations\n3. Potential improvements\n4. Dependencies worth updating\n5. Performance considerations\n6. Security observations", target),
	}
}

// ─── /color ───────────────────────────────────────

func (h *Handler) colorCmd(args []string) Result {
	return Result{Message: "Color settings:\n\n  Current: dark (default)\n\n  Use /theme dark or /theme light to switch themes.\n  Custom colors are not yet supported."}
}

// ─── /bughunter ───────────────────────────────────

func (h *Handler) bughunterCmd(args []string) Result {
	target := strings.Join(args, " ")
	if target == "" {
		return Result{Message: "Usage: /bughunter <description or file>\n\nDeep investigation mode: systematically hunt for bugs."}
	}
	return Result{
		SkillPrompt: fmt.Sprintf("Deep bug investigation for: %s\n\n1. Read relevant code thoroughly\n2. Trace the execution path\n3. Check edge cases and error handling\n4. Look for race conditions, null checks, off-by-ones\n5. Verify assumptions in tests\n6. Report ALL bugs found with severity and fix suggestions", target),
	}
}

// ─── /commit-push-pr ──────────────────────────────

func (h *Handler) commitPushPrCmd(args []string) Result {
	desc := strings.Join(args, " ")
	prompt := "Do all of these in sequence:\n1. Review git diff, stage changes, create a commit with good message\n2. Push the branch to remote\n3. Create a pull request using `gh pr create`"
	if desc != "" {
		prompt += fmt.Sprintf("\n\nContext: %s", desc)
	}
	return Result{SkillPrompt: prompt}
}

// ─── /pr-comments ─────────────────────────────────

func (h *Handler) prCommentsCmd(args []string) Result {
	prNum := ""
	if len(args) > 0 {
		prNum = args[0]
	}
	prompt := "Review the comments on the current pull request."
	if prNum != "" {
		prompt = fmt.Sprintf("Review comments on PR #%s.", prNum)
	}
	prompt += "\nSummarize the feedback and suggest responses or code changes."
	return Result{SkillPrompt: prompt}
}

// ─── /keybindings ─────────────────────────────────

func (h *Handler) keybindingsCmd(args []string) Result {
	return Result{Message: "Keybindings:\n\n  Enter            Send message\n  Shift+Enter      New line\n  Ctrl+C           Cancel / Exit\n  Ctrl+D           Exit (empty input)\n  Ctrl+L           Clear conversation\n  Ctrl+O           Toggle expand tool output\n  Up/Down          History / Scroll\n  PgUp/PgDown      Scroll viewport\n  Tab              Complete slash command\n  Esc              Clear input / cancel\n  ! <cmd>          Run shell command\n\nDuring query:\n  Enter            Queue message (btw)\n  j/k              Scroll up/down\n  g/G              Top/bottom\n\nCustom keybindings coming in a future version.\nConfig: ~/.codeany/keybindings.json (not yet supported)"}
}

// ─── /release-notes ───────────────────────────────

func (h *Handler) releaseNotesCmd(args []string) Result {
	return Result{Message: "Codeany Release Notes\n\nv0.8.0 — 66 commands, SDK v0.5.0\n  - /agents /tasks /rewind /brief /share /insights /passes\n  - Turn completion verbs, effort control\n  - 30+ new built-in tools from SDK\n\nv0.7.0 — SDK v0.5.0 upgrade\n  - EnterPlanMode, TeamCreate, Worktree, Cron, LSP tools\n  - Effort levels, fallback model, file checkpointing\n\nv0.6.0 — OpenAI model support\n  - GPT, DeepSeek, Ollama, OpenRouter, custom providers\n  - Interactive /login wizard\n\nv0.5.0 — /btw, plan mode, plugins\nv0.4.0 — Teams, worktrees, subagents\nv0.3.0 — Permissions, hooks, session persistence\n\nFull changelog: https://github.com/codeany-ai/codeany/releases"}
}

// ─── /reload-plugins ──────────────────────────────

func (h *Handler) reloadPluginsCmd(args []string) Result {
	allPlugins := plugins.LoadAll(config.GlobalConfigDir())
	return Result{Message: fmt.Sprintf("✓ Reloaded %d plugins", len(allPlugins))}
}

// ─── /thinkback ───────────────────────────────────

func (h *Handler) thinkbackCmd(args []string) Result {
	return Result{
		SkillPrompt: "Reflect on what was just accomplished in this session:\n1. What tasks were completed?\n2. What approach was taken?\n3. Were there any issues or trade-offs?\n4. What could be improved?\n5. Any follow-up tasks needed?\n\nBe concise and actionable.",
	}
}

// ─── /statusline ──────────────────────────────────

func (h *Handler) statuslineCmd(args []string) Result {
	return Result{Message: "Status line shows:\n  Model · Permission mode · Cost · Tokens ↑↓ · MCP connections · Scroll %\n\nThe header shows: codeany · model · session duration\n\nCustomization not yet available. Coming soon."}
}

// ─── /privacy ─────────────────────────────────────

func (h *Handler) privacyCmd(args []string) Result {
	return Result{Message: "Privacy:\n\n  • Conversations are stored locally in ~/.codeany/sessions/\n  • API calls go directly to your configured provider\n  • No telemetry or analytics are collected\n  • No data is shared with third parties\n  • API keys are stored in ~/.codeany/settings.json (0600 permissions)\n  • Session data never leaves your machine\n\nClear all data: rm -rf ~/.codeany/"}
}

// ─── /issue ───────────────────────────────────────

func (h *Handler) issueCmd(args []string) Result {
	desc := strings.Join(args, " ")
	if desc == "" {
		return Result{Message: "Report issues at: https://github.com/codeany-ai/codeany/issues\n\nOr use: /issue <description> to pre-fill"}
	}
	return Result{
		SkillPrompt: fmt.Sprintf("The user wants to report this issue: %s\n\nHelp them draft a good bug report with:\n1. Steps to reproduce\n2. Expected vs actual behavior\n3. Environment info (run codeany version)\n4. Suggest opening at https://github.com/codeany-ai/codeany/issues", desc),
	}
}

// ─── helpers ──────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
