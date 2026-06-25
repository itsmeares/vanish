package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/itsmeares/vanish/internal/instagram"
)

func TestInitialViewContainsVanish(t *testing.T) {
	m := NewModel()

	if !strings.Contains(m.View().Content, "Vanish") {
		t.Fatalf("expected initial view to contain app name")
	}
	if !strings.Contains(m.View().Content, "Import Instagram export ZIP") {
		t.Fatalf("expected initial view to show import action")
	}
	if !strings.Contains(m.View().Content, "Demo import") {
		t.Fatalf("expected initial view to show demo action")
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
	m.current = screenImportResult

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

func TestImportPathScreenAcceptsTypedPath(t *testing.T) {
	m := NewModel()

	updated, _ := m.Update(keyPress("i"))
	next := requireModel(t, updated)
	if next.current != screenImportPath {
		t.Fatalf("expected import path screen, got %v", next.current)
	}

	updated, _ = next.Update(keyPress("a"))
	next = requireModel(t, updated)
	updated, _ = next.Update(keyPress("b"))
	next = requireModel(t, updated)
	updated, _ = next.Update(keyPress("c"))
	next = requireModel(t, updated)

	if next.pathInput.Value() != "abc" {
		t.Fatalf("expected typed path to be captured, got %q", next.pathInput.Value())
	}
}

func TestImportResultViewShowsSummaryCounts(t *testing.T) {
	m := NewModel()
	result := instagram.ImportResult{
		Summary: instagram.ImportSummary{
			Total:     4,
			Likes:     1,
			Comments:  1,
			Following: 1,
			Followers: 1,
			Skipped:   1,
		},
		Warnings: []string{"settings/unknown_shape.json: unsupported Instagram JSON skipped"},
	}

	updated, cmd := m.Update(importFinishedMsg{result: result, source: "demo instagram export"})
	if cmd != nil {
		t.Fatalf("expected result message not to return a command")
	}

	next := requireModel(t, updated)
	view := next.View().Content
	for _, want := range []string{
		"Import Complete",
		"Total parsed items: 4",
		"Likes: 1",
		"Comments: 1",
		"Following: 1",
		"Followers: 1",
		"Skipped or unknown files: 1",
		"Warnings: 1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected result view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestImportResultViewShowsFailure(t *testing.T) {
	m := NewModel()

	updated, _ := m.Update(importFinishedMsg{
		err:    errors.New("open instagram export zip: not found"),
		source: "missing.zip",
	})

	next := requireModel(t, updated)
	view := next.View().Content
	if !strings.Contains(view, "Import Failed") || !strings.Contains(view, "not found") {
		t.Fatalf("expected failure view, got:\n%s", view)
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

func requireModel(t *testing.T, model tea.Model) Model {
	t.Helper()

	next, ok := model.(Model)
	if !ok {
		t.Fatalf("expected updated model to be tui.Model")
	}
	return next
}
