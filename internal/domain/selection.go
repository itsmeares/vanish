package domain

// ActivitySelection tracks activity items chosen for a future cleanup plan.
// It stores only stable activity item IDs, never indexes or raw activity data.
type ActivitySelection struct {
	ids map[string]struct{}
}

type ActivitySelectionCounts struct {
	Total     int
	Likes     int
	Comments  int
	Posts     int
	Following int
	Followers int
}

func (selection *ActivitySelection) Toggle(id string) bool {
	if selection.Contains(id) {
		selection.Deselect(id)
		return false
	}
	selection.Select(id)
	return id != ""
}

func (selection *ActivitySelection) Select(id string) {
	if id == "" {
		return
	}
	selection.ensure()
	selection.ids[id] = struct{}{}
}

func (selection *ActivitySelection) Deselect(id string) {
	if selection.ids == nil {
		return
	}
	delete(selection.ids, id)
}

func (selection ActivitySelection) Contains(id string) bool {
	if id == "" || selection.ids == nil {
		return false
	}
	_, ok := selection.ids[id]
	return ok
}

func (selection *ActivitySelection) Clear() {
	selection.ids = nil
}

func (selection ActivitySelection) Len() int {
	return len(selection.ids)
}

func (selection *ActivitySelection) SelectItems(items []ActivityItem) {
	for _, item := range items {
		selection.Select(item.ID)
	}
}

func (selection *ActivitySelection) DeselectItems(items []ActivityItem) {
	for _, item := range items {
		selection.Deselect(item.ID)
	}
}

func (selection ActivitySelection) SelectedItems(items []ActivityItem) []ActivityItem {
	selected := make([]ActivityItem, 0, selection.Len())
	for _, item := range items {
		if selection.Contains(item.ID) {
			selected = append(selected, item)
		}
	}
	return selected
}

func (selection ActivitySelection) Counts(items []ActivityItem) ActivitySelectionCounts {
	var counts ActivitySelectionCounts
	for _, item := range items {
		if !selection.Contains(item.ID) {
			continue
		}
		counts.Total++
		switch ActivityFilterTypeForItem(item) {
		case ActivityFilterLike:
			counts.Likes++
		case ActivityFilterComment:
			counts.Comments++
		case ActivityFilterPost:
			counts.Posts++
		case ActivityFilterFollowing:
			counts.Following++
		case ActivityFilterFollower:
			counts.Followers++
		}
	}
	return counts
}

func (selection *ActivitySelection) ensure() {
	if selection.ids == nil {
		selection.ids = make(map[string]struct{})
	}
}
