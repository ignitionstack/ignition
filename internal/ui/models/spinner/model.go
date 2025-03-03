package spinner

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/muesli/reflow/indent"
)

// more concise and user-friendly error messages.
func cleanErrorMessage(errMsg string) string {
	prefixes := []string{
		"Build failed: ",
		"builder initialization failed: ",
		"build failed: ",
		"failed to build function: ",
		"hash calculation failed: ",
		"Error: ",
	}

	for _, prefix := range prefixes {
		if strings.Contains(errMsg, prefix) {
			index := strings.Index(errMsg, prefix)
			if index >= 0 {
				errMsg = errMsg[:index] + errMsg[index+len(prefix):]
			}
		}
	}

	// Capitalize the first letter
	if len(errMsg) > 0 {
		errMsg = strings.ToUpper(errMsg[:1]) + errMsg[1:]
	}

	return errMsg
}

// Model represents an interactive spinner with state.
type Model struct {
	spinner       spinner.Model
	step          string
	steps         []string
	err           error
	done          bool
	result        interface{}
	startTime     time.Time
	showProgress  bool
	progressMax   int
	progressValue int
}

// HasError checks if the spinner has an error.
func (m Model) HasError() bool {
	return m.err != nil
}

// HasResult checks if the spinner has a result.
func (m Model) HasResult() bool {
	return m.result != nil
}

// Note: This should only be used outside of tea.Model Update cycle.
func SetResult(m *Model, result interface{}) {
	m.result = result
}

// GetResult returns the spinner result.
func (m Model) GetResult() interface{} {
	return m.result
}

// GetError returns the spinner error.
func (m Model) GetError() error {
	return m.err
}

// Note: This should only be used outside of tea.Model Update cycle.
func AddStep(m *Model, step string) {
	m.steps = append(m.steps, step)
}

// Note: This should only be used outside of tea.Model Update cycle.
func SetProgress(m *Model, value, progressMax int) {
	m.progressValue = value
	m.progressMax = progressMax
	m.showProgress = true
}

// Note: This should only be used outside of tea.Model Update cycle.
func SetDone(m *Model) {
	m.done = true
}

// NewSpinnerModel creates a default spinner model.
func NewSpinnerModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot // Default spinner style
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.InfoColor))

	return Model{
		spinner:   s,
		step:      "Starting...",
		steps:     []string{},
		startTime: time.Now(),
	}
}

// NewSpinnerModelWithMessage creates a spinner model with a custom initial message.
func NewSpinnerModelWithMessage(message string) Model {
	s := spinner.New()

	// Use line spinner for a more modern feel
	s.Spinner = spinner.Line

	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.PrimaryColor))

	return Model{
		spinner:   s,
		step:      message,
		steps:     []string{message},
		startTime: time.Now(),
	}
}

// Init initializes the spinner model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
	)
}

// Message types for spinner communication.
type ResultMsg struct {
	Result interface{}
}

type ErrorMsg struct {
	Err error
}

type DoneMsg struct {
	Result interface{}
}

type ProgressMsg struct {
	Value int
	Max   int
}

// Update handles spinner state updates.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case error:
		newModel := m
		newModel.err = msg
		newModel.done = true
		return newModel, tea.Sequence(
			tea.Printf("%s", ui.ErrorStyle.Bold(true).Render(fmt.Sprintf("\n%s %s", ui.ErrorSymbol, cleanErrorMessage(msg.Error())))),
			tea.Quit,
		)

	case ErrorMsg:
		newModel := m
		newModel.err = msg.Err
		newModel.done = true
		return newModel, tea.Sequence(
			tea.Printf("%s", ui.ErrorStyle.Bold(true).Render(fmt.Sprintf("\n%s %s", ui.ErrorSymbol, cleanErrorMessage(msg.Err.Error())))),
			tea.Quit,
		)

	case DoneMsg:
		newModel := m
		newModel.result = msg.Result
		newModel.done = true
		duration := time.Since(m.startTime).Round(time.Millisecond)
		return newModel, tea.Sequence(
			tea.Printf("\n%s %s %s",
				ui.SuccessStyle.Bold(true).Render(ui.SuccessSymbol),
				ui.SuccessStyle.Render("Done!"),
				ui.DimStyle.Render(fmt.Sprintf("(%s)", duration))),
			tea.Quit,
		)

	case ResultMsg:
		newModel := m
		newModel.result = msg.Result
		newModel.done = true
		duration := time.Since(m.startTime).Round(time.Millisecond)
		return newModel, tea.Sequence(
			tea.Printf("\n%s %s %s",
				ui.SuccessStyle.Bold(true).Render(ui.SuccessSymbol),
				ui.SuccessStyle.Render("Done!"),
				ui.DimStyle.Render(fmt.Sprintf("(%s)", duration))),
			tea.Quit,
		)

	case ProgressMsg:
		newModel := m
		newModel.progressValue = msg.Value
		newModel.progressMax = msg.Max
		newModel.showProgress = true
		var cmd tea.Cmd
		newModel.spinner, cmd = m.spinner.Update(msg)
		return newModel, cmd

	case string:
		newModel := m
		newModel.step = msg
		newModel.steps = append(newModel.steps, msg)
		var cmd tea.Cmd
		newModel.spinner, cmd = m.spinner.Update(msg)
		return newModel, cmd

	default:
		newModel := m
		var cmd tea.Cmd
		newModel.spinner, cmd = m.spinner.Update(msg)
		return newModel, cmd
	}

	return m, nil
}

// renderProgressBar creates a visual progress bar.
func (m Model) renderProgressBar() string {
	if !m.showProgress || m.progressMax <= 0 {
		return ""
	}

	// Calculate the percentage and bar width
	percent := float64(m.progressValue) / float64(m.progressMax)
	width := 30 // Total progress bar width
	filled := int(percent * float64(width))

	if filled > width {
		filled = width
	}

	// Create the progress bar characters
	bar := "["

	// Using a switch statement instead of if-else chain
	for i := range width {
		switch {
		case i < filled:
			bar += "="
		case i == filled:
			bar += ">"
		default:
			bar += " "
		}
	}
	bar += "]"

	// Add percentage
	bar += fmt.Sprintf(" %d%%", int(percent*100))

	return "\n" + ui.InfoStyle.Render(bar)
}

// renderStepHistory shows recent steps.
func (m Model) renderStepHistory() string {
	// Show only the last few steps
	maxSteps := 3
	if len(m.steps) <= 1 {
		return ""
	}

	var history string
	startIdx := len(m.steps) - maxSteps
	if startIdx < 0 {
		startIdx = 0
	}

	if startIdx > 0 {
		history += ui.DimStyle.Render("...\n")
	}

	for i := startIdx; i < len(m.steps)-1; i++ {
		step := m.steps[i]
		history += ui.DimStyle.Render("  " + ui.CheckmarkSymbol + " " + step + "\n")
	}

	return history
}

// View renders the spinner model.
func (m Model) View() string {
	if m.err != nil || m.done {
		return ""
	}

	// Calculate elapsed time
	elapsed := time.Since(m.startTime).Round(time.Second)
	elapsedStr := ""
	if elapsed > 2*time.Second {
		elapsedStr = ui.DimStyle.Render(fmt.Sprintf(" (%s)", elapsed))
	}

	// Prepare the spinner display
	stepHistory := m.renderStepHistory()
	current := fmt.Sprintf("%s %s%s", m.spinner.View(), m.step, elapsedStr)
	progressBar := m.renderProgressBar()

	// Combine all elements
	output := stepHistory + current
	if progressBar != "" {
		output += progressBar
	}

	// Apply indent for better readability
	return indent.String(output, 2)
}
