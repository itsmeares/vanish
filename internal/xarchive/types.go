package xarchive

import "time"

const DatasetFormatVersion = 1

type ActivityType string

const (
	ActivityPost      ActivityType = "post"
	ActivityReply     ActivityType = "reply"
	ActivityQuotePost ActivityType = "quote_post"
	ActivityRepost    ActivityType = "repost"
)

type MediaKind string

const (
	MediaPhoto     MediaKind = "photo"
	MediaVideo     MediaKind = "video"
	MediaAnimation MediaKind = "animation"
)

type AccountIdentity struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
}

type RelatedPost struct {
	PostID   string `json:"post_id,omitempty"`
	Username string `json:"username,omitempty"`
}

type MediaRef struct {
	Kind           MediaKind `json:"kind"`
	MIME           string    `json:"mime"`
	RelativePath   string    `json:"relative_path"`
	SHA256         string    `json:"sha256"`
	Bytes          int64     `json:"bytes"`
	Width          int       `json:"width,omitempty"`
	Height         int       `json:"height,omitempty"`
	DurationMillis int64     `json:"duration_millis,omitempty"`
	Bitrate        int64     `json:"bitrate,omitempty"`
}

type Activity struct {
	ID           string       `json:"id"`
	SourcePostID string       `json:"source_post_id"`
	SourceKind   string       `json:"source_kind"`
	Type         ActivityType `json:"type"`
	OccurredAt   time.Time    `json:"occurred_at"`
	Text         string       `json:"text"`
	ReplyTo      *RelatedPost `json:"reply_to,omitempty"`
	Quote        *RelatedPost `json:"quote,omitempty"`
	RepostOf     *RelatedPost `json:"repost_of,omitempty"`
	Media        []MediaRef   `json:"media,omitempty"`
}

type ActivityCounts struct {
	Total      int `json:"total"`
	Posts      int `json:"posts"`
	Replies    int `json:"replies"`
	QuotePosts int `json:"quote_posts"`
	Reposts    int `json:"reposts"`
	Media      int `json:"media"`
}

type DatasetManifest struct {
	FormatVersion int             `json:"format_version"`
	DatasetID     string          `json:"dataset_id"`
	Account       AccountIdentity `json:"account"`
	ImportedAt    time.Time       `json:"imported_at"`
	Demo          bool            `json:"demo"`
	Counts        ActivityCounts  `json:"counts"`
	WarningCount  int             `json:"warning_count"`
	PostBytes     int64           `json:"post_bytes"`
	IndexBytes    int64           `json:"index_bytes"`
	IndexSHA256   string          `json:"index_sha256"`
	MediaBytes    int64           `json:"media_bytes"`
}

type DatasetSummary struct {
	DatasetID    string
	Account      AccountIdentity
	ImportedAt   time.Time
	Demo         bool
	Counts       ActivityCounts
	WarningCount int
	StoredBytes  int64
}

type IndexEntry struct {
	ID              string       `json:"id"`
	SourcePostID    string       `json:"source_post_id"`
	Type            ActivityType `json:"type"`
	OccurredAt      time.Time    `json:"occurred_at"`
	RelevantAccount string       `json:"relevant_account,omitempty"`
	Media           int          `json:"media"`
	Offset          int64        `json:"offset"`
	Length          int64        `json:"length"`
	RecordSHA256    string       `json:"record_sha256"`
}

type WarningUnit string

const (
	WarningRecord WarningUnit = "record"
	WarningFile   WarningUnit = "file"
	WarningMedia  WarningUnit = "media"
)

type WarningGroup struct {
	Source   string      `json:"source"`
	Category string      `json:"category"`
	Reason   string      `json:"reason"`
	Unit     WarningUnit `json:"unit"`
	Count    int         `json:"count"`
}

type WarningSummary struct {
	Total  int            `json:"total"`
	Groups []WarningGroup `json:"groups"`
}

type ImportResult struct {
	Dataset  *Dataset
	Summary  DatasetSummary
	Warnings WarningSummary
}
