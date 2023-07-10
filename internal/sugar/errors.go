package sugar

import (
	tea "github.com/charmbracelet/bubbletea"
)

type ErrorModel interface {
	tea.Model
	GetError() error
}

func RunProgramWithErrors(model ErrorModel) (resultModel tea.Model, err error) {
	resultModel, teaErr := tea.NewProgram(model).Run()
	if errorModel, ok := resultModel.(ErrorModel); ok {
		err = errorModel.GetError()
	}

	// Bubble Tea errors override custom errors
	if teaErr != nil {
		err = teaErr
	}

	return
}
