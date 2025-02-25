package spinner

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ignitionstack/ignition/internal/ui"
)

type SpinnerModel struct {
	spinner spinner.Model
	step    string
	err     error
	done    bool
	result  interface{}
}

func (m SpinnerModel) HasError() bool {
	return m.err != nil
}

func (m *SpinnerModel) SetResult(result interface{}) {
	m.result = result
}

func (m SpinnerModel) GetResult() interface{} {
	return m.result
}

func (m SpinnerModel) GetError() error {
	return m.err
}

func NewSpinnerModel() SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.InfoColor))
	return SpinnerModel{
		spinner: s,
		step:    "Starting...",
	}
}

func NewSpinnerModelWithMessage(message string) SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.InfoColor))
	return SpinnerModel{
		spinner: s,
		step:    message,
	}
}

func (m SpinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
	)
}

type ResultMsg struct {
	Result interface{}
}

type ErrorMsg struct {
	Err error
}

type DoneMsg struct {
	Result interface{}
}

func (m SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	case error:
		m.err = msg
		m.done = true
		return m, tea.Sequence(
			tea.Printf("%s", ui.ErrorStyle.Render(fmt.Sprintf("█ Error: %s", strings.TrimSpace(msg.Error())))),
			tea.Quit,
		)
	case ErrorMsg:
		m.err = msg.Err
		m.done = true
		return m, tea.Sequence(
			tea.Printf("%s", ui.ErrorStyle.Render(fmt.Sprintf("█ Error: %s", strings.TrimSpace(msg.Err.Error())))),
			tea.Quit,
		)
	case DoneMsg:
		m.result = msg.Result
		m.done = true
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

func (m SpinnerModel) View() string {
	if m.err != nil || m.done {
		return ""
	}
	return fmt.Sprintf("%s %s", m.spinner.View(), m.step)
}

func (m *SpinnerModel) SetDone() {
	m.done = true
}
