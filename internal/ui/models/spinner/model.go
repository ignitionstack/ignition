package spinner

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ignitionstack/ignition/internal/ui"
)

// SpinnerModel represents a reusable spinner component
type SpinnerModel struct {
	spinner spinner.Model
	step    string
	err     error
	done    bool
	result  interface{}
}

// HasError returns whether the spinner has encountered an error
func (m SpinnerModel) HasError() bool {
	return m.err != nil
}

// SetResult sets the result for the spinner
func (m *SpinnerModel) SetResult(result interface{}) {
	m.result = result
}

// GetResult returns the current result
func (m SpinnerModel) GetResult() interface{} {
	return m.result
}

// GetError returns the current error if any
func (m SpinnerModel) GetError() error {
	return m.err
}

// NewSpinnerModel creates a new spinner model with default settings
func NewSpinnerModel() SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.InfoColor))
	return SpinnerModel{
		spinner: s,
		step:    "Starting...",
	}
}

// NewSpinnerModelWithMessage creates a new spinner model with a custom initial message
func NewSpinnerModelWithMessage(message string) SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.InfoColor))
	return SpinnerModel{
		spinner: s,
		step:    message,
	}
}

// Init initializes the spinner model
func (m SpinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
	)
}

// ResultMsg is used to pass the final result
type ResultMsg struct {
	Result interface{}
}

// Update handles messages for the spinner model
func (m SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	case error:
		m.err = msg
		return m, tea.Quit
	case ResultMsg:
		m.result = msg.Result
		m.done = true
		return m, tea.Quit
	case string:
		m.step = msg
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

// View renders the spinner model
func (m SpinnerModel) View() string {
	if m.err != nil {
		return ui.ErrorStyle.Render(fmt.Sprintf("\n█ Error: %v\n", m.err))
	}
	if m.done {
		return ""
	}
	return fmt.Sprintf("%s %s", m.spinner.View(), m.step)
}

// SetDone marks the spinner as complete
func (m *SpinnerModel) SetDone() {
	m.done = true
}
