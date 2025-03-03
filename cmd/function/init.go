package function

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/spf13/cobra"
)

var language string

func NewFunctionInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Initialize a new function",
		Args:  cobra.MaximumNArgs(1),
		RunE:  functionInit,
	}
	cmd.Flags().StringVarP(&language, "language", "l", "", "Programming language")

	return cmd
}

func functionInit(cmd *cobra.Command, args []string) error {
	name := args[0]

	if language == "" {
		supportedLanguages := []huh.Option[string]{
			huh.NewOption("JavaScript", "javascript"),
			huh.NewOption("TypeScript", "typescript"),
			huh.NewOption("Golang", "golang"),
			huh.NewOption("Rust", "rust"),
			huh.NewOption("AssemblyScript", "assemblyscript"),
			// huh.NewOption("Zig", "zig"),
			huh.NewOption("Python", "python"),
		}

		baseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ui.InfoColor))
		theme := huh.Theme{
			Focused: huh.FieldStyles{
				Title:          baseStyle.Bold(true),
				SelectedOption: ui.SelectStyle,
				SelectSelector: baseStyle,
			},
		}

		selectLanguage := huh.NewSelect[string]().
			Title("Choose a programming language").
			Options(supportedLanguages...).
			Value(&language)

		form := huh.NewForm(huh.NewGroup(selectLanguage))
		if err := form.WithTheme(&theme).Run(); err != nil {
			return fmt.Errorf("error during language selection: %w", err)
		}
	}

	p := tea.NewProgram(spinner.NewSpinnerModel())

	go func() {
		p.Send("Initializing function...")
		service := services.NewFunctionService()
		err := service.InitFunction(name, language)
		if err != nil {
			p.Send(fmt.Errorf("error initializing function: %w", err))
			return
		}

		p.Send(spinner.ResultMsg{Result: "successfully created function"})
	}()

	// Run the spinner
	m, err := p.Run()
	if err != nil {
		return err
	}

	finalModel, ok := m.(spinner.Model)
	if !ok {
		return fmt.Errorf("unexpected model type returned")
	}

	if !finalModel.HasError() {
		result, ok := finalModel.GetResult().(string)
		if !ok {
			return fmt.Errorf("unexpected result type")
		}
		ui.PrintSuccess(result)
	}

	return nil
}
