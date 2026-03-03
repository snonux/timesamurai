package tui

import "github.com/charmbracelet/lipgloss"

// Styles groups visual styles for the root TUI scaffold.
type Styles struct {
	App       lipgloss.Style
	Header    lipgloss.Style
	Tab       lipgloss.Style
	ActiveTab lipgloss.Style
	Body      lipgloss.Style
	Help      lipgloss.Style
	Hint      lipgloss.Style
}

// DefaultStyles returns the default style set.
func DefaultStyles() Styles {
	return Styles{
		App: lipgloss.NewStyle().
			Padding(1, 2),
		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A")).
			Bold(true),
		Tab: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("#B5B5B5")),
		ActiveTab: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("#101010")).
			Background(lipgloss.Color("#8BD3DD")).
			Bold(true),
		Body: lipgloss.NewStyle().
			PaddingTop(1),
		Help: lipgloss.NewStyle().
			PaddingTop(1).
			Foreground(lipgloss.Color("#A0D568")),
		Hint: lipgloss.NewStyle().
			PaddingTop(1).
			Foreground(lipgloss.Color("#777777")),
	}
}
