package apply

import (
	"context"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

// ActionOutcome describes what an executor observed at runtime. Outcomes are
// richer than persisted domain action statuses and never enter cleanup-plan
// JSON.
type ActionOutcome string

const (
	OutcomeSucceeded              ActionOutcome = "succeeded"
	OutcomeAlreadySatisfied       ActionOutcome = "already_satisfied"
	OutcomeRetryableFailure       ActionOutcome = "retryable_failure"
	OutcomePermanentFailure       ActionOutcome = "permanent_failure"
	OutcomeRateLimited            ActionOutcome = "rate_limited"
	OutcomeAuthenticationRequired ActionOutcome = "authentication_required"
	OutcomeStopped                ActionOutcome = "stopped"
	OutcomeCancelled              ActionOutcome = "cancelled"
)

// ProviderResult contains only provider-owned runtime information. The runner
// supplies action identity, status, attempt, route, and retry decisions.
type ProviderResult struct {
	Outcome      ActionOutcome
	Message      ProviderMessage
	RetryAfter   time.Duration
	ProviderCode string
}

// ProviderMessage is a closed identifier for runtime-owned user-facing copy.
// Arbitrary provider text is never copied into normalized results.
type ProviderMessage string

const ProviderMessageNoopCompleted ProviderMessage = "noop_completed"

type ActionResult struct {
	ActionID     string
	Platform     domain.PlatformName
	Type         domain.ActionType
	Status       domain.ActionStatus
	Outcome      ActionOutcome
	Attempt      int
	RetryAfter   time.Duration
	ProviderCode string
	Message      string
}

func (result ActionResult) Retryable() bool {
	return result.Outcome == OutcomeRetryableFailure
}

// RunPolicy bounds automatic attempts and controls ordinary final failures.
// Provider halts, explicit stops, and cancellation always stop execution.
type RunPolicy struct {
	MaxAttemptsPerAction  int
	StopAfterFinalFailure bool
}

// MaxAutomaticAttemptsPerAction is the hard runtime ceiling for tight,
// automatic retry loops. Policies above this value are safely clamped.
const MaxAutomaticAttemptsPerAction = 5

func DefaultRunPolicy() RunPolicy {
	return RunPolicy{MaxAttemptsPerAction: 1}
}

func (policy RunPolicy) normalized() RunPolicy {
	if policy.MaxAttemptsPerAction <= 0 {
		policy.MaxAttemptsPerAction = 1
	} else if policy.MaxAttemptsPerAction > MaxAutomaticAttemptsPerAction {
		policy.MaxAttemptsPerAction = MaxAutomaticAttemptsPerAction
	}
	return policy
}

type Executor interface {
	Execute(context.Context, domain.CleanupAction) (ProviderResult, error)
}

type NoopExecutor struct{}

func (NoopExecutor) Execute(ctx context.Context, _ domain.CleanupAction) (ProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return ProviderResult{}, err
	}
	return ProviderResult{
		Outcome: OutcomeSucceeded,
		Message: ProviderMessageNoopCompleted,
	}, nil
}

func normalizeProviderResult(ctx context.Context, action domain.CleanupAction, attempt int, providerResult ProviderResult, executeErr error) ActionResult {
	if runnerContextDone(ctx) {
		return normalizedActionResult(action, attempt, ProviderResult{Outcome: OutcomeCancelled}, "Execution cancelled.")
	}
	if executeErr != nil {
		return normalizedActionResult(action, attempt, ProviderResult{Outcome: OutcomePermanentFailure}, "Executor failed unexpectedly.")
	}

	providerResult.ProviderCode = strings.TrimSpace(providerResult.ProviderCode)
	if !validProviderResultMetadata(providerResult) {
		return normalizedActionResult(action, attempt, ProviderResult{Outcome: OutcomePermanentFailure}, "Executor returned an invalid result.")
	}
	message, knownMessage := runtimeProviderMessage(providerResult.Outcome, providerResult.Message)
	if !knownMessage {
		providerResult.Message = ""
		providerResult.ProviderCode = ""
	}
	return normalizedActionResult(action, attempt, providerResult, message)
}

func normalizedActionResult(action domain.CleanupAction, attempt int, providerResult ProviderResult, message string) ActionResult {
	status, ok := statusForOutcome(providerResult.Outcome)
	if !ok {
		status = domain.ActionStatusFailed
		providerResult = ProviderResult{Outcome: OutcomePermanentFailure}
		message = "Executor returned an invalid result."
	}
	return ActionResult{
		ActionID:     action.ID,
		Platform:     action.Platform,
		Type:         action.Type,
		Status:       status,
		Outcome:      providerResult.Outcome,
		Attempt:      attempt,
		RetryAfter:   providerResult.RetryAfter,
		ProviderCode: providerResult.ProviderCode,
		Message:      message,
	}
}

func validProviderResultMetadata(result ProviderResult) bool {
	if _, ok := statusForOutcome(result.Outcome); !ok {
		return false
	}
	if result.RetryAfter < 0 {
		return false
	}
	if result.RetryAfter > 0 && result.Outcome != OutcomeRetryableFailure && result.Outcome != OutcomeRateLimited {
		return false
	}
	if !validProviderCode(result.ProviderCode) {
		return false
	}
	return true
}

func statusForOutcome(outcome ActionOutcome) (domain.ActionStatus, bool) {
	switch outcome {
	case OutcomeSucceeded, OutcomeAlreadySatisfied:
		return domain.ActionStatusDone, true
	case OutcomeRetryableFailure, OutcomePermanentFailure, OutcomeRateLimited, OutcomeAuthenticationRequired:
		return domain.ActionStatusFailed, true
	case OutcomeStopped:
		return domain.ActionStatusStopped, true
	case OutcomeCancelled:
		return domain.ActionStatusCancelled, true
	default:
		return "", false
	}
}

func runnerContextDone(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}

func validProviderCode(code string) bool {
	if code == "" {
		return true
	}
	if len(code) > 64 {
		return false
	}
	for _, char := range code {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.') {
			return false
		}
	}
	normalized := strings.ToLower(code)
	for _, forbidden := range []string{"authorization", "cookie", "credential", "password", "secret", "session", "token"} {
		if strings.Contains(normalized, forbidden) {
			return false
		}
	}
	return true
}

func runtimeProviderMessage(outcome ActionOutcome, message ProviderMessage) (string, bool) {
	switch message {
	case "":
		return defaultOutcomeMessage(outcome), true
	case ProviderMessageNoopCompleted:
		if outcome == OutcomeSucceeded {
			return "No-op apply completed.", true
		}
	}
	return defaultOutcomeMessage(outcome), false
}

func defaultOutcomeMessage(outcome ActionOutcome) string {
	switch outcome {
	case OutcomeSucceeded:
		return "Action completed."
	case OutcomeAlreadySatisfied:
		return "Action was already satisfied."
	case OutcomeRetryableFailure:
		return "Action failed and may be retried safely."
	case OutcomePermanentFailure:
		return "Action failed."
	case OutcomeRateLimited:
		return "Provider rate limit reached."
	case OutcomeAuthenticationRequired:
		return "Reconnect the account before trying again."
	case OutcomeStopped:
		return "Execution stopped."
	case OutcomeCancelled:
		return "Execution cancelled."
	default:
		return "Executor returned an invalid result."
	}
}
