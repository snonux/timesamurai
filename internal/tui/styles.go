package tui

import "github.com/charmbracelet/lipgloss"

// Styles groups visual styles for the root TUI scaffold.
type Styles struct {
	App           lipgloss.Style
	Header        lipgloss.Style
	Tab           lipgloss.Style
	ActiveTab     lipgloss.Style
	Body          lipgloss.Style
	Help          lipgloss.Style
	Hint          lipgloss.Style
	Status        lipgloss.Style
	TableHeader   lipgloss.Style
	TableCell     lipgloss.Style
	TableSelected lipgloss.Style
	SearchMatch   lipgloss.Style
	Error         lipgloss.Style
	Warning       lipgloss.Style
}

// DefaultStyles returns the default style set.
func DefaultStyles() Styles {
	return StylesFromTheme(DefaultTheme())
}

// StylesFromTheme builds styles from a theme palette.
func StylesFromTheme(theme Theme) Styles {
	return Styles{
		App: lipgloss.NewStyle().
			Padding(0, 1),
		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.HeaderFG)).
			Background(lipgloss.Color(theme.StatusBG)).
			Padding(0, 1).
			Bold(true),
		Tab: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color(theme.StatusFG)),
		ActiveTab: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color(theme.SelectedFG)).
			Background(lipgloss.Color(theme.SelectedBG)).
			Bold(true),
		Body: lipgloss.NewStyle().
			PaddingTop(1),
		Help: lipgloss.NewStyle().
			PaddingTop(1).
			Foreground(lipgloss.Color(theme.HeaderFG)),
		Hint: lipgloss.NewStyle().
			PaddingTop(1).
			Foreground(lipgloss.Color("245")),
		Status: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.StatusFG)).
			Background(lipgloss.Color(theme.StatusBG)).
			Padding(0, 1),
		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(theme.HeaderFG)).
			Background(lipgloss.Color(theme.SelectedBG)).
			Padding(0, 1),
		TableCell: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.RowFG)).
			Background(lipgloss.Color(theme.RowBG)).
			Padding(0, 1),
		TableSelected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(theme.SelectedFG)).
			Background(lipgloss.Color(theme.SelectedBG)).
			Padding(0, 1),
		SearchMatch: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.SearchFG)).
			Background(lipgloss.Color(theme.SearchBG)),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true),
	}
}
