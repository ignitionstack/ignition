package ui

import "github.com/charmbracelet/lipgloss"

var (
	AccentColor  = "#FF4155" // Coral red
	SuccessColor = "#00F0C9" // Bright cyan
	DimTextColor = "#A1A1B3" // Dimmed text
	ErrorColor   = "#FF4155"
	SelectColor  = "#FFFFFF" // Orange accent
	InfoColor    = "#3B82F6" // Blue info color

	Highlight = lipgloss.NewStyle().
			Foreground(lipgloss.Color(AccentColor)).
			Bold(true)

	BaseStyle = lipgloss.NewStyle().
			Padding(0, 0, 1, 2)

	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ErrorColor))
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(SuccessColor))
	SelectStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(SelectColor))
	InfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(InfoColor))

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#00FF00")).
			Padding(1).
			MarginTop(1)
)
