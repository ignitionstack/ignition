package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Standard symbols for consistent appearance
const (
	SuccessSymbol = "✓"
	ErrorSymbol   = "✗"
	InfoSymbol    = "ℹ"
	WarningSymbol = "⚠"
	BulletSymbol  = "•"
)

// PrintSuccess prints a success message with the standard success style
func PrintSuccess(message string) {
	fmt.Println(SuccessStyle.Bold(false).Render(SuccessSymbol + " " + message))
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
		lipgloss.NewStyle().Foreground(lipgloss.Color(InfoColor)).Render(InfoSymbol),
		lipgloss.NewStyle().Foreground(lipgloss.Color(DimTextColor)).Render(label+" "+value))
}

func PrintHighlight(text string) {
	fmt.Printf("%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color(AccentColor)).Render(text))
}

// Table represents a formatted table with headers and rows
type Table struct {
	Headers     []string
	Rows        [][]string
	ColumnWidth []int
}

// NewTable creates a new table with the given headers
func NewTable(headers []string) *Table {
	columnWidth := make([]int, len(headers))
	for i, h := range headers {
		columnWidth[i] = len(h) + 4 // Add some padding
	}
	return &Table{
		Headers:     headers,
		Rows:        [][]string{},
		ColumnWidth: columnWidth,
	}
}

// AddRow adds a new row to the table
func (t *Table) AddRow(values ...string) {
	if len(values) != len(t.Headers) {
		panic(fmt.Sprintf("Row has %d values, expected %d", len(values), len(t.Headers)))
	}

	// Update column widths if necessary
	for i, v := range values {
		if len(v)+4 > t.ColumnWidth[i] {
			t.ColumnWidth[i] = len(v) + 4
		}
	}

	t.Rows = append(t.Rows, values)
}

// AddRowWithStyles adds a row with styled values
func (t *Table) AddRowWithStyles(values []string, styles []lipgloss.Style) []string {
	if len(values) != len(t.Headers) {
		panic(fmt.Sprintf("Row has %d values, expected %d", len(values), len(t.Headers)))
	}

	styledValues := make([]string, len(values))
	for i, v := range values {
		if i < len(styles) {
			styledValues[i] = styles[i].Render(v)
		} else {
			styledValues[i] = v
		}
	}

	t.Rows = append(t.Rows, styledValues)
	return styledValues
}

// StyleStatusValue applies appropriate styling based on status value
func StyleStatusValue(status string) string {
	status = strings.ToLower(status)

	switch status {
	case "running":
		return SuccessStyle.Render(status)
	case "stopped", "error", "failed":
		return ErrorStyle.Render(status)
	case "pending":
		return InfoStyle.Render(status)
	default:
		return status
	}
}

// RenderTable renders the table with consistent styling
func RenderTable(table *Table) string {
	tableHeaderStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(InfoColor)).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(DimTextColor)).
		BorderBottom(true)

	tableRowStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		PaddingLeft(1)

	// Format string for header and rows
	headerFormat := " "
	for i, width := range table.ColumnWidth {
		headerFormat += fmt.Sprintf("%%-%ds", width)
		if i < len(table.ColumnWidth)-1 {
			headerFormat += " "
		}
	}

	// Add header row
	var tableRows []string
	tableRows = append(tableRows, tableHeaderStyle.Render(fmt.Sprintf(headerFormat, toInterfaceSlice(table.Headers)...)))

	// Add data rows
	for _, row := range table.Rows {
		tableRows = append(tableRows, tableRowStyle.Render(fmt.Sprintf(headerFormat, toInterfaceSlice(row)...)))
	}

	// Join rows into a table
	renderedTable := lipgloss.JoinVertical(lipgloss.Left, tableRows...)
	return lipgloss.JoinVertical(lipgloss.Left, "\n", renderedTable, "\n")
}

// Helper to convert string slice to interface slice for fmt.Sprintf
func toInterfaceSlice(ss []string) []interface{} {
	is := make([]interface{}, len(ss))
	for i, s := range ss {
		is[i] = s
	}
	return is
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
		message = SuccessStyle.Render(SuccessSymbol + " Result copied to clipboard!")
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
	fmt.Println(ErrorStyle.Render(fmt.Sprintf(ErrorSymbol+" Error: %s", message)))
}

// PrintWarning prints a warning message with an appropriate style
func PrintWarning(message string) {
	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFA500")). // Orange for warnings
		Bold(true)
	fmt.Println(warningStyle.Render(fmt.Sprintf(WarningSymbol+" Warning: %s", message)))
}

// StyleServiceName applies appropriate styling to service names
func StyleServiceName(serviceName string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(AccentColor)).
		Bold(true).
		Render(serviceName)
}
