package domain

import "testing"

func TestActivitySelectionSelectsOneItemByID(t *testing.T) {
	var selection ActivitySelection

	selection.Select("like-1")

	if !selection.Contains("like-1") {
		t.Fatalf("expected selected ID")
	}
	if selection.Len() != 1 {
		t.Fatalf("expected one selected ID, got %d", selection.Len())
	}
}

func TestActivitySelectionDeselectsOneItemByID(t *testing.T) {
	var selection ActivitySelection
	selection.Select("like-1")

	selection.Deselect("like-1")

	if selection.Contains("like-1") {
		t.Fatalf("expected ID to be deselected")
	}
	if selection.Len() != 0 {
		t.Fatalf("expected empty selection, got %d", selection.Len())
	}
}

func TestActivitySelectionTogglesSelection(t *testing.T) {
	var selection ActivitySelection

	if selected := selection.Toggle("like-1"); !selected {
		t.Fatalf("expected toggle to select ID")
	}
	if selected := selection.Toggle("like-1"); selected {
		t.Fatalf("expected second toggle to deselect ID")
	}
	if selection.Len() != 0 {
		t.Fatalf("expected empty selection, got %d", selection.Len())
	}
}

func TestActivitySelectionSelectsAllFilteredItems(t *testing.T) {
	items := filterTestItems()
	filtered := FilterActivityItems(items, ActivityItemFilter{
		IncludeTypes: map[ActivityFilterType]bool{ActivityFilterLike: true},
	})
	var selection ActivitySelection

	selection.SelectItems(filtered)

	if !selection.Contains("like-1") {
		t.Fatalf("expected filtered item to be selected")
	}
	if selection.Contains("comment-1") {
		t.Fatalf("expected unfiltered item not to be selected")
	}
}

func TestActivitySelectionDeselectsAllFilteredItems(t *testing.T) {
	items := filterTestItems()
	var selection ActivitySelection
	selection.SelectItems(items)
	filtered := FilterActivityItems(items, ActivityItemFilter{
		IncludeTypes: map[ActivityFilterType]bool{ActivityFilterLike: true},
	})

	selection.DeselectItems(filtered)

	if selection.Contains("like-1") {
		t.Fatalf("expected filtered item to be deselected")
	}
	if !selection.Contains("comment-1") {
		t.Fatalf("expected unfiltered item to stay selected")
	}
}

func TestActivitySelectionPersistsWhenFiltersChange(t *testing.T) {
	items := filterTestItems()
	var selection ActivitySelection
	selection.Select("like-1")

	comments := FilterActivityItems(items, ActivityItemFilter{
		IncludeTypes: map[ActivityFilterType]bool{ActivityFilterComment: true},
	})
	selection.SelectItems(comments)

	if !selection.Contains("like-1") || !selection.Contains("comment-1") {
		t.Fatalf("expected selection to persist across filtered views")
	}
}

func TestActivitySelectionResetByNewSelection(t *testing.T) {
	var selection ActivitySelection
	selection.Select("like-1")

	selection = ActivitySelection{}

	if selection.Len() != 0 || selection.Contains("like-1") {
		t.Fatalf("expected new selection to reset IDs")
	}
}

func TestActivitySelectionCountsByType(t *testing.T) {
	items := append(filterTestItems(), ActivityItem{
		ID:       "post-1",
		Platform: PlatformReddit,
		Type:     ItemTypePost,
		TargetID: "t3_post",
	})
	var selection ActivitySelection
	selection.SelectItems(items)

	counts := selection.Counts(items)

	if counts.Total != 5 || counts.Likes != 1 || counts.Comments != 1 || counts.Posts != 1 || counts.Following != 1 || counts.Followers != 1 {
		t.Fatalf("unexpected counts: %#v", counts)
	}
}

func TestActivitySelectionUsesIDsNotIndexes(t *testing.T) {
	items := filterTestItems()
	var selection ActivitySelection
	selection.Select(items[0].ID)
	reordered := []ActivityItem{items[1], items[0]}

	selected := selection.SelectedItems(reordered)

	if len(selected) != 1 || selected[0].ID != "like-1" {
		t.Fatalf("expected selected item by ID after reorder, got %#v", selected)
	}
}

func TestActivitySelectionIgnoresBlankIDs(t *testing.T) {
	var selection ActivitySelection

	if selected := selection.Toggle(""); selected {
		t.Fatalf("expected blank toggle not to select")
	}
	selection.Select("")

	if selection.Len() != 0 {
		t.Fatalf("expected blank IDs to be ignored, got %d", selection.Len())
	}
}
