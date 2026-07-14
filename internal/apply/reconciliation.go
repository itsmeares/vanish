package apply

import (
	"context"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

// ReconciliationOutcome is a closed runtime-owned description of remote state.
// Providers may return only the five provider outcomes. Unsupported and invalid
// are produced by the runner so arbitrary provider values never reach storage.
type ReconciliationOutcome string

const (
	ReconciliationAlreadyApplied         ReconciliationOutcome = "already_applied"
	ReconciliationNotApplied             ReconciliationOutcome = "not_applied"
	ReconciliationConflictingState       ReconciliationOutcome = "conflicting_state"
	ReconciliationUnknown                ReconciliationOutcome = "unknown"
	ReconciliationTemporarilyUnavailable ReconciliationOutcome = "temporarily_unavailable"
	ReconciliationUnsupported            ReconciliationOutcome = "unsupported"
	ReconciliationInvalid                ReconciliationOutcome = "invalid"
)

type ReconciliationRequest struct {
	Action           domain.CleanupAction
	IdempotencyKey   ActionIdempotencyKey
	Attempt          int
	AttemptStartedAt time.Time
}

type Reconciler interface {
	Reconcile(context.Context, ReconciliationRequest) (ReconciliationOutcome, error)
}

type ReconciliationRecord struct {
	ActionID              string
	ActionType            domain.ActionType
	Platform              domain.PlatformName
	Attempt               int
	ReconciliationAttempt int
	StartedAt             time.Time
	ResultAt              time.Time
	Outcome               ReconciliationOutcome
}

func normalizeReconciliationOutcome(reconciler Reconciler, outcome ReconciliationOutcome, err error) ReconciliationOutcome {
	if reconciler == nil {
		return ReconciliationUnsupported
	}
	if err != nil {
		return ReconciliationTemporarilyUnavailable
	}
	if !outcome.providerKnown() {
		return ReconciliationInvalid
	}
	return outcome
}

func (outcome ReconciliationOutcome) providerKnown() bool {
	switch outcome {
	case ReconciliationAlreadyApplied, ReconciliationNotApplied, ReconciliationConflictingState, ReconciliationUnknown, ReconciliationTemporarilyUnavailable:
		return true
	default:
		return false
	}
}

func (outcome ReconciliationOutcome) journalKnown() bool {
	return outcome.providerKnown() || outcome == ReconciliationUnsupported || outcome == ReconciliationInvalid
}

// Known reports whether outcome is safe to expose through runtime-owned events.
func (outcome ReconciliationOutcome) Known() bool {
	return outcome.journalKnown()
}

func (outcome ReconciliationOutcome) resolvesAttempt() bool {
	return outcome == ReconciliationAlreadyApplied || outcome == ReconciliationNotApplied
}

func reconciliationBlockReason(outcome ReconciliationOutcome) string {
	switch outcome {
	case ReconciliationAlreadyApplied:
		return "Reconciliation confirmed the action was already applied. Resume is explicit."
	case ReconciliationNotApplied:
		return "Reconciliation confirmed the action was not applied. Resume is explicit."
	case ReconciliationConflictingState:
		return "Provider state conflicts with this action. Reconciliation is still required."
	case ReconciliationUnknown:
		return "Provider could not determine the action state. Reconciliation is still required."
	case ReconciliationTemporarilyUnavailable:
		return "Reconciliation is temporarily unavailable."
	case ReconciliationUnsupported:
		return "This provider cannot reconcile the unresolved action."
	case ReconciliationInvalid:
		return "Provider returned an invalid reconciliation result."
	default:
		return "A previous action has an unknown result."
	}
}
