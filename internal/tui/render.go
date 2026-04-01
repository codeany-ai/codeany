package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"github.com/codeany-ai/codeany/internal/theme"
)

var mdRenderer *glamour.TermRenderer

func init() {
	var err error
	mdRenderer, err = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		mdRenderer = nil
	}
}

// ─── Block Renderers ───────────────────────────────

func renderUserBlock(block DisplayBlock, width int) string {
	var b strings.Builder

	// Separator line before user message
	sepStyle := lipgloss.NewStyle().Foreground(theme.Dim)
	b.WriteString(sepStyle.Render("  " + strings.Repeat("─", min(width-4, 60))) + "\n")

	// User prompt with > prefix
	promptStyle := lipgloss.NewStyle().Foreground(theme.Primary).Bold(true)
	b.WriteString("  " + promptStyle.Render(">") + " ")

	content := block.Content
	if len(content) > 10000 {
		head := content[:500]
		tail := content[len(content)-500:]
		lines := strings.Count(content, "\n")
		content = head + fmt.Sprintf("\n  ... +%d lines ...\n  ", lines) + tail
	}

	wrapped := wordwrap.String(content, width-6)
	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		if i == 0 {
			b.WriteString(line + "\n")
		} else {
			b.WriteString("    " + line + "\n")
		}
	}

	return b.String()
}

func renderAssistantBlock(block DisplayBlock, width int) string {
	var b strings.Builder

	if block.Content != "" {
		rendered := renderMarkdown(block.Content, width-4)
		for _, line := range strings.Split(rendered, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	return b.String()
}

func renderSystemBlock(block DisplayBlock) string {
	style := lipgloss.NewStyle().Foreground(theme.Muted).Italic(true)
	lines := strings.Split(block.Content, "\n")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("  " + style.Render(line) + "\n")
	}
	return b.String()
}

// ─── Markdown Renderer ─────────────────────────────

func renderMarkdown(content string, width int) string {
	if mdRenderer != nil {
		rendered, err := mdRenderer.Render(content)
		if err == nil {
			return strings.TrimSpace(rendered)
		}
	}
	return wordwrap.String(content, width)
}

// ─── Tool Input Formatting ─────────────────────────

func formatToolInput(toolName string, input map[string]interface{}) string {
	switch toolName {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if idx := strings.Index(cmd, "\n"); idx > 0 {
				return cmd[:idx] + "..."
			}
			return cmd
		}
	case "Read":
		if fp, ok := input["file_path"].(string); ok {
			s := shortenPath(fp)
			if offset, ok := input["offset"].(float64); ok {
				s += fmt.Sprintf(":%d", int(offset))
			}
			return s
		}
	case "Write":
		if fp, ok := input["file_path"].(string); ok {
			return shortenPath(fp)
		}
	case "Edit":
		if fp, ok := input["file_path"].(string); ok {
			old, _ := input["old_string"].(string)
			if len(old) > 40 {
				old = old[:40] + "..."
			}
			return fmt.Sprintf("%s: %q → ...", shortenPath(fp), old)
		}
	case "Glob":
		if p, ok := input["pattern"].(string); ok {
			path, _ := input["path"].(string)
			if path != "" {
				return p + " in " + shortenPath(path)
			}
			return p
		}
	case "Grep":
		if p, ok := input["pattern"].(string); ok {
			path, _ := input["path"].(string)
			if path != "" {
				return fmt.Sprintf("%q in %s", p, shortenPath(path))
			}
			return fmt.Sprintf("%q", p)
		}
	case "WebFetch":
		if u, ok := input["url"].(string); ok {
			return u
		}
	case "WebSearch":
		if q, ok := input["query"].(string); ok {
			return fmt.Sprintf("%q", q)
		}
	case "Agent":
		desc, _ := input["description"].(string)
		if desc != "" {
			return desc
		}
		prompt, _ := input["prompt"].(string)
		if len(prompt) > 60 {
			prompt = prompt[:60] + "..."
		}
		return prompt
	case "TaskCreate":
		subj, _ := input["subject"].(string)
		return subj
	case "TaskUpdate":
		id, _ := input["taskId"].(string)
		status, _ := input["status"].(string)
		return fmt.Sprintf("#%s → %s", id, status)
	}

	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Sprintf("%v", input)
	}
	s := string(data)
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}

func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
