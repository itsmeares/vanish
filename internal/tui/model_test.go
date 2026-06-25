package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInitialViewContainsVanish(t *testing.T) {
	m := NewModel()

	if !strings.Contains(m.View().Content, "Vanish") {
		t.Fatalf("expected initial view to contain app name")
	}
}

func TestShiftQReturnsQuitCommand(t *testing.T) {
	m := NewModel()

	_, cmd := m.Update(keyPress("Q"))
	if cmd == nil {
		t.Fatalf("expected shift+q to return a command")
	}

	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected shift+q command to return tea.QuitMsg")
	}
}

func TestHomeKeyReturnsHome(t *testing.T) {
	m := NewModel()
	m.current = screen(99)

	updated, cmd := m.Update(keyPress("h"))
	if cmd != nil {
		t.Fatalf("expected home key not to return a command")
	}

	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected updated model to be tui.Model")
	}

	if next.current != screenHome {
		t.Fatalf("expected h to return to home screen")
	}
}

func keyPress(text string) tea.KeyPressMsg {
	key := tea.Key{Text: text}
	if len(text) == 1 {
		key.Code = []rune(strings.ToLower(text))[0]
		key.ShiftedCode = []rune(text)[0]
	}

	return tea.KeyPressMsg(key)
}
