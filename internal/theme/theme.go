package theme

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	Primary   = lipgloss.Color("#D4A574") // warm gold/amber
	Secondary = lipgloss.Color("#7AA2F7") // blue
	Success   = lipgloss.Color("#9ECE6A") // green
	Warning   = lipgloss.Color("#E0AF68") // yellow
	Error     = lipgloss.Color("#F7768E") // red
	Muted     = lipgloss.Color("#565F89") // gray
	Text      = lipgloss.Color("#C0CAF5") // light text
	Dim       = lipgloss.Color("#414868") // dim text
	White     = lipgloss.Color("#FFFFFF")

	// Styles
	BoldStyle   = lipgloss.NewStyle().Bold(true)
	MutedStyle  = lipgloss.NewStyle().Foreground(Muted)
	ErrorStyle  = lipgloss.NewStyle().Foreground(Error).Bold(true)
	SuccessText = lipgloss.NewStyle().Foreground(Success)
	WarningText = lipgloss.NewStyle().Foreground(Warning)
	PrimaryText = lipgloss.NewStyle().Foreground(Primary)
	SecondaryText = lipgloss.NewStyle().Foreground(Secondary)
	DimText     = lipgloss.NewStyle().Foreground(Dim)

	// Component styles
	StatusBar = lipgloss.NewStyle().
			Foreground(Muted).
			PaddingLeft(1)

	ToolName = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	UserLabel = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	AssistantLabel = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	PermissionBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Warning).
			Padding(0, 1)

	InputPrompt = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	Spinner = lipgloss.NewStyle().
		Foreground(Primary)
)
