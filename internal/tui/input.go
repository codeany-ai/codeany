package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codeany-ai/codeany/internal/theme"
)

const maxHistory = 100

// InputModel wraps bubbles/textarea for full IME/Chinese support + history
type InputModel struct {
	ta         textarea.Model
	history    []string
	historyIdx int
	historyTmp string // temp store for current input when browsing history
	focused    bool
	width      int
}

func NewInputModel() *InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Enter to send, Shift+Enter for newline)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // unlimited
	ta.MaxHeight = 10
	ta.SetHeight(1)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.Dim)
	ta.FocusedStyle.Text = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Primary).Bold(true)
	ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Dim)
	ta.Prompt = "> "
	ta.Focus()
	ta.SetWidth(80)

	return &InputModel{
		ta:         ta,
		history:    make([]string, 0),
		historyIdx: -1,
		focused:    true,
		width:      80,
	}
}

func (i *InputModel) Focus() {
	i.focused = true
	i.ta.Focus()
}

func (i *InputModel) Blur() {
	i.focused = false
	i.ta.Blur()
}

func (i *InputModel) Value() string {
	return strings.TrimSpace(i.ta.Value())
}

func (i *InputModel) Reset() {
	val := i.Value()
	if val != "" {
		i.history = append(i.history, val)
		if len(i.history) > maxHistory {
			i.history = i.history[1:]
		}
	}
	i.ta.Reset()
	i.ta.SetHeight(1)
	i.historyIdx = -1
	i.historyTmp = ""
}

func (i *InputModel) SetWidth(w int) {
	i.width = w
	i.ta.SetWidth(w - 4) // account for prompt and padding
}

// Update processes a key message, returns true if Enter was pressed (submit)
func (i *InputModel) Update(msg tea.Msg) (bool, tea.Cmd) {
	if !i.focused {
		return false, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Submit on Enter (only if not empty)
			if i.Value() != "" {
				return true, nil
			}
			return false, nil

		case "up":
			// History navigation when textarea is single line and at beginning
			if i.ta.Line() == 0 && i.ta.Value() == "" || (i.historyIdx >= 0) {
				return false, i.navigateHistory(-1)
			}

		case "down":
			// History navigation
			if i.historyIdx >= 0 {
				return false, i.navigateHistory(1)
			}

		case "shift+enter", "alt+enter":
			// Insert newline - let textarea handle it
			i.ta.InsertString("\n")
			// Grow height
			lines := strings.Count(i.ta.Value(), "\n") + 1
			if lines > 10 {
				lines = 10
			}
			if lines < 1 {
				lines = 1
			}
			i.ta.SetHeight(lines)
			return false, nil
		}
	}

	// Let textarea handle everything else (IME, Chinese, paste, etc.)
	var cmd tea.Cmd
	i.ta, cmd = i.ta.Update(msg)

	// Auto-resize height based on content
	lines := strings.Count(i.ta.Value(), "\n") + 1
	if lines > 10 {
		lines = 10
	}
	if lines < 1 {
		lines = 1
	}
	i.ta.SetHeight(lines)

	return false, cmd
}

func (i *InputModel) navigateHistory(direction int) tea.Cmd {
	if len(i.history) == 0 {
		return nil
	}

	if direction < 0 { // up
		if i.historyIdx == -1 {
			i.historyTmp = i.ta.Value()
			i.historyIdx = len(i.history) - 1
		} else if i.historyIdx > 0 {
			i.historyIdx--
		}
		i.ta.Reset()
		i.ta.InsertString(i.history[i.historyIdx])
	} else { // down
		if i.historyIdx >= 0 {
			if i.historyIdx < len(i.history)-1 {
				i.historyIdx++
				i.ta.Reset()
				i.ta.InsertString(i.history[i.historyIdx])
			} else {
				i.historyIdx = -1
				i.ta.Reset()
				i.ta.InsertString(i.historyTmp)
			}
		}
	}
	return nil
}

func (i *InputModel) View() string {
	return "  " + i.ta.View()
}
