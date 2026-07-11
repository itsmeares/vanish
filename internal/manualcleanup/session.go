package manualcleanup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
)

const FormatVersion = 1

type Mode string

const ModeInstagramManual Mode = "instagram-manual"

type State string

const (
	StateActive    State = "active"
	StateStopped   State = "stopped"
	StateCompleted State = "completed"
)

type Outcome string

const (
	OutcomePending Outcome = "pending"
	OutcomeDone    Outcome = "done"
	OutcomeSkipped Outcome = "skipped"
)

type Action struct {
	ActionID   string               `json:"action_id"`
	Type       domain.ActionType    `json:"type"`
	TargetURL  string               `json:"target_url"`
	TargetKind instagram.TargetKind `json:"target_kind"`
	TargetID   string               `json:"target_id"`
	Actor      string               `json:"actor,omitempty"`
	OccurredAt *time.Time           `json:"occurred_at,omitempty"`
	TextHash   string               `json:"text_hash,omitempty"`
}

type Manifest struct {
	FormatVersion int             `json:"format_version"`
	ID            string          `json:"id"`
	PlanID        string          `json:"plan_id"`
	Mode          Mode            `json:"mode"`
	CreatedAt     time.Time       `json:"created_at"`
	PlanCreatedAt time.Time       `json:"plan_created_at"`
	PlanMode      domain.PlanMode `json:"plan_mode"`
	SourceName    string          `json:"source_name,omitempty"`
	Actions       []Action        `json:"actions"`
}

type Session struct {
	Manifest
	UpdatedAt       time.Time
	CurrentPosition int
	State           State
	Outcomes        []Outcome
}

type Unavailable struct {
	ActionID string
	Reason   string
}

func NewID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create manual cleanup ID: %w", err)
	}
	return "manual-" + hex.EncodeToString(value[:]), nil
}

func New(id string, plan domain.CleanupPlan, items []domain.ActivityItem, now time.Time) (Session, []Unavailable, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, nil, errors.New("manual cleanup ID is required")
	}
	if plan.Platform != domain.PlatformInstagram {
		return Session{}, nil, errors.New("manual cleanup requires an Instagram plan")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	byID := make(map[string]domain.ActivityItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	actions := make([]Action, 0, len(plan.Actions))
	unavailable := make([]Unavailable, 0)
	for _, planned := range plan.Actions {
		switch planned.Type {
		case domain.ActionUnfollow, domain.ActionUnlike, domain.ActionDeleteComment:
		default:
			unavailable = append(unavailable, Unavailable{ActionID: planned.ID, Reason: "unsupported action"})
			continue
		}
		target, err := instagram.ValidateCleanupTarget(planned.Type, planned.TargetURL, planned.TargetID)
		if err != nil {
			unavailable = append(unavailable, Unavailable{ActionID: planned.ID, Reason: "unsafe or unavailable target"})
			continue
		}
		action := Action{
			ActionID:   planned.ID,
			Type:       planned.Type,
			TargetURL:  target.URL,
			TargetKind: target.Kind,
			TargetID:   target.Identifier,
		}
		if item, ok := byID[planned.SourceActivityItemID]; ok {
			action.Actor = strings.TrimSpace(item.Actor)
			action.OccurredAt = item.OccurredAt
			if item.Text != nil {
				action.TextHash = strings.TrimSpace(item.Text.Hash)
			}
		}
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		return Session{}, unavailable, errors.New("plan has no supported manual cleanup actions")
	}

	outcomes := make([]Outcome, len(actions))
	for i := range outcomes {
		outcomes[i] = OutcomePending
	}
	return Session{
		Manifest: Manifest{
			FormatVersion: FormatVersion,
			ID:            id,
			PlanID:        plan.ID,
			Mode:          ModeInstagramManual,
			CreatedAt:     now.UTC(),
			PlanCreatedAt: plan.CreatedAt,
			PlanMode:      plan.Mode,
			SourceName:    plan.SourceName,
			Actions:       actions,
		},
		UpdatedAt: now.UTC(),
		State:     StateActive,
		Outcomes:  outcomes,
	}, unavailable, nil
}

func (session Session) Current() (Action, bool) {
	if session.CurrentPosition < 0 || session.CurrentPosition >= len(session.Actions) || session.State == StateCompleted {
		return Action{}, false
	}
	return session.Actions[session.CurrentPosition], true
}

func (session Session) Counts() (done, skipped, pending int) {
	for _, outcome := range session.Outcomes {
		switch outcome {
		case OutcomeDone:
			done++
		case OutcomeSkipped:
			skipped++
		default:
			pending++
		}
	}
	return
}

func (session Session) RebuildPlan() domain.CleanupPlan {
	actions := make([]domain.CleanupAction, 0, len(session.Actions))
	for _, action := range session.Actions {
		actions = append(actions, domain.CleanupAction{
			ID:                   action.ActionID,
			Platform:             domain.PlatformInstagram,
			Type:                 action.Type,
			TargetURL:            action.TargetURL,
			TargetID:             action.TargetID,
			SourceActivityItemID: action.ActionID,
			Status:               domain.ActionStatusPending,
			CreatedAt:            session.PlanCreatedAt,
		})
	}
	plan := domain.NewCleanupPlan(session.PlanID, domain.PlatformInstagram, session.SourceName, session.PlanCreatedAt, actions)
	if session.PlanMode != "" {
		plan.Mode = session.PlanMode
	}
	return plan
}
