package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SelectFolder opens a folder selection dialog and returns the selected path
func (a *App) SelectFolder() (string, error) {
	result, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Recordings Folder",
	})
	if err != nil {
		return "", err
	}
	return result, nil
}
