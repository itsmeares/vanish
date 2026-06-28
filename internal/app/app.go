package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/itsmeares/vanish/internal/tui"
	"github.com/itsmeares/vanish/internal/workspace"
)

// Run owns the terminal program setup. Keeping this outside the TUI package
// lets the UI model focus on state, events, and rendering.
func Run() error {
	localWorkspace, localErr := workspace.OpenDefault()
	_, err := tea.NewProgram(tui.NewModelWithWorkspace(localWorkspace, localErr)).Run()
	return err
}
