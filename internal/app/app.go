package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/itsmeares/vanish/internal/tui"
)

// Run owns the terminal program setup. Keeping this outside the TUI package
// lets the UI model focus on state, events, and rendering.
func Run() error {
	_, err := tea.NewProgram(tui.NewModel()).Run()
	return err
}
