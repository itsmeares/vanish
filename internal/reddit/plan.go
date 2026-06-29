package reddit

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
	DeleteComment int
	DeletePost    int
}

func BuildCleanupPlan(req platform.BuildPlanRequest) (PlanBuildResult, error) {
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	sourceName := strings.TrimSpace(req.SourceName)
	if sourceName == "" {
		sourceName = redditSourceName
	}

	actions := make([]domain.CleanupAction, 0, len(req.Items))
	skips := make([]PlanBuildSkip, 0)
	counts := PlanActionCounts{}
	actionIDs := make([]string, 0, len(req.Items))

	for _, item := range req.Items {
		actionType, supported, reason := redditCleanupActionTypeForItem(item)
		if !supported {
			skips = append(skips, redditPlanBuildSkip(item, reason))
			continue
		}

		actionID := redditStableCleanupActionID(item.ID, actionType)
		action, err := domain.NewCleanupActionFromItem(actionID, item, actionType, createdAt)
		if err != nil {
			skips = append(skips, redditPlanBuildSkip(item, err.Error()))
			continue
		}

		action.Metadata = map[string]string{"reddit_plan_only": "true"}
		if err := action.Validate(); err != nil {
			skips = append(skips, redditPlanBuildSkip(item, err.Error()))
			continue
		}
		actions = append(actions, action)
		actionIDs = append(actionIDs, action.ID)
		switch action.Type {
		case domain.ActionRedditDeleteComment:
			counts.DeleteComment++
		case domain.ActionRedditDeletePost:
			counts.DeletePost++
		}
	}

	plan := domain.NewCleanupPlan(
		redditStableCleanupPlanID(sourceName, req.Items, actionIDs),
		domain.PlatformReddit,
		sourceName,
		createdAt,
		actions,
	)

	message := ""
	if len(req.Items) == 0 {
		message = "Select at least one Reddit item before generating a plan."
	}

	return PlanBuildResult{
		Plan:          plan,
		SelectedCount: len(req.Items),
		Skipped:       skips,
		Counts:        counts,
		Message:       message,
	}, nil
}

func redditCleanupActionTypeForItem(item domain.ActivityItem) (domain.ActionType, bool, string) {
	if item.Platform != domain.PlatformReddit {
		return "", false, fmt.Sprintf("unsupported platform %q", item.Platform)
	}

	switch item.Type {
	case domain.ItemTypeComment:
		return domain.ActionRedditDeleteComment, true, ""
	case domain.ItemTypePost:
		return domain.ActionRedditDeletePost, true, ""
	default:
		return "", false, fmt.Sprintf("unsupported activity type %q", item.Type)
	}
}

func redditPlanBuildSkip(item domain.ActivityItem, reason string) PlanBuildSkip {
	return PlanBuildSkip{
		SourceActivityItemID: item.ID,
		ItemType:             item.Type,
		TargetRef:            firstNonEmpty(item.TargetURL, item.TargetID),
		Reason:               reason,
	}
}

func redditStableCleanupActionID(sourceItemID string, actionType domain.ActionType) string {
	return "reddit-action:" + redditShortStableHash(sourceItemID, string(actionType))
}

func redditStableCleanupPlanID(sourceName string, items []domain.ActivityItem, actionIDs []string) string {
	parts := []string{"reddit-plan", sourceName}
	for _, item := range items {
		parts = append(parts, item.ID)
	}
	parts = append(parts, actionIDs...)
	return "reddit-plan:" + redditShortStableHash(parts...)
}

func redditShortStableHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}
