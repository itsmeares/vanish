package domain

import "time"

const PlanFormatVersion = 1

// PlatformName is a typed string. It still serializes as plain JSON text, but
// the named type makes function signatures clearer than passing any string.
type PlatformName string

const (
	PlatformInstagram PlatformName = "instagram"
	PlatformReddit    PlatformName = "reddit"
	PlatformX         PlatformName = "x"
	PlatformYouTube   PlatformName = "youtube"
)

// ActivityItemType describes what kind of social activity was discovered.
type ActivityItemType string

const (
	ItemTypeLike    ActivityItemType = "like"
	ItemTypeComment ActivityItemType = "comment"
	ItemTypeFollow  ActivityItemType = "follow"
	ItemTypePost    ActivityItemType = "post"
	ItemTypeSave    ActivityItemType = "save"
	ItemTypeRepost  ActivityItemType = "repost"
)

// ActionType describes the cleanup operation Vanish may perform later.
type ActionType string

const (
	ActionUnlike        ActionType = "unlike"
	ActionDeleteComment ActionType = "delete_comment"
	ActionUnfollow      ActionType = "unfollow"
	ActionDeletePost    ActionType = "delete_post"
	ActionUnsave        ActionType = "unsave"
	ActionUndoRepost    ActionType = "undo_repost"

	ActionRedditDeleteComment ActionType = "reddit_delete_comment"
	ActionRedditDeletePost    ActionType = "reddit_delete_post"
)

// ActionStatus tracks progress for future resume/apply/audit flows.
type ActionStatus string

const (
	ActionStatusPending   ActionStatus = "pending"
	ActionStatusRunning   ActionStatus = "running"
	ActionStatusDone      ActionStatus = "done"
	ActionStatusFailed    ActionStatus = "failed"
	ActionStatusSkipped   ActionStatus = "skipped"
	ActionStatusStopped   ActionStatus = "stopped"
	ActionStatusCancelled ActionStatus = "cancelled"
)

// PlanMode separates today's safe dry-run files from a future apply mode.
type PlanMode string

const (
	PlanModeDryRun PlanMode = "dry-run"
	PlanModeApply  PlanMode = "apply"
)

// SourceMetadata records where an activity item came from without storing
// credentials. JSON tags choose the stable field names used in plan files and
// omit empty optional fields.
type SourceMetadata struct {
	Name       string            `json:"name,omitempty"`
	ImportID   string            `json:"import_id,omitempty"`
	ImportedAt *time.Time        `json:"imported_at,omitempty"`
	FileName   string            `json:"file_name,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// SafeTextReference lets Vanish point at text-like activity without storing the
// complete body. Hash can identify the original text later, while Preview should
// stay short and non-sensitive.
type SafeTextReference struct {
	Hash    string `json:"hash,omitempty"`
	Preview string `json:"preview,omitempty"`
}

// ActivityItem represents one discovered piece of social activity.
//
// A struct is a group of named fields. These fields are intentionally boring:
// IDs, platform names, target references, timestamps, and safe metadata. Vanish
// should not persist passwords, cookies, tokens, session IDs, or raw message
// bodies here.
type ActivityItem struct {
	ID         string             `json:"id"`
	Platform   PlatformName       `json:"platform"`
	Type       ActivityItemType   `json:"type"`
	TargetURL  string             `json:"target_url,omitempty"`
	TargetID   string             `json:"target_id,omitempty"`
	Actor      string             `json:"actor,omitempty"`
	OccurredAt *time.Time         `json:"occurred_at,omitempty"`
	Source     SourceMetadata     `json:"source,omitempty"`
	Metadata   map[string]string  `json:"metadata,omitempty"`
	Text       *SafeTextReference `json:"safe_text,omitempty"`
}

// CleanupAction represents one future cleanup operation selected by the user.
type CleanupAction struct {
	ID                   string            `json:"id"`
	Platform             PlatformName      `json:"platform"`
	Type                 ActionType        `json:"type"`
	TargetURL            string            `json:"target_url,omitempty"`
	TargetID             string            `json:"target_id,omitempty"`
	SourceActivityItemID string            `json:"source_activity_item_id"`
	Status               ActionStatus      `json:"status"`
	CreatedAt            time.Time         `json:"created_at"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

// CleanupPlan is the stable JSON document Vanish can save and load.
type CleanupPlan struct {
	FormatVersion int             `json:"format_version"`
	ID            string          `json:"id"`
	Platform      PlatformName    `json:"platform"`
	CreatedAt     time.Time       `json:"created_at"`
	SourceName    string          `json:"source_name"`
	Mode          PlanMode        `json:"mode"`
	Actions       []CleanupAction `json:"actions"`
}
