package apply

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

const ExecutionJournalFormatVersion = 1

type ExecutionID string

type JournalEventKind string

const (
	JournalExecutionStarted   JournalEventKind = "execution_started"
	JournalAttemptStarted     JournalEventKind = "action_attempt_started"
	JournalResultRecorded     JournalEventKind = "action_result_recorded"
	JournalExecutionResumed   JournalEventKind = "execution_resumed"
	JournalExecutionHalted    JournalEventKind = "execution_halted"
	JournalExecutionStopped   JournalEventKind = "execution_stopped"
	JournalExecutionCancelled JournalEventKind = "execution_cancelled"
	JournalExecutionFailed    JournalEventKind = "execution_failed"
	JournalExecutionCompleted JournalEventKind = "execution_completed"
	JournalExecutionAbandoned JournalEventKind = "execution_abandoned"
)

type Resumability string

const (
	ResumabilityResumable       Resumability = "resumable"
	ResumabilityWaitingRetry    Resumability = "waiting_for_retry_time"
	ResumabilityWaitingProvider Resumability = "waiting_for_provider_readiness"
	ResumabilityResolution      Resumability = "resolution_required"
	ResumabilityTerminal        Resumability = "terminal"
	ResumabilityCorrupt         Resumability = "corrupt"
	ResumabilityLocked          Resumability = "locked"
)

type ManifestSummary struct {
	SourceLabel string              `json:"source_label"`
	Platform    domain.PlatformName `json:"platform"`
	ActionCount int                 `json:"action_count"`
}

type ExecutionManifest struct {
	FormatVersion int                 `json:"format_version"`
	ExecutionID   ExecutionID         `json:"execution_id"`
	CreatedAt     time.Time           `json:"created_at"`
	Plan          domain.CleanupPlan  `json:"plan"`
	PlanID        string              `json:"plan_id"`
	Platform      domain.PlatformName `json:"platform"`
	Mode          ExecutionMode       `json:"execution_mode"`
	Executor      ExecutorID          `json:"executor_id"`
	Policy        RunPolicy           `json:"policy"`
	Fingerprint   string              `json:"fingerprint"`
	Summary       ManifestSummary     `json:"summary"`
}

type JournalEvent struct {
	ExecutionID      ExecutionID         `json:"execution_id"`
	Fingerprint      string              `json:"fingerprint"`
	Sequence         int64               `json:"sequence"`
	Timestamp        time.Time           `json:"timestamp"`
	Kind             JournalEventKind    `json:"kind"`
	ActionID         string              `json:"action_id,omitempty"`
	ActionType       domain.ActionType   `json:"action_type,omitempty"`
	Platform         domain.PlatformName `json:"platform,omitempty"`
	Attempt          int                 `json:"attempt,omitempty"`
	Outcome          ActionOutcome       `json:"outcome,omitempty"`
	Status           domain.ActionStatus `json:"status,omitempty"`
	RetryAfterMillis int64               `json:"retry_after_ms,omitempty"`
	ProviderCode     ProviderCode        `json:"provider_code,omitempty"`
	MessageID        ProviderMessage     `json:"message_id,omitempty"`
	HaltReason       ActionOutcome       `json:"halt_reason,omitempty"`
}

type ExecutionSummary struct {
	FormatVersion   int                 `json:"format_version"`
	ExecutionID     ExecutionID         `json:"execution_id"`
	Fingerprint     string              `json:"fingerprint"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	SourceLabel     string              `json:"source_label"`
	Platform        domain.PlatformName `json:"platform"`
	Mode            ExecutionMode       `json:"execution_mode"`
	State           ExecutionState      `json:"state"`
	Resumability    Resumability        `json:"resumability"`
	BlockReason     string              `json:"block_reason,omitempty"`
	Counts          ResultCounts        `json:"counts"`
	LastSequence    int64               `json:"last_sequence"`
	JournalBytes    int64               `json:"journal_bytes"`
	RecoveryWarning string              `json:"recovery_warning,omitempty"`
	storeKey        string
}

type AttemptRecord struct {
	StartedAt time.Time
	ResultAt  time.Time
	Result    ActionResult
}

type UnresolvedAttempt struct {
	ActionID   string
	ActionType domain.ActionType
	Platform   domain.PlatformName
	Attempt    int
	StartedAt  time.Time
}

type ExecutionView struct {
	Manifest          ExecutionManifest
	Plan              domain.CleanupPlan
	State             ExecutionState
	HaltReason        ActionOutcome
	Counts            ResultCounts
	AttemptHistory    map[string][]AttemptRecord
	LastAttempts      map[string]int
	NextActionID      string
	NextAttempt       int
	RetryNotBefore    time.Time
	Resumability      Resumability
	BlockReason       string
	RecoveryWarning   string
	Unresolved        *UnresolvedAttempt
	NeedsFinalization bool
	LastSequence      int64
	UpdatedAt         time.Time
	TerminalKind      JournalEventKind
}

var (
	ErrExecutionStoreUnavailable   = errors.New("durable execution storage is unavailable")
	ErrExecutionExists             = errors.New("a matching execution already exists")
	ErrExecutionLocked             = errors.New("execution is active in another process")
	ErrExecutionCorrupt            = errors.New("execution journal is corrupt")
	ErrExecutionResolutionRequired = errors.New("execution requires resolution")
	ErrExecutionTerminal           = errors.New("execution is terminal")
	ErrExecutionNotReady           = errors.New("execution is not ready to resume")
	ErrExecutionMustAbandon        = errors.New("execution must be abandoned before deletion")
	ErrExecutionIdentityMismatch   = errors.New("execution identity does not match")
)

type ExistingExecutionError struct {
	Summary ExecutionSummary
}

func (err ExistingExecutionError) Error() string { return ErrExecutionExists.Error() }
func (err ExistingExecutionError) Unwrap() error { return ErrExecutionExists }

func NewExecutionID() (ExecutionID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create execution ID: %w", err)
	}
	return ExecutionID("exec-" + hex.EncodeToString(value[:])), nil
}

func newExecutionManifest(id ExecutionID, plan domain.CleanupPlan, mode ExecutionMode, executor ExecutorID, policy RunPolicy, createdAt time.Time) (ExecutionManifest, error) {
	if strings.TrimSpace(string(id)) == "" {
		return ExecutionManifest{}, errors.New("execution ID is required")
	}
	cloned := cloneExecutionPlan(plan)
	if err := cloned.Validate(); err != nil {
		return ExecutionManifest{}, err
	}
	policy = policy.normalized()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	fingerprint, err := executionFingerprint(cloned, mode, executor, policy)
	if err != nil {
		return ExecutionManifest{}, err
	}
	return ExecutionManifest{
		FormatVersion: ExecutionJournalFormatVersion,
		ExecutionID:   id,
		CreatedAt:     createdAt.UTC(),
		Plan:          cloned,
		PlanID:        cloned.ID,
		Platform:      cloned.Platform,
		Mode:          mode,
		Executor:      executor,
		Policy:        policy,
		Fingerprint:   fingerprint,
		Summary:       ManifestSummary{SourceLabel: cloned.SourceName, Platform: cloned.Platform, ActionCount: len(cloned.Actions)},
	}, nil
}

func executionFingerprint(plan domain.CleanupPlan, mode ExecutionMode, executor ExecutorID, policy RunPolicy) (string, error) {
	identity := struct {
		Plan     domain.CleanupPlan `json:"plan"`
		Mode     ExecutionMode      `json:"execution_mode"`
		Executor ExecutorID         `json:"executor_id"`
		Policy   RunPolicy          `json:"policy"`
	}{Plan: plan, Mode: mode, Executor: executor, Policy: policy.normalized()}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode execution identity: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validateExecutionManifest(manifest ExecutionManifest) error {
	if manifest.FormatVersion != ExecutionJournalFormatVersion || strings.TrimSpace(string(manifest.ExecutionID)) == "" || manifest.CreatedAt.IsZero() {
		return ErrExecutionCorrupt
	}
	if err := manifest.Plan.Validate(); err != nil || manifest.Plan.ID != manifest.PlanID || manifest.Plan.Platform != manifest.Platform {
		return ErrExecutionCorrupt
	}
	if manifest.Mode != ExecutionModeSimulation || strings.TrimSpace(string(manifest.Executor)) == "" || manifest.Policy != manifest.Policy.normalized() {
		return ErrExecutionCorrupt
	}
	if manifest.Summary.SourceLabel != manifest.Plan.SourceName || manifest.Summary.Platform != manifest.Platform || manifest.Summary.ActionCount != len(manifest.Plan.Actions) {
		return ErrExecutionCorrupt
	}
	want, err := executionFingerprint(manifest.Plan, manifest.Mode, manifest.Executor, manifest.Policy)
	if err != nil || !validFingerprint(manifest.Fingerprint) || manifest.Fingerprint != want {
		return ErrExecutionCorrupt
	}
	return nil
}

func cloneExecutionPlan(plan domain.CleanupPlan) domain.CleanupPlan {
	cloned := plan
	cloned.Actions = make([]domain.CleanupAction, len(plan.Actions))
	copy(cloned.Actions, plan.Actions)
	for index := range cloned.Actions {
		if plan.Actions[index].Metadata == nil {
			continue
		}
		cloned.Actions[index].Metadata = make(map[string]string, len(plan.Actions[index].Metadata))
		for key, value := range plan.Actions[index].Metadata {
			cloned.Actions[index].Metadata[key] = value
		}
	}
	return cloned
}

func validFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}

func journalRetryDuration(milliseconds int64) time.Duration {
	if milliseconds <= 0 || milliseconds > int64((1<<63-1)/int64(time.Millisecond)) {
		return 0
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func RuntimeErrorMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrExecutionLocked):
		return "Execution is active in another Vanish process."
	case errors.Is(err, ErrExecutionResolutionRequired):
		return "A previous action has an unknown result. Abandon this execution to stop it."
	case errors.Is(err, ErrExecutionCorrupt):
		return "Execution data is unreadable. Resume is unavailable."
	case errors.Is(err, ErrExecutionNotReady):
		return "Execution is not ready to resume."
	case errors.Is(err, ErrExecutionTerminal):
		return "This execution has already ended."
	case errors.Is(err, ErrExecutionExists):
		return "This plan already has a durable execution."
	case errors.Is(err, ErrExecutionStoreUnavailable):
		return "Durable execution storage is unavailable."
	default:
		return "Vanish could not safely save execution progress."
	}
}
