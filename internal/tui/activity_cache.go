package tui

import "github.com/itsmeares/vanish/internal/domain"

type activityItemIndex struct {
	filtered   bool
	indices    []int
	generation uint64
}

func (index *activityItemIndex) reset() {
	index.filtered = false
	index.indices = index.indices[:0]
	index.generation++
}

func (index *activityItemIndex) rebuild(items []domain.ActivityItem, filter domain.ActivityItemFilter) {
	index.generation++
	if !filter.Active() {
		index.filtered = false
		index.indices = index.indices[:0]
		return
	}

	index.filtered = true
	index.indices = index.indices[:0]
	for sourceIndex, item := range items {
		if domain.MatchActivityItem(filter, item) {
			index.indices = append(index.indices, sourceIndex)
		}
	}
}

func (index activityItemIndex) len(items []domain.ActivityItem) int {
	if index.filtered {
		return len(index.indices)
	}
	return len(items)
}

func (index activityItemIndex) sourceIndex(items []domain.ActivityItem, visibleIndex int) (int, bool) {
	if visibleIndex < 0 || visibleIndex >= index.len(items) {
		return 0, false
	}
	if index.filtered {
		return index.indices[visibleIndex], true
	}
	return visibleIndex, true
}

func (index activityItemIndex) item(items []domain.ActivityItem, visibleIndex int) (domain.ActivityItem, bool) {
	sourceIndex, ok := index.sourceIndex(items, visibleIndex)
	if !ok {
		return domain.ActivityItem{}, false
	}
	return items[sourceIndex], true
}

func adjustSelectionCounts(counts *domain.ActivitySelectionCounts, item domain.ActivityItem, delta int) {
	if counts == nil || delta == 0 {
		return
	}
	counts.Total += delta
	switch domain.ActivityFilterTypeForItem(item) {
	case domain.ActivityFilterLike:
		counts.Likes += delta
	case domain.ActivityFilterComment:
		counts.Comments += delta
	case domain.ActivityFilterPost:
		counts.Posts += delta
	case domain.ActivityFilterFollowing:
		counts.Following += delta
	case domain.ActivityFilterFollower:
		counts.Followers += delta
	}
}
