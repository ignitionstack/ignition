package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PrintSuccess prints a success message with the standard success style
func PrintSuccess(message string) {
	fmt.Println(SuccessStyle.Bold(false).Render("░ " + message))
}

// PrintInfo prints an info message with label and value
func PrintInfo(label, value string) {
	fmt.Printf("%s %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color(DimTextColor)).Render(label+":"),
		InfoStyle.Render(value))
}

// PrintMetadata prints metadata with an accent prefix
func PrintMetadata(label, value string) {
	fmt.Printf("%s %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color(InfoColor)).Render("░"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(DimTextColor)).Render(label+" "+value))
}

func PrintHighlight(text string) {
	fmt.Printf("%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color(AccentColor)).Render(text))

}

// ResultDisplayModel handles displaying JSON results with copy functionality
type ResultDisplayModel struct {
	resultJSON string
	copied     bool
	quit       bool
}

// NewResultDisplayModel creates a new result display model
func NewResultDisplayModel(resultJSON string) ResultDisplayModel {
	return ResultDisplayModel{resultJSON: resultJSON}
}

// Init initializes the result display model
func (m ResultDisplayModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the result display model
func (m ResultDisplayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			m.quit = true
			return m, tea.Quit
		case "c":
			err := clipboard.WriteAll(m.resultJSON)
			m.copied = err == nil
		}
	}
	return m, nil
}

// View renders the result display model
func (m ResultDisplayModel) View() string {
	var message string
	if m.copied {
		message = SuccessStyle.Render("░ Result copied to clipboard!")
	} else {
		message = lipgloss.NewStyle().Italic(true).Render("Press 'c' to copy, 'q' to quit.")
	}

	highlightedJSON := HighlightJSON(m.resultJSON)
	return fmt.Sprintf("%s\n\n%s", highlightedJSON, message)
}

// HighlightJSON formats and highlights JSON string
func HighlightJSON(jsonStr string) string {
	var builder strings.Builder
	err := quick.Highlight(&builder, jsonStr, "json", "terminal", "monokai")
	if err != nil {
		return fmt.Sprintf("Error rendering JSON: %v", err)
	}
	return builder.String()
}

// PrintError prints an error message with the standard error style
func PrintError(message string) {
	fmt.Println(ErrorStyle.Render(fmt.Sprintf("█ Error: %s", message)))
}
