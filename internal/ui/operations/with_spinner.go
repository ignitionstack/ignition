package operations

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
)

type OperationFunc func() (interface{}, error)

type DisplayFunc func(result interface{})

func WithSpinner(message string, operation OperationFunc, display DisplayFunc) error {
	spinnerModel := spinner.NewSpinnerModelWithMessage(message)
	program := tea.NewProgram(spinnerModel)

	go func() {
		startTime := time.Now()
		result, err := operation()

		if err != nil {
			program.Send(err)
			return
		}

		program.Send(spinner.ResultMsg{
			Result: struct {
				Data          interface{}
				ExecutionTime time.Duration
			}{
				Data:          result,
				ExecutionTime: time.Since(startTime),
			},
		})
	}()

	model, err := program.Run()
	if err != nil {
		return err
	}

	finalModel, ok := model.(spinner.SpinnerModel)
	if !ok {
		return fmt.Errorf("program finished with invalid model")
	}
	
	if finalModel.HasError() {
		return finalModel.GetError()
	}

	if display != nil && finalModel.HasResult() {
		resultData := finalModel.GetResult()
		display(resultData)
	}

	return nil
}
