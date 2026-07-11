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

const FormatVersion = 2

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
	FormatVersion int                `json:"format_version"`
	ID            string             `json:"id"`
	PlanID        string             `json:"plan_id"`
	Mode          Mode               `json:"mode"`
	CreatedAt     time.Time          `json:"created_at"`
	PlanSnapshot  domain.CleanupPlan `json:"plan"`
	Actions       []Action           `json:"actions"`
}

type Session struct {
	Manifest
	UpdatedAt       time.Time
	CurrentPosition int
	State           State
	Outcomes        []Outcome
	doneCount       int
	skippedCount    int
	countsReady     bool
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
	if err := plan.Validate(); err != nil {
		return Session{}, nil, errors.New("manual cleanup requires a valid Instagram plan")
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
			if actor, valid := instagram.NormalizeUsername(item.Actor); valid {
				action.Actor = actor
			}
			if item.OccurredAt != nil {
				occurredAt := item.OccurredAt.UTC()
				action.OccurredAt = &occurredAt
			}
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
			PlanSnapshot:  clonePlan(plan),
			Actions:       actions,
		},
		UpdatedAt:   now.UTC(),
		State:       StateActive,
		Outcomes:    outcomes,
		countsReady: true,
	}, unavailable, nil
}

func (session Session) Current() (Action, bool) {
	if session.CurrentPosition < 0 || session.CurrentPosition >= len(session.Actions) || session.State == StateCompleted {
		return Action{}, false
	}
	return session.Actions[session.CurrentPosition], true
}

func (session Session) Counts() (done, skipped, pending int) {
	if session.countsReady {
		return session.doneCount, session.skippedCount, len(session.Actions) - session.doneCount - session.skippedCount
	}
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

func (session Session) OriginalPlan() domain.CleanupPlan {
	return clonePlan(session.PlanSnapshot)
}

func (session *Session) initializeProgress() {
	session.UpdatedAt = session.CreatedAt
	session.CurrentPosition = 0
	session.State = StateActive
	session.Outcomes = make([]Outcome, len(session.Actions))
	for index := range session.Outcomes {
		session.Outcomes[index] = OutcomePending
	}
	session.doneCount = 0
	session.skippedCount = 0
	session.countsReady = true
}

func (session *Session) ensureCounts() {
	if session.countsReady {
		return
	}
	session.doneCount = 0
	session.skippedCount = 0
	for _, outcome := range session.Outcomes {
		switch outcome {
		case OutcomeDone:
			session.doneCount++
		case OutcomeSkipped:
			session.skippedCount++
		}
	}
	session.countsReady = true
}

func (session *Session) recordOutcome(index int, outcome Outcome) error {
	if index < 0 || index >= len(session.Actions) || index >= len(session.Outcomes) || session.Outcomes[index] != OutcomePending {
		return errors.New("manual cleanup progress is unreadable")
	}
	session.ensureCounts()
	session.Outcomes[index] = outcome
	switch outcome {
	case OutcomeDone:
		session.doneCount++
	case OutcomeSkipped:
		session.skippedCount++
	default:
		return errors.New("manual cleanup progress is unreadable")
	}
	return nil
}

func clonePlan(plan domain.CleanupPlan) domain.CleanupPlan {
	cloned := plan
	cloned.Actions = make([]domain.CleanupAction, len(plan.Actions))
	copy(cloned.Actions, plan.Actions)
	for index := range cloned.Actions {
		metadata := plan.Actions[index].Metadata
		if metadata == nil {
			continue
		}
		cloned.Actions[index].Metadata = make(map[string]string, len(metadata))
		for key, value := range metadata {
			cloned.Actions[index].Metadata[key] = value
		}
	}
	return cloned
}
