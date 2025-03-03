package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Enhanced symbols with consistent appearance.
const (
	SuccessSymbol    = "✓"
	ErrorSymbol      = "✗"
	InfoSymbol       = "ℹ"
	WarningSymbol    = "⚠"
	BulletSymbol     = "•"
	ArrowRightSymbol = "→"
	ArrowLeftSymbol  = "←"
	CheckmarkSymbol  = "✓"
	StartSymbol      = "○"
	EndSymbol        = "●"
	LoadingDots      = "..."
)

// Command prompt symbol.
const CommandPrompt = "❯"

// PrintLogo prints the Ignition logo banner.
func PrintLogo() {
	width := TerminalWidth()
	if width < 80 {
		// Use compact logo for smaller terminals
		fmt.Println(TitleStyle.Render("Ignition CLI"))
		return
	}

	// Multi-line styled logo for larger terminals
	logo := `█ █▀▀ █▄░█ █ ▀█▀ █ █▀█ █▄░█
█ █▄█ █░▀█ █ ░█░ █ █▄█ █░▀█`

	// Apply gradient colors to the logo
	lines := strings.Split(logo, "\n")
	colors := []string{SecondaryColor, InfoColor}

	for i, line := range lines {
		if len(line) > 0 {
			colorIdx := i % len(colors)
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color(colors[colorIdx])).Render(line))
		} else {
			fmt.Println()
		}
	}

	// Print subtitle
	subtitle := "\nWebAssembly Function Platform"
	fmt.Println(CenterText(SubtitleStyle.Render(subtitle)))
}

// PrintSuccess prints a success message with enhanced styling.
func PrintSuccess(message string) {
	// Create a success box for important messages
	fmt.Println(lipgloss.NewStyle().
		Foreground(lipgloss.Color(SuccessColor)).
		Bold(true).
		Render(SuccessSymbol + " " + message))
}

// PrintError prints an error message with enhanced styling.
func PrintError(message string) {
	// Add padding and make errors more visible with box styling
	errorBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ErrorColor)).
		Padding(0, 1).
		Render(ErrorStyle.Bold(true).Render(ErrorSymbol + " Error: " + message))

	fmt.Println(errorBox)
}

// PrintWarning prints a warning message with enhanced styling.
func PrintWarning(message string) {
	fmt.Println(WarningStyle.Bold(true).Render(WarningSymbol + " " + message))
}

// PrintInfo prints an info message with label and value in a cleaner format.
func PrintInfo(label, value string) {
	labelStyle := DimStyle.Bold(true)
	fmt.Printf("%s %s\n",
		labelStyle.Render(label+":"),
		InfoStyle.Render(value))
}

// PrintMetadata prints metadata with styled label and value.
func PrintMetadata(label, value string) {
	if value == "" {
		fmt.Printf("%s %s\n",
			InfoStyle.Render(InfoSymbol),
			DimStyle.Bold(true).Render(label))
	} else {
		fmt.Printf("%s %s %s\n",
			InfoStyle.Render(InfoSymbol),
			DimStyle.Bold(true).Render(label),
			InfoStyle.Render(value))
	}
}

// PrintStep prints a step in a multi-step process.
func PrintStep(stepNumber int, totalSteps int, description string) {
	progress := fmt.Sprintf("[%d/%d]", stepNumber, totalSteps)
	fmt.Printf("%s %s %s\n",
		InfoStyle.Render(progress),
		TitleStyle.Render(ArrowRightSymbol),
		HeaderStyle.Render(description))
}

// PrintHighlight prints highlighted text.
func PrintHighlight(text string) {
	fmt.Println(TitleStyle.Render(text))
}

// PrintJSON prints formatted and syntax-highlighted JSON.
func PrintJSON(jsonStr string) {
	// Use box styling to make JSON output stand out
	jsonBox := BoxStyle.
		BorderForeground(lipgloss.Color(InfoColor)).
		Render(HighlightJSON(jsonStr))

	fmt.Println(jsonBox)

	// Add helpful message about copying
	fmt.Println(DimStyle.Render("Tip: Use 'command | jq' to process this output"))
}

// PrintCommand shows a command that could be run.
func PrintCommand(command string) {
	prompt := DimStyle.Render(CommandPrompt + " ")
	fmt.Printf("%s%s\n", prompt, LinkStyle.Render(command))
}

// PrintTimestamp shows when an operation completed.
func PrintTimestamp(operation string, duration time.Duration) {
	timestamp := time.Now().Format("15:04:05")
	durationStr := ""
	if duration > 0 {
		durationStr = fmt.Sprintf(" (took %s)", duration.Round(time.Millisecond))
	}

	fmt.Printf("%s %s%s\n",
		DimStyle.Render(timestamp),
		operation,
		DimStyle.Render(durationStr))
}

// PrintSeparator prints a horizontal separator line.
func PrintSeparator() {
	width := TerminalWidth()
	if width <= 0 {
		width = 80
	}

	separator := strings.Repeat("─", width)
	fmt.Println(DimStyle.Render(separator))
}

// Table represents a formatted table with headers and rows.
type Table struct {
	Headers     []string
	Rows        [][]string
	ColumnWidth []int
}

// NewTable creates a new table with the given headers.
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

// AddRow adds a new row to the table.
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

// AddRowWithStyles adds a row with styled values.
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

// StyleServiceName styles a service name for log output.
func StyleServiceName(serviceName string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(InfoColor)).
		Bold(true).
		Render(serviceName)
}

// PrintServiceLog prints a log line with a styled service name prefix.
func PrintServiceLog(serviceName, logLine string) {
	fmt.Printf("%s | %s\n", StyleServiceName(serviceName), logLine)
}

// StyleStatusValue applies appropriate styling based on status value.
func StyleStatusValue(status string) string {
	status = strings.ToLower(status)

	switch status {
	case "running":
		return RunningStyle.Render(SuccessSymbol + " " + status)
	case "error", "failed":
		return ErrorStyle.Render(ErrorSymbol + " " + status)
	case "pending":
		return PendingStyle.Render("⋯ " + status)
	case "unloaded":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(UnloadedColor)).Render("◌ " + status)
	case "stopped":
		return StoppedStyle.Render("⊘ " + status)
	default:
		return status
	}
}

// RenderTable renders the table with enhanced styling.
func RenderTable(table *Table) string {
	// Calculate total width for responsive sizing
	totalWidth := 0
	for _, width := range table.ColumnWidth {
		totalWidth += width
	}

	// Adjust for terminal width
	termWidth := TerminalWidth()
	if totalWidth > termWidth && termWidth > 40 {
		// Scale down column widths proportionally
		scale := float64(termWidth-10) / float64(totalWidth)
		for i := range table.ColumnWidth {
			table.ColumnWidth[i] = int(float64(table.ColumnWidth[i]) * scale)
			if table.ColumnWidth[i] < 10 {
				table.ColumnWidth[i] = 10
			}
		}
	}

	// Format string for header and rows with no leading space
	headerFormat := ""
	for i, width := range table.ColumnWidth {
		headerFormat += fmt.Sprintf("%%-%ds", width)
		if i < len(table.ColumnWidth)-1 {
			headerFormat += " "
		}
	}

	// Add header row with enhanced styling
	var tableRows []string
	tableRows = append(tableRows, TableHeaderStyle.Render(fmt.Sprintf(headerFormat, toInterfaceSlice(table.Headers)...)))

	// Calculate the actual width for the separator
	headerText := fmt.Sprintf(headerFormat, toInterfaceSlice(table.Headers)...)
	separatorWidth := len(headerText)
	separator := strings.Repeat("─", separatorWidth)
	tableRows = append(tableRows, DimStyle.Render(separator))

	// Add data rows with alternating styles for better readability
	for i, row := range table.Rows {
		style := TableRowStyle
		if i%2 == 1 {
			// Apply subtle alternating row coloring
			style = style.Background(lipgloss.Color(AlternatingRowDark))
		}
		tableRows = append(tableRows, style.Render(fmt.Sprintf(headerFormat, toInterfaceSlice(row)...)))
	}

	// Join rows into a table without a border
	renderedTable := lipgloss.JoinVertical(lipgloss.Left, tableRows...)

	// Removed record count caption for cleaner output

	// Add vertical spacing around the table by wrapping in empty lines
	return fmt.Sprintf("\n%s\n", renderedTable)
}

// Helper to convert string slice to interface slice for fmt.Sprintf.
func toInterfaceSlice(ss []string) []interface{} {
	is := make([]interface{}, len(ss))
	for i, s := range ss {
		is[i] = s
	}
	return is
}

// ResultDisplayModel handles displaying JSON results with copy functionality.
type ResultDisplayModel struct {
	resultJSON string
	copied     bool
	quit       bool
}

// NewResultDisplayModel creates a new result display model.
func NewResultDisplayModel(resultJSON string) ResultDisplayModel {
	return ResultDisplayModel{resultJSON: resultJSON}
}

// Init initializes the result display model.
func (m ResultDisplayModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the result display model.
func (m ResultDisplayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
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

// View renders the result display model with enhanced styling.
func (m ResultDisplayModel) View() string {
	var message string
	if m.copied {
		message = SuccessStyle.Render(SuccessSymbol + " Copied to clipboard!")
	} else {
		message = InfoStyle.Render("Press 'c' to copy, 'q' to quit")
	}

	highlightedJSON := HighlightJSON(m.resultJSON)

	// Add a border around the JSON for better visibility
	jsonWithBorder := BoxStyle.
		BorderForeground(lipgloss.Color(InfoColor)).
		Render(highlightedJSON)

	return fmt.Sprintf("%s\n\n%s", jsonWithBorder, message)
}

// HighlightJSON formats and highlights JSON string.
func HighlightJSON(jsonStr string) string {
	var builder strings.Builder
	err := quick.Highlight(&builder, jsonStr, "json", "terminal", "monokai")
	if err != nil {
		return fmt.Sprintf("Error rendering JSON: %v", err)
	}
	return builder.String()
}

// Note: StyleServiceName is defined earlier in this file

// PrintEmptyState shows a message when no data is available.
func PrintEmptyState(message string) {
	box := BoxStyle.
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(DimTextColor)).
		Align(lipgloss.Center).
		Width(40).
		Render(DimStyle.Render(message))

	fmt.Println(CenterText(box))
}
