package domain

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewCleanupPlanDefaultsToDryRun(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	action := validAction(createdAt)

	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, []CleanupAction{action})

	if plan.FormatVersion != PlanFormatVersion {
		t.Fatalf("expected format version %d, got %d", PlanFormatVersion, plan.FormatVersion)
	}
	if plan.ID != "plan-1" {
		t.Fatalf("expected stable plan id, got %q", plan.ID)
	}
	if plan.Platform != PlatformInstagram {
		t.Fatalf("expected instagram platform, got %q", plan.Platform)
	}
	if plan.SourceName != "instagram-export" {
		t.Fatalf("expected source name, got %q", plan.SourceName)
	}
	if plan.CreatedAt != createdAt {
		t.Fatalf("expected created time to be preserved")
	}
	if plan.Mode != PlanModeDryRun {
		t.Fatalf("expected dry-run mode, got %q", plan.Mode)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Status != ActionStatusPending {
		t.Fatalf("expected one pending action, got %#v", plan.Actions)
	}
}

func TestNewCleanupActionFromItemCopiesSafeReferences(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	occurredAt := createdAt.Add(-24 * time.Hour)
	item := ActivityItem{
		ID:         "item-1",
		Platform:   PlatformReddit,
		Type:       ItemTypeComment,
		TargetURL:  "https://reddit.example/comments/1",
		TargetID:   "comment-1",
		Actor:      "ares",
		OccurredAt: &occurredAt,
		Source: SourceMetadata{
			Name:     "reddit-export",
			ImportID: "import-1",
		},
		Text: &SafeTextReference{
			Hash:    "sha256:abc",
			Preview: "short harmless preview",
		},
	}

	action, err := NewCleanupActionFromItem("action-1", item, ActionDeleteComment, createdAt)
	if err != nil {
		t.Fatalf("expected action, got error: %v", err)
	}

	if action.ID != "action-1" {
		t.Fatalf("expected action id, got %q", action.ID)
	}
	if action.Platform != item.Platform {
		t.Fatalf("expected platform %q, got %q", item.Platform, action.Platform)
	}
	if action.Type != ActionDeleteComment {
		t.Fatalf("expected delete_comment, got %q", action.Type)
	}
	if action.TargetURL != item.TargetURL || action.TargetID != item.TargetID {
		t.Fatalf("expected target references to be copied")
	}
	if action.SourceActivityItemID != item.ID {
		t.Fatalf("expected source item id %q, got %q", item.ID, action.SourceActivityItemID)
	}
	if action.Status != ActionStatusPending {
		t.Fatalf("expected pending status, got %q", action.Status)
	}
}

func TestWriteAndReadPlanJSON(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	plan := NewCleanupPlan("plan-1", PlatformYouTube, "youtube-export", createdAt, []CleanupAction{
		validAction(createdAt),
	})

	var buf bytes.Buffer
	if err := WritePlanJSON(&buf, plan); err != nil {
		t.Fatalf("expected plan to write, got error: %v", err)
	}

	jsonText := buf.String()
	if !strings.Contains(jsonText, "\n  \"format_version\"") {
		t.Fatalf("expected pretty JSON, got %s", jsonText)
	}
	if !strings.Contains(jsonText, "\"mode\": \"dry-run\"") {
		t.Fatalf("expected dry-run mode in JSON, got %s", jsonText)
	}

	loaded, err := ReadPlanJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("expected plan to load, got error: %v", err)
	}

	if loaded.ID != plan.ID || loaded.Platform != plan.Platform || loaded.SourceName != plan.SourceName {
		t.Fatalf("loaded plan did not preserve identity fields: %#v", loaded)
	}
	if len(loaded.Actions) != 1 || loaded.Actions[0].SourceActivityItemID != "item-1" {
		t.Fatalf("loaded plan did not preserve actions: %#v", loaded.Actions)
	}
}

func TestCleanupPlanValidateRequiresFields(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		mutate  func(*CleanupPlan)
		wantErr string
	}{
		{
			name: "missing plan id",
			mutate: func(plan *CleanupPlan) {
				plan.ID = ""
			},
			wantErr: "id is required",
		},
		{
			name: "missing platform",
			mutate: func(plan *CleanupPlan) {
				plan.Platform = ""
			},
			wantErr: "platform is required",
		},
		{
			name: "missing source name",
			mutate: func(plan *CleanupPlan) {
				plan.SourceName = ""
			},
			wantErr: "source_name is required",
		},
		{
			name: "missing created time",
			mutate: func(plan *CleanupPlan) {
				plan.CreatedAt = time.Time{}
			},
			wantErr: "created_at is required",
		},
		{
			name: "invalid mode",
			mutate: func(plan *CleanupPlan) {
				plan.Mode = "live-delete"
			},
			wantErr: "mode",
		},
		{
			name: "invalid action status",
			mutate: func(plan *CleanupPlan) {
				plan.Actions[0].Status = "paused"
			},
			wantErr: "status",
		},
		{
			name: "missing action target",
			mutate: func(plan *CleanupPlan) {
				plan.Actions[0].TargetURL = ""
				plan.Actions[0].TargetID = ""
			},
			wantErr: "target_url or target_id is required",
		},
		{
			name: "missing action source item id",
			mutate: func(plan *CleanupPlan) {
				plan.Actions[0].SourceActivityItemID = ""
			},
			wantErr: "source_activity_item_id is required",
		},
		{
			name: "duplicate action id",
			mutate: func(plan *CleanupPlan) {
				duplicate := plan.Actions[0]
				duplicate.Type = ActionDeleteComment
				plan.Actions = append(plan.Actions, duplicate)
			},
			wantErr: `duplicate id "action-1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, []CleanupAction{
				validAction(createdAt),
			})
			tt.mutate(&plan)

			err := plan.Validate()
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestWritePlanJSONRejectsSecretLikeMetadataKeys(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	action := validAction(createdAt)
	action.Metadata = map[string]string{
		"access_token": "must-not-write",
	}
	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, []CleanupAction{action})

	var buf bytes.Buffer
	err := WritePlanJSON(&buf, plan)
	if err == nil {
		t.Fatalf("expected secret-like metadata key to be rejected")
	}
	if !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("expected secret-like error, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected invalid plan not to be written, got %s", buf.String())
	}
}

func TestWritePlanJSONRejectsApplyModeForNow(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, []CleanupAction{
		validAction(createdAt),
	})
	plan.Mode = PlanModeApply

	var buf bytes.Buffer
	err := WritePlanJSON(&buf, plan)
	if err == nil {
		t.Fatalf("expected apply mode to be rejected by the writer")
	}
	if !strings.Contains(err.Error(), "only \"dry-run\" plans are supported") {
		t.Fatalf("expected dry-run only error, got %v", err)
	}
}

func TestPlanModelDoesNotExposeObviousSecretFields(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(ActivityItem{}),
		reflect.TypeOf(CleanupAction{}),
		reflect.TypeOf(CleanupPlan{}),
		reflect.TypeOf(SourceMetadata{}),
		reflect.TypeOf(SafeTextReference{}),
	}
	forbidden := []string{
		"password",
		"passwd",
		"cookie",
		"token",
		"session",
		"secret",
		"credential",
		"private_message",
		"direct_message",
		"dm_body",
		"message_body",
		"full_message",
		"raw_message",
	}

	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			candidates := []string{
				strings.ToLower(field.Name),
				strings.ToLower(strings.Split(field.Tag.Get("json"), ",")[0]),
			}
			for _, candidate := range candidates {
				for _, word := range forbidden {
					if strings.Contains(candidate, word) {
						t.Fatalf("%s.%s exposes forbidden field or JSON name %q", typ.Name(), field.Name, candidate)
					}
				}
			}
		}
	}
}

func validAction(createdAt time.Time) CleanupAction {
	return CleanupAction{
		ID:                   "action-1",
		Platform:             PlatformInstagram,
		Type:                 ActionUnlike,
		TargetURL:            "https://instagram.example/p/1",
		TargetID:             "target-1",
		SourceActivityItemID: "item-1",
		Status:               ActionStatusPending,
		CreatedAt:            createdAt,
		Metadata: map[string]string{
			"reason": "older-than-cutoff",
		},
	}
}
