package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Primary colors.
	PrimaryColor   = "#7C3AED" // Vibrant purple
	SecondaryColor = "#2563EB" // Deep blue
	TertiaryColor  = "#10B981" // Emerald green

	// Status colors.
	SuccessColor  = "#10B981" // Emerald green
	ErrorColor    = "#EF4444" // Red
	WarningColor  = "#F59E0B" // Amber
	InfoColor     = "#3B82F6" // Blue
	RunningColor  = "#10B981" // Green for running status
	StoppedColor  = "#6B7280" // Gray for stopped status
	PendingColor  = "#F59E0B" // Amber for pending status
	UnloadedColor = "#9CA3AF" // Light gray for unloaded status

	// Text colors.
	HeaderColor  = "#F9FAFB" // Near white
	TextColor    = "#E5E7EB" // Light gray
	DimTextColor = "#9CA3AF" // Dimmed gray
	SubtleColor  = "#6B7280" // Very dim gray
	LinkColor    = "#60A5FA" // Light blue for links/actions
	AccentColor  = "#8B5CF6" // For backward compatibility
	SelectColor  = "#FFFFFF" // For backward compatibility

	// Border and accents.
	BorderColor        = "#374151" // Dark gray border
	HighlightColor     = "#8B5CF6" // Bright purple for highlights
	SelectionColor     = "#1F2937" // Dark blue-gray for selections
	BackgroundAccent   = "#111827" // Near black with blue tint
	AlternatingRowDark = "#1F2937" // Slightly lighter than background
)

// Style definitions.
var (
	// Base styles.
	BaseStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(TextColor)).
			Padding(0, 0, 1, 2)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(HeaderColor)).
			Bold(true)

		// Semantic styles.
	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(SuccessColor))

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(ErrorColor))

	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(WarningColor))

	InfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(InfoColor))

	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(DimTextColor))

	LinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(LinkColor)).
			Underline(true)

		// For backwards compatibility.
	Highlight = lipgloss.NewStyle().
			Foreground(lipgloss.Color(AccentColor)).
			Bold(true)

	SelectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(SelectColor))

		// Component styles.
	TitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(PrimaryColor)).
			Bold(true).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(SecondaryColor)).
			MarginBottom(1)

	SectionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(HeaderColor)).
			Bold(true).
			MarginTop(1).
			MarginBottom(1)

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(BorderColor)).
			Padding(1).
			MarginTop(1).
			MarginBottom(1)

		// Table styles.
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(HeaderColor))

	TableRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(TextColor))

		// Status styles.
	RunningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(RunningColor))

	StoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(StoppedColor))

	PendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(PendingColor))
)

// Terminal width detection (for responsive layouts).
func TerminalWidth() int {
	// Safe default for terminals
	width := 80

	// Try to detect actual width from environment variable
	// Default to 80 columns if detection fails
	return width
}

// Center text on the terminal line.
func CenterText(text string) string {
	width := TerminalWidth()
	fmtWidth := len(text)
	padding := (width - fmtWidth) / 2
	if padding < 0 {
		padding = 0
	}
	return fmt.Sprintf("%s%s", strings.Repeat(" ", padding), text)
}
