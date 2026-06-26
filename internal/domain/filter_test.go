package domain

import (
	"testing"
	"time"
)

func TestFilterActivityItemsNoFiltersReturnsAllItems(t *testing.T) {
	items := filterTestItems()

	filtered := FilterActivityItems(items, ActivityItemFilter{})

	if len(filtered) != len(items) {
		t.Fatalf("expected all items, got %d", len(filtered))
	}
}

func TestFilterActivityItemsByOneType(t *testing.T) {
	filtered := FilterActivityItems(filterTestItems(), ActivityItemFilter{
		IncludeTypes: map[ActivityFilterType]bool{ActivityFilterLike: true},
	})

	requireIDs(t, filtered, "like-1")
}

func TestFilterActivityItemsByMultipleTypes(t *testing.T) {
	filtered := FilterActivityItems(filterTestItems(), ActivityItemFilter{
		IncludeTypes: map[ActivityFilterType]bool{
			ActivityFilterComment:  true,
			ActivityFilterFollower: true,
		},
	})

	requireIDs(t, filtered, "comment-1", "follower-1")
}

func TestFilterActivityItemsActorContainsCaseInsensitive(t *testing.T) {
	filtered := FilterActivityItems(filterTestItems(), ActivityItemFilter{ActorContains: "ARTIST"})

	requireIDs(t, filtered, "like-1")
}

func TestFilterActivityItemsTargetContainsCaseInsensitive(t *testing.T) {
	items := filterTestItems()

	filteredByURL := FilterActivityItems(items, ActivityItemFilter{TargetContains: "POST/ONE"})
	requireIDs(t, filteredByURL, "like-1")

	filteredByID := FilterActivityItems(items, ActivityItemFilter{TargetContains: "OWNER"})
	requireIDs(t, filteredByID, "comment-1")
}

func TestFilterActivityItemsOlderThanDate(t *testing.T) {
	olderThan := mustFilterDate(t, "2024-03-10")

	filtered := FilterActivityItems(filterTestItems(), ActivityItemFilter{OlderThan: &olderThan})

	requireIDs(t, filtered, "like-1")
}

func TestFilterActivityItemsNewerThanDate(t *testing.T) {
	newerThan := mustFilterDate(t, "2024-03-10")

	filtered := FilterActivityItems(filterTestItems(), ActivityItemFilter{NewerThan: &newerThan})

	requireIDs(t, filtered, "following-1", "follower-1")
}

func TestFilterActivityItemsCombinedFilters(t *testing.T) {
	olderThan := mustFilterDate(t, "2024-03-12")
	filtered := FilterActivityItems(filterTestItems(), ActivityItemFilter{
		IncludeTypes:  map[ActivityFilterType]bool{ActivityFilterFollowing: true},
		ActorContains: "follow",
		OlderThan:     &olderThan,
	})

	requireIDs(t, filtered, "following-1")
}

func TestFilterActivityItemsEmptyResult(t *testing.T) {
	filtered := FilterActivityItems(filterTestItems(), ActivityItemFilter{ActorContains: "nobody"})

	if len(filtered) != 0 {
		t.Fatalf("expected empty result, got %#v", filtered)
	}
}

func TestFilterActivityItemsMissingFieldsDoNotCrash(t *testing.T) {
	items := append(filterTestItems(), ActivityItem{
		ID:       "missing-fields",
		Platform: PlatformInstagram,
		Type:     ItemTypeLike,
	})
	olderThan := mustFilterDate(t, "2024-03-10")

	filtered := FilterActivityItems(items, ActivityItemFilter{
		ActorContains:  "demo",
		TargetContains: "instagram",
		OlderThan:      &olderThan,
	})

	requireIDs(t, filtered, "like-1")
}

func filterTestItems() []ActivityItem {
	likeDate := time.Date(2024, 3, 9, 16, 0, 0, 0, time.UTC)
	commentDate := time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)
	followingDate := time.Date(2024, 3, 11, 8, 0, 0, 0, time.UTC)
	followerDate := time.Date(2024, 3, 12, 8, 0, 0, 0, time.UTC)

	return []ActivityItem{
		{
			ID:         "like-1",
			Platform:   PlatformInstagram,
			Type:       ItemTypeLike,
			Actor:      "Demo_Artist",
			TargetURL:  "https://www.instagram.com/post/one/",
			TargetID:   "post-one",
			OccurredAt: &likeDate,
		},
		{
			ID:         "comment-1",
			Platform:   PlatformInstagram,
			Type:       ItemTypeComment,
			Actor:      "demo_bakery",
			TargetID:   "MediaOwner42",
			OccurredAt: &commentDate,
		},
		{
			ID:         "following-1",
			Platform:   PlatformInstagram,
			Type:       ItemTypeFollow,
			Actor:      "demo_following",
			TargetID:   "demo_following",
			OccurredAt: &followingDate,
			Metadata:   map[string]string{"relationship": "following"},
		},
		{
			ID:         "follower-1",
			Platform:   PlatformInstagram,
			Type:       ItemTypeFollow,
			Actor:      "demo_follower",
			TargetID:   "demo_follower",
			OccurredAt: &followerDate,
			Metadata:   map[string]string{"relationship": "follower"},
		},
	}
}

func mustFilterDate(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := ParseFilterDate(value)
	if err != nil {
		t.Fatalf("parse filter date %q: %v", value, err)
	}
	return parsed
}

func requireIDs(t *testing.T, items []ActivityItem, ids ...string) {
	t.Helper()

	if len(items) != len(ids) {
		t.Fatalf("expected ids %v, got %#v", ids, items)
	}
	for i, id := range ids {
		if items[i].ID != id {
			t.Fatalf("expected id %q at %d, got %q", id, i, items[i].ID)
		}
	}
}
