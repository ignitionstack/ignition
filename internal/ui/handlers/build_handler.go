package handlers

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/ignitionstack/ignition/pkg/client"
	"github.com/ignitionstack/ignition/pkg/types"
)

type TagInfo struct {
	Namespace string
	Name      string
	Tag       string
}

type BuildHandler struct {
	engineClient client.EngineClient
}

func NewBuildHandler(engineClient client.EngineClient) *BuildHandler {
	return &BuildHandler{
		engineClient: engineClient,
	}
}

func (h *BuildHandler) BuildWithSpinner(
	buildOperation func() (*types.BuildResult, error),
) (*types.BuildResult, error) {
	spinnerModel := spinner.NewSpinnerModelWithMessage("Building...")
	program := tea.NewProgram(spinnerModel)

	go func() {
		buildStart := time.Now()
		result, err := buildOperation()
		if err != nil {
			program.Send(err)
			return
		}

		if result != nil {
			result.BuildTime = time.Since(buildStart)
			program.Send(spinner.ResultMsg{Result: *result})
		}
	}()

	model, err := program.Run()
	if err != nil {
		return nil, err
	}

	finalModel := model.(spinner.SpinnerModel)
	if finalModel.HasError() {
		return nil, finalModel.GetError()
	}

	result := finalModel.GetResult().(types.BuildResult)
	return &result, nil
}

func DisplayBuildResults(result types.BuildResult, tags []TagInfo) {
	ui.PrintSuccess("Function built successfully")
	fmt.Println()

	ui.PrintMetadata("Tags ›", "")
	for _, tag := range tags {
		fmt.Printf("  • %s/%s:%s\n", tag.Namespace, tag.Name, tag.Tag)
	}
	ui.PrintMetadata("Hash ›", result.Digest)
	fmt.Println()
	ui.PrintInfo("Build time", result.BuildTime.Round(time.Millisecond).String())
}