package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
)

func TestInitialViewContainsSelectableHomeMenu(t *testing.T) {
	m := NewModel()
	view := m.View().Content

	if !strings.Contains(view, "Vanish") {
		t.Fatalf("expected initial view to contain app name")
	}
	for _, want := range []string{
		"Import Instagram export ZIP",
		"Demo import with fake local data",
		"Quit",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected initial view to show %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{
		"i  Import Instagram export ZIP",
		"d  Demo import with fake local data",
	} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected shortcut row %q to be removed, got:\n%s", unwanted, view)
		}
	}
}

func TestHomeMenuNavigationUsesArrowAndJK(t *testing.T) {
	m := NewModel()

	next := updateModel(t, m, keyPress("down"))
	if next.homeCursor != homeDemo {
		t.Fatalf("expected down to select demo, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.homeCursor != homeQuit {
		t.Fatalf("expected j to select quit, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.homeCursor != homeQuit {
		t.Fatalf("expected cursor to stay at bottom, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("up"))
	if next.homeCursor != homeDemo {
		t.Fatalf("expected up to select demo, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.homeCursor != homeImportZip {
		t.Fatalf("expected k to select import, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.homeCursor != homeImportZip {
		t.Fatalf("expected cursor to stay at top, got %d", next.homeCursor)
	}
}

func TestEnterOnDemoStartsDemoImport(t *testing.T) {
	m := NewModel()
	m.homeCursor = homeDemo

	updated, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected demo import to return a command")
	}

	next := requireModel(t, updated)
	if next.current != screenImporting {
		t.Fatalf("expected demo import to move to importing screen, got %v", next.current)
	}
	if next.importSource != "demo instagram export" {
		t.Fatalf("expected demo source, got %q", next.importSource)
	}
}

func TestQAndCtrlCReturnQuitCommand(t *testing.T) {
	for _, keyName := range []string{"q", "ctrl+c"} {
		m := NewModel()

		_, cmd := m.Update(keyPress(keyName))
		if cmd == nil {
			t.Fatalf("expected %s to return a command", keyName)
		}

		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected %s command to return tea.QuitMsg", keyName)
		}
	}
}

func TestImportPathScreenAcceptsTypedPathAndEscReturnsHome(t *testing.T) {
	m := NewModel()

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenImportPath {
		t.Fatalf("expected import path screen, got %v", next.current)
	}

	for _, keyName := range []string{"a", "b", "c"} {
		next = updateModel(t, next, keyPress(keyName))
	}
	if next.pathInput.Value() != "abc" {
		t.Fatalf("expected typed path to be captured, got %q", next.pathInput.Value())
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenHome {
		t.Fatalf("expected esc to return home, got %v", next.current)
	}
}

func TestImportResultViewShowsSummaryCountsAndActions(t *testing.T) {
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
		"View parsed items",
		"View warnings",
		"Back home",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected result view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestImportResultActionsOpenItemsAndWarnings(t *testing.T) {
	m := importedModel(t, fakeImportResult())

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected enter to open items browser, got %v", next.current)
	}
	itemView := next.View().Content
	for _, want := range []string{
		"Parsed Items",
		"like | demo_artist | https://www.instagram.com/p/demo_like/ | 2024-03-09T16:00:00Z",
		"ID: item-like",
		"Safe text hash: sha256:abcdef",
	} {
		if !strings.Contains(itemView, want) {
			t.Fatalf("expected items browser to contain %q, got:\n%s", want, itemView)
		}
	}
	if strings.Contains(itemView, "raw private comment") {
		t.Fatalf("expected items browser not to render raw text preview, got:\n%s", itemView)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenImportResult {
		t.Fatalf("expected esc to return to import summary, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenWarnings {
		t.Fatalf("expected enter to open warnings, got %v", next.current)
	}
	warningView := next.View().Content
	if !strings.Contains(warningView, "Import Warnings") || !strings.Contains(warningView, "unsupported Instagram JSON skipped") {
		t.Fatalf("expected warnings view, got:\n%s", warningView)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenImportResult {
		t.Fatalf("expected esc to return to import summary, got %v", next.current)
	}
}

func TestItemsBrowserNavigationUsesArrowAndJK(t *testing.T) {
	result := fakeImportResult()
	result.Items = append(result.Items, domain.ActivityItem{
		ID:        "item-follow",
		Platform:  domain.PlatformInstagram,
		Type:      domain.ItemTypeFollow,
		Actor:     "demo_following",
		TargetID:  "demo_following",
		Metadata:  map[string]string{"relationship": "following"},
		Source:    domain.SourceMetadata{FileName: "following.json"},
		TargetURL: "https://www.instagram.com/demo_following/",
	})

	m := importedModel(t, result)
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("down"))
	if next.itemCursor != 1 {
		t.Fatalf("expected down to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.itemCursor != 2 {
		t.Fatalf("expected j to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.itemCursor != 2 {
		t.Fatalf("expected item cursor to stay at bottom, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("up"))
	if next.itemCursor != 1 {
		t.Fatalf("expected up to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.itemCursor != 0 {
		t.Fatalf("expected k to move item cursor, got %d", next.itemCursor)
	}
}

func TestItemsBrowserShowsVisibleAndTotalCount(t *testing.T) {
	m := importedModel(t, fakeImportResult())

	next := updateModel(t, m, keyPress("enter"))
	view := next.View().Content

	if !strings.Contains(view, "2 / 2 parsed items visible") {
		t.Fatalf("expected visible and total count, got:\n%s", view)
	}
}

func TestFiltersScreenOpensFromItemsBrowser(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("f"))
	view := next.View().Content

	if next.current != screenFilters {
		t.Fatalf("expected filters screen, got %v", next.current)
	}
	for _, want := range []string{"Filters", "[ ] Like", "Actor contains", "Apply filters", "Clear all filters"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected filters view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestApplyingTypeFilterUpdatesItemsBrowser(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("f"))

	next = updateModel(t, next, keyPress("enter"))
	for range filterRowApply {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))

	view := next.View().Content
	if next.current != screenItemsBrowser {
		t.Fatalf("expected items browser, got %v", next.current)
	}
	for _, want := range []string{
		"1 / 4 parsed items visible",
		"Filters active",
		"like | demo_artist",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected filtered items view to contain %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "comment | demo_bakery") {
		t.Fatalf("expected comment to be filtered out, got:\n%s", view)
	}
}

func TestInvalidFilterDateShowsFriendlyErrorAndDoesNotApply(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("f"))

	for range filterRowOlder {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	for _, keyName := range []string{"n", "o", "t", "-", "a", "-", "d", "a", "t", "e"} {
		next = updateModel(t, next, keyPress(keyName))
	}
	next = updateModel(t, next, keyPress("enter"))
	for range filterRowApply - filterRowOlder {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))

	view := next.View().Content
	if next.current != screenFilters {
		t.Fatalf("expected filters screen after invalid date, got %v", next.current)
	}
	if next.itemFilter.Active() {
		t.Fatalf("expected invalid date not to apply filter")
	}
	if !strings.Contains(view, "Older than date must use YYYY-MM-DD.") {
		t.Fatalf("expected friendly date error, got:\n%s", view)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected esc to return to items browser, got %v", next.current)
	}
}

func TestWarningsViewShowsEmptyState(t *testing.T) {
	result := fakeImportResult()
	result.Warnings = nil
	m := importedModel(t, result)
	m.resultCursor = resultViewWarnings

	next := updateModel(t, m, keyPress("enter"))
	view := next.View().Content
	if !strings.Contains(view, "No warnings.") {
		t.Fatalf("expected empty warnings state, got:\n%s", view)
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

func fakeImportResult() instagram.ImportResult {
	occurred := time.Date(2024, 3, 9, 16, 0, 0, 0, time.UTC)
	return instagram.ImportResult{
		Items: []domain.ActivityItem{
			{
				ID:         "item-like",
				Platform:   domain.PlatformInstagram,
				Type:       domain.ItemTypeLike,
				Actor:      "demo_artist",
				TargetURL:  "https://www.instagram.com/p/demo_like/",
				TargetID:   "demo_artist",
				OccurredAt: &occurred,
				Source:     domain.SourceMetadata{FileName: "liked_posts.json"},
				Metadata:   map[string]string{"instagram_kind": "liked_post", "username": "demo_artist"},
				Text:       &domain.SafeTextReference{Hash: "sha256:abcdef", Preview: "raw private comment"},
			},
			{
				ID:       "item-comment",
				Platform: domain.PlatformInstagram,
				Type:     domain.ItemTypeComment,
				Actor:    "demo_bakery",
				TargetID: "demo_bakery",
				Source:   domain.SourceMetadata{FileName: "post_comments_1.json"},
				Metadata: map[string]string{"instagram_kind": "comment", "media_owner": "demo_bakery"},
				Text:     &domain.SafeTextReference{Hash: "sha256:123456"},
			},
		},
		Summary: instagram.ImportSummary{
			Total:    2,
			Likes:    1,
			Comments: 1,
			Skipped:  1,
		},
		Warnings: []string{"settings/unknown_shape.json: unsupported Instagram JSON skipped"},
	}
}

func fakeImportResultWithRelationships() instagram.ImportResult {
	result := fakeImportResult()
	followingOccurred := time.Date(2024, 3, 10, 16, 0, 0, 0, time.UTC)
	followerOccurred := time.Date(2024, 3, 11, 16, 0, 0, 0, time.UTC)
	result.Items = append(result.Items,
		domain.ActivityItem{
			ID:         "item-following",
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeFollow,
			Actor:      "demo_following",
			TargetID:   "demo_following",
			TargetURL:  "https://www.instagram.com/demo_following/",
			OccurredAt: &followingOccurred,
			Metadata:   map[string]string{"relationship": "following"},
			Source:     domain.SourceMetadata{FileName: "following.json"},
		},
		domain.ActivityItem{
			ID:         "item-follower",
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeFollow,
			Actor:      "demo_follower",
			TargetID:   "demo_follower",
			TargetURL:  "https://www.instagram.com/demo_follower/",
			OccurredAt: &followerOccurred,
			Metadata:   map[string]string{"relationship": "follower"},
			Source:     domain.SourceMetadata{FileName: "followers_1.json"},
		},
	)
	result.Summary.Total = len(result.Items)
	result.Summary.Following = 1
	result.Summary.Followers = 1
	return result
}

func importedModel(t *testing.T, result instagram.ImportResult) Model {
	t.Helper()

	updated, cmd := NewModel().Update(importFinishedMsg{result: result, source: "demo instagram export"})
	if cmd != nil {
		t.Fatalf("expected import result message not to return command")
	}
	return requireModel(t, updated)
}

func updateModel(t *testing.T, model Model, msg tea.Msg) Model {
	t.Helper()

	updated, _ := model.Update(msg)
	return requireModel(t, updated)
}

func keyPress(text string) tea.KeyPressMsg {
	switch text {
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	case "ctrl+c":
		return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	}

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
