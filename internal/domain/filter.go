package domain

import (
	"fmt"
	"strings"
	"time"
)

type ActivityFilterType string

const (
	ActivityFilterLike      ActivityFilterType = "like"
	ActivityFilterComment   ActivityFilterType = "comment"
	ActivityFilterFollowing ActivityFilterType = "following"
	ActivityFilterFollower  ActivityFilterType = "follower"
)

type ActivityItemFilter struct {
	IncludeTypes   map[ActivityFilterType]bool
	ActorContains  string
	TargetContains string
	OlderThan      *time.Time
	NewerThan      *time.Time
}

func (filter ActivityItemFilter) Active() bool {
	if strings.TrimSpace(filter.ActorContains) != "" ||
		strings.TrimSpace(filter.TargetContains) != "" ||
		filter.OlderThan != nil ||
		filter.NewerThan != nil {
		return true
	}

	for _, included := range filter.IncludeTypes {
		if included {
			return true
		}
	}

	return false
}

func FilterActivityItems(items []ActivityItem, filter ActivityItemFilter) []ActivityItem {
	if !filter.Active() {
		return append([]ActivityItem(nil), items...)
	}

	filtered := make([]ActivityItem, 0, len(items))
	for _, item := range items {
		if MatchActivityItem(filter, item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func MatchActivityItem(filter ActivityItemFilter, item ActivityItem) bool {
	if hasIncludedTypes(filter.IncludeTypes) && !filter.IncludeTypes[ActivityFilterTypeForItem(item)] {
		return false
	}

	if needle := normalizedContainsValue(filter.ActorContains); needle != "" {
		if !strings.Contains(strings.ToLower(item.Actor), needle) {
			return false
		}
	}

	if needle := normalizedContainsValue(filter.TargetContains); needle != "" {
		targetURL := strings.ToLower(item.TargetURL)
		targetID := strings.ToLower(item.TargetID)
		if !strings.Contains(targetURL, needle) && !strings.Contains(targetID, needle) {
			return false
		}
	}

	if filter.OlderThan != nil {
		if item.OccurredAt == nil || !calendarDay(*item.OccurredAt).Before(calendarDay(*filter.OlderThan)) {
			return false
		}
	}

	if filter.NewerThan != nil {
		if item.OccurredAt == nil || !calendarDay(*item.OccurredAt).After(calendarDay(*filter.NewerThan)) {
			return false
		}
	}

	return true
}

func ActivityFilterTypeForItem(item ActivityItem) ActivityFilterType {
	switch item.Type {
	case ItemTypeLike:
		return ActivityFilterLike
	case ItemTypeComment:
		return ActivityFilterComment
	case ItemTypeFollow:
		if strings.EqualFold(strings.TrimSpace(item.Metadata["relationship"]), "follower") {
			return ActivityFilterFollower
		}
		return ActivityFilterFollowing
	default:
		return ""
	}
}

func ParseFilterDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must use YYYY-MM-DD")
	}
	return calendarDay(parsed), nil
}

func hasIncludedTypes(includeTypes map[ActivityFilterType]bool) bool {
	for _, included := range includeTypes {
		if included {
			return true
		}
	}
	return false
}

func normalizedContainsValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func calendarDay(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}
