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

// SpinnerModel represents an interactive spinner with state
type SpinnerModel struct {
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

// HasError checks if the spinner has an error
func (m SpinnerModel) HasError() bool {
	return m.err != nil
}

// HasResult checks if the spinner has a result
func (m SpinnerModel) HasResult() bool {
	return m.result != nil
}

// SetResult sets the result and marks the spinner as done
func (m *SpinnerModel) SetResult(result interface{}) {
	m.result = result
}

// GetResult returns the spinner result
func (m SpinnerModel) GetResult() interface{} {
	return m.result
}

// GetError returns the spinner error
func (m SpinnerModel) GetError() error {
	return m.err
}

// AddStep adds a step to the history
func (m *SpinnerModel) AddStep(step string) {
	m.steps = append(m.steps, step)
}

// SetProgress sets the progress value for progress indicators
func (m *SpinnerModel) SetProgress(value, max int) {
	m.progressValue = value
	m.progressMax = max
	m.showProgress = true
}

// SetDone marks the spinner as done
func (m *SpinnerModel) SetDone() {
	m.done = true
}

// NewSpinnerModel creates a default spinner model
func NewSpinnerModel() SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot // Default spinner style
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.InfoColor))

	return SpinnerModel{
		spinner:   s,
		step:      "Starting...",
		steps:     []string{},
		startTime: time.Now(),
	}
}

// NewSpinnerModelWithMessage creates a spinner model with a custom initial message
func NewSpinnerModelWithMessage(message string) SpinnerModel {
	s := spinner.New()

	// Use line spinner for a more modern feel
	s.Spinner = spinner.Line

	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.PrimaryColor))

	return SpinnerModel{
		spinner:   s,
		step:      message,
		steps:     []string{message},
		startTime: time.Now(),
	}
}

// Init initializes the spinner model
func (m SpinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
	)
}

// Message types for spinner communication
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

// Update handles spinner state updates
func (m SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case error:
		m.err = msg
		m.done = true
		return m, tea.Sequence(
			tea.Printf("%s", ui.ErrorStyle.Bold(true).Render(fmt.Sprintf("\n%s Error: %s", ui.ErrorSymbol, strings.TrimSpace(msg.Error())))),
			tea.Quit,
		)

	case ErrorMsg:
		m.err = msg.Err
		m.done = true
		return m, tea.Sequence(
			tea.Printf("%s", ui.ErrorStyle.Bold(true).Render(fmt.Sprintf("\n%s Error: %s", ui.ErrorSymbol, strings.TrimSpace(msg.Err.Error())))),
			tea.Quit,
		)

	case DoneMsg:
		m.result = msg.Result
		m.done = true
		duration := time.Since(m.startTime).Round(time.Millisecond)
		return m, tea.Sequence(
			tea.Printf("\n%s %s %s",
				ui.SuccessStyle.Bold(true).Render(ui.SuccessSymbol),
				ui.SuccessStyle.Render("Done!"),
				ui.DimStyle.Render(fmt.Sprintf("(%s)", duration))),
			tea.Quit,
		)

	case ResultMsg:
		m.result = msg.Result
		m.done = true
		duration := time.Since(m.startTime).Round(time.Millisecond)
		return m, tea.Sequence(
			tea.Printf("\n%s %s %s",
				ui.SuccessStyle.Bold(true).Render(ui.SuccessSymbol),
				ui.SuccessStyle.Render("Done!"),
				ui.DimStyle.Render(fmt.Sprintf("(%s)", duration))),
			tea.Quit,
		)

	case ProgressMsg:
		m.progressValue = msg.Value
		m.progressMax = msg.Max
		m.showProgress = true
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case string:
		m.step = msg
		m.AddStep(msg)
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// renderProgressBar creates a visual progress bar
func (m SpinnerModel) renderProgressBar() string {
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
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "="
		} else if i == filled {
			bar += ">"
		} else {
			bar += " "
		}
	}
	bar += "]"

	// Add percentage
	bar += fmt.Sprintf(" %d%%", int(percent*100))

	return "\n" + ui.InfoStyle.Render(bar)
}

// renderStepHistory shows recent steps
func (m SpinnerModel) renderStepHistory() string {
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

// View renders the spinner model
func (m SpinnerModel) View() string {
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
