package instagram

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/platform"
)

type PlanBuildResult struct {
	Plan          domain.CleanupPlan
	SelectedCount int
	Skipped       []PlanBuildSkip
	Counts        PlanActionCounts
	Message       string
}

type PlanBuildSkip struct {
	SourceActivityItemID string
	ItemType             domain.ActivityItemType
	TargetRef            string
	Reason               string
}

type PlanActionCounts struct {
	Unlike        int
	DeleteComment int
	Unfollow      int
}

func BuildCleanupPlan(req platform.BuildPlanRequest) (PlanBuildResult, error) {
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	sourceName := strings.TrimSpace(req.SourceName)
	if sourceName == "" {
		sourceName = "instagram-export"
	}

	actions := make([]domain.CleanupAction, 0, len(req.Items))
	skips := make([]PlanBuildSkip, 0)
	counts := PlanActionCounts{}
	actionIDs := make([]string, 0, len(req.Items))

	for _, item := range req.Items {
		actionType, supported, reason := cleanupActionTypeForItem(item)
		if !supported {
			skips = append(skips, planBuildSkip(item, reason))
			continue
		}

		actionID := stableCleanupActionID(item.Platform, item.ID, actionType)
		action, err := domain.NewCleanupActionFromItem(actionID, item, actionType, createdAt)
		if err != nil {
			skips = append(skips, planBuildSkip(item, err.Error()))
			continue
		}

		actions = append(actions, action)
		actionIDs = append(actionIDs, action.ID)
		switch action.Type {
		case domain.ActionUnlike:
			counts.Unlike++
		case domain.ActionDeleteComment:
			counts.DeleteComment++
		case domain.ActionUnfollow:
			counts.Unfollow++
		}
	}

	plan := domain.NewCleanupPlan(
		stableCleanupPlanID(sourceName, req.Items, actionIDs),
		domain.PlatformInstagram,
		sourceName,
		createdAt,
		actions,
	)

	message := ""
	if len(req.Items) == 0 {
		message = "Select at least one item before generating a plan."
	}

	return PlanBuildResult{
		Plan:          plan,
		SelectedCount: len(req.Items),
		Skipped:       skips,
		Counts:        counts,
		Message:       message,
	}, nil
}

func cleanupActionTypeForItem(item domain.ActivityItem) (domain.ActionType, bool, string) {
	if item.Platform != domain.PlatformInstagram {
		return "", false, fmt.Sprintf("unsupported platform %q", item.Platform)
	}

	switch item.Type {
	case domain.ItemTypeLike:
		return domain.ActionUnlike, true, ""
	case domain.ItemTypeComment:
		return domain.ActionDeleteComment, true, ""
	case domain.ItemTypeFollow:
		switch strings.ToLower(strings.TrimSpace(item.Metadata["relationship"])) {
		case "following":
			return domain.ActionUnfollow, true, ""
		case "follower":
			return "", false, "unsupported follower"
		default:
			return "", false, "follow item has unsupported relationship"
		}
	default:
		return "", false, fmt.Sprintf("unsupported activity type %q", item.Type)
	}
}

func planBuildSkip(item domain.ActivityItem, reason string) PlanBuildSkip {
	return PlanBuildSkip{
		SourceActivityItemID: item.ID,
		ItemType:             item.Type,
		TargetRef:            firstNonEmpty(item.TargetURL, item.TargetID),
		Reason:               reason,
	}
}

func stableCleanupActionID(platform domain.PlatformName, sourceItemID string, actionType domain.ActionType) string {
	return "instagram-action:" + shortStableHash(string(platform), sourceItemID, string(actionType))
}

func stableCleanupPlanID(sourceName string, items []domain.ActivityItem, actionIDs []string) string {
	parts := []string{"instagram-plan", sourceName}
	for _, item := range items {
		parts = append(parts, item.ID)
	}
	parts = append(parts, actionIDs...)
	return "instagram-plan:" + shortStableHash(parts...)
}

func shortStableHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}
