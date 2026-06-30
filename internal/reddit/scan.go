package reddit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

const (
	DefaultScanLimit    = MaxListingSize
	DefaultScanMaxPages = 20
	DefaultTextPreview  = 120

	redditSourceName = "reddit-api"
)

type ScanOptions struct {
	IncludeComments bool
	IncludePosts    bool
	Limit           int
	MaxPages        int
	MaxItems        int
	Now             func() time.Time
}

type ScanResult struct {
	Items    []domain.ActivityItem
	Summary  ScanSummary
	Warnings []string
}

type ScanSummary struct {
	Total    int
	Comments int
	Posts    int
	Skipped  int
}

type redditListing struct {
	Data redditListingData `json:"data"`
}

type redditListingData struct {
	After    string              `json:"after"`
	Children []redditListingItem `json:"children"`
}

type redditListingItem struct {
	Kind string          `json:"kind"`
	Data redditThingData `json:"data"`
}

type redditThingData struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Author      string  `json:"author"`
	Subreddit   string  `json:"subreddit"`
	CreatedUTC  float64 `json:"created_utc"`
	Permalink   string  `json:"permalink"`
	Body        string  `json:"body"`
	LinkID      string  `json:"link_id"`
	ParentID    string  `json:"parent_id"`
	Title       string  `json:"title"`
	SelfText    string  `json:"selftext"`
	IsSelf      bool    `json:"is_self"`
	Over18      bool    `json:"over_18"`
	Spoiler     bool    `json:"spoiler"`
	AuthorFull  string  `json:"author_fullname"`
	SubredditID string  `json:"subreddit_id"`
}

func (client *Client) ScanActivity(ctx context.Context, username string, options ScanOptions) (ScanResult, error) {
	if client == nil {
		return ScanResult{}, errors.New("reddit client is required")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return ScanResult{}, errors.New("reddit username is required")
	}

	options = normalizeScanOptions(options)
	var result ScanResult

	if options.IncludeComments {
		client.scanActivityListing(ctx, username, "comments", "t1", domain.ItemTypeComment, options, &result)
	}
	if options.IncludePosts && (options.MaxItems <= 0 || len(result.Items) < options.MaxItems) {
		client.scanActivityListing(ctx, username, "submitted", "t3", domain.ItemTypePost, options, &result)
	}

	result.Summary = summarizeScan(result.Items, result.Summary.Skipped)
	if len(result.Warnings) > 0 && len(result.Items) == 0 {
		return result, errors.New("reddit scan found no supported activity")
	}
	return result, nil
}

func (client *Client) scanActivityListing(ctx context.Context, username, listing, wantKind string, itemType domain.ActivityItemType, options ScanOptions, result *ScanResult) {
	path := "/user/" + url.PathEscape(username) + "/" + listing
	after := ""
	count := 0

	for page := 0; page < options.MaxPages; page++ {
		query := (ListingParams{Limit: options.Limit, After: after, Count: count}).Values()
		var payload redditListing
		if err := client.GetJSON(ctx, path, query, &payload); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s page %d skipped: %v", listing, page+1, err))
			result.Summary.Skipped++
			return
		}

		for i, child := range payload.Data.Children {
			item, ok, warning := normalizeRedditThing(child, username, wantKind, itemType, options.Now())
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s page %d item %d skipped: %s", listing, page+1, i+1, warning))
				result.Summary.Skipped++
				continue
			}
			result.Items = append(result.Items, item)
			if options.MaxItems > 0 && len(result.Items) >= options.MaxItems {
				result.Summary = summarizeScan(result.Items, result.Summary.Skipped)
				return
			}
		}

		count += len(payload.Data.Children)
		after = strings.TrimSpace(payload.Data.After)
		if after == "" {
			return
		}
	}
}

func normalizeRedditThing(child redditListingItem, username, wantKind string, itemType domain.ActivityItemType, scannedAt time.Time) (domain.ActivityItem, bool, string) {
	kind := strings.TrimSpace(child.Kind)
	if kind == "" {
		kind = thingKindFromName(child.Data.Name)
	}
	if kind != wantKind {
		return domain.ActivityItem{}, false, fmt.Sprintf("unexpected kind %q", kind)
	}

	fullname := redditFullname(wantKind, child.Data)
	targetURL := redditPermalink(child.Data.Permalink)
	if fullname == "" && targetURL == "" {
		return domain.ActivityItem{}, false, "missing safe target"
	}

	occurredAt := redditCreatedAt(child.Data.CreatedUTC)
	metadata := map[string]string{
		"reddit_kind": wantKind,
	}
	addMetadata(metadata, "subreddit", child.Data.Subreddit)
	addMetadata(metadata, "author_fullname", child.Data.AuthorFull)
	addMetadata(metadata, "subreddit_id", child.Data.SubredditID)

	var text *domain.SafeTextReference
	switch itemType {
	case domain.ItemTypeComment:
		addMetadata(metadata, "link_id", child.Data.LinkID)
		addMetadata(metadata, "parent_id", child.Data.ParentID)
		text = safeText(child.Data.Body)
	case domain.ItemTypePost:
		metadata["is_self"] = fmt.Sprintf("%t", child.Data.IsSelf)
		metadata["over_18"] = fmt.Sprintf("%t", child.Data.Over18)
		metadata["spoiler"] = fmt.Sprintf("%t", child.Data.Spoiler)
		text = safeText(strings.TrimSpace(child.Data.Title + "\n" + child.Data.SelfText))
	}

	item := domain.ActivityItem{
		ID:         redditItemID(itemType, fullname, targetURL, occurredAt),
		Platform:   domain.PlatformReddit,
		Type:       itemType,
		TargetURL:  targetURL,
		TargetID:   fullname,
		Actor:      firstNonEmpty(child.Data.Author, username),
		OccurredAt: occurredAt,
		Source: domain.SourceMetadata{
			Name:       redditSourceName,
			ImportedAt: timePtr(scannedAt),
		},
		Metadata: metadata,
		Text:     text,
	}
	if err := item.Validate(); err != nil {
		return domain.ActivityItem{}, false, err.Error()
	}
	return item, true, ""
}

func normalizeScanOptions(options ScanOptions) ScanOptions {
	if !options.IncludeComments && !options.IncludePosts {
		options.IncludeComments = true
		options.IncludePosts = true
	}
	if options.Limit <= 0 || options.Limit > MaxListingSize {
		options.Limit = DefaultScanLimit
	}
	if options.MaxPages <= 0 {
		options.MaxPages = DefaultScanMaxPages
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	return options
}

func summarizeScan(items []domain.ActivityItem, skipped int) ScanSummary {
	summary := ScanSummary{Total: len(items), Skipped: skipped}
	for _, item := range items {
		switch item.Type {
		case domain.ItemTypeComment:
			summary.Comments++
		case domain.ItemTypePost:
			summary.Posts++
		}
	}
	return summary
}

func redditFullname(kind string, data redditThingData) string {
	name := strings.TrimSpace(data.Name)
	if name != "" {
		return name
	}
	id := strings.TrimSpace(data.ID)
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, kind+"_") {
		return id
	}
	return kind + "_" + id
}

func thingKindFromName(name string) string {
	if parts := strings.SplitN(strings.TrimSpace(name), "_", 2); len(parts) == 2 {
		return parts[0]
	}
	return ""
}

func redditPermalink(permalink string) string {
	permalink = strings.TrimSpace(permalink)
	if permalink == "" {
		return ""
	}
	if strings.HasPrefix(permalink, "https://") || strings.HasPrefix(permalink, "http://") {
		parsed, err := url.Parse(permalink)
		if err != nil {
			return ""
		}
		host := strings.ToLower(parsed.Hostname())
		if host == "reddit.com" || host == "www.reddit.com" || strings.HasSuffix(host, ".reddit.com") {
			return parsed.String()
		}
		return ""
	}
	if strings.HasPrefix(permalink, "/") {
		return "https://www.reddit.com" + permalink
	}
	return "https://www.reddit.com/" + permalink
}

func redditCreatedAt(value float64) *time.Time {
	if value <= 0 {
		return nil
	}
	seconds := int64(value)
	nanos := int64((value - float64(seconds)) * 1e9)
	createdAt := time.Unix(seconds, nanos).UTC()
	return &createdAt
}

func redditItemID(itemType domain.ActivityItemType, fullname, targetURL string, occurredAt *time.Time) string {
	if fullname != "" {
		return "reddit:" + string(itemType) + ":" + fullname
	}
	parts := []string{string(itemType), targetURL}
	if occurredAt != nil {
		parts = append(parts, occurredAt.Format(time.RFC3339Nano))
	}
	return "reddit:" + string(itemType) + ":" + shortScanHash(parts...)
}

func safeText(value string) *domain.SafeTextReference {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &domain.SafeTextReference{
		Hash:    "sha256:" + fullScanHash(value),
		Preview: textPreview(value),
	}
}

func textPreview(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= DefaultTextPreview {
		return value
	}
	return strings.TrimSpace(string(runes[:DefaultTextPreview])) + "..."
}

func addMetadata(metadata map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		metadata[key] = value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func fullScanHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func shortScanHash(parts ...string) string {
	return fullScanHash(strings.Join(parts, "\x00"))[:16]
}
