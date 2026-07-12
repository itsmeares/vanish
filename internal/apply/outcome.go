package apply

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	SafeMessage  string
	RetryAfter   time.Duration
	ProviderCode string
}

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

func DefaultRunPolicy() RunPolicy {
	return RunPolicy{MaxAttemptsPerAction: 1}
}

func (policy RunPolicy) normalized() RunPolicy {
	if policy.MaxAttemptsPerAction <= 0 {
		policy.MaxAttemptsPerAction = 1
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
		Outcome:     OutcomeSucceeded,
		SafeMessage: "No-op apply completed.",
	}, nil
}

func normalizeProviderResult(ctx context.Context, action domain.CleanupAction, attempt int, providerResult ProviderResult, executeErr error) ActionResult {
	if contextCancelled(ctx, executeErr) {
		return normalizedActionResult(action, attempt, ProviderResult{
			Outcome:     OutcomeCancelled,
			SafeMessage: "Execution cancelled.",
		})
	}
	if executeErr != nil {
		return normalizedActionResult(action, attempt, ProviderResult{
			Outcome:     OutcomePermanentFailure,
			SafeMessage: "Executor failed unexpectedly.",
		})
	}

	providerResult.SafeMessage = strings.TrimSpace(providerResult.SafeMessage)
	providerResult.ProviderCode = strings.TrimSpace(providerResult.ProviderCode)
	if !validProviderResult(providerResult) {
		return normalizedActionResult(action, attempt, ProviderResult{
			Outcome:     OutcomePermanentFailure,
			SafeMessage: "Executor returned an invalid result.",
		})
	}
	if providerResult.SafeMessage == "" {
		providerResult.SafeMessage = defaultOutcomeMessage(providerResult.Outcome)
	}
	return normalizedActionResult(action, attempt, providerResult)
}

func normalizedActionResult(action domain.CleanupAction, attempt int, providerResult ProviderResult) ActionResult {
	status, ok := statusForOutcome(providerResult.Outcome)
	if !ok {
		status = domain.ActionStatusFailed
		providerResult = ProviderResult{
			Outcome:     OutcomePermanentFailure,
			SafeMessage: "Executor returned an invalid result.",
		}
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
		Message:      providerResult.SafeMessage,
	}
}

func validProviderResult(result ProviderResult) bool {
	if _, ok := statusForOutcome(result.Outcome); !ok {
		return false
	}
	if result.RetryAfter < 0 {
		return false
	}
	if result.RetryAfter > 0 && result.Outcome != OutcomeRetryableFailure && result.Outcome != OutcomeRateLimited {
		return false
	}
	if !validSafeMessage(result.SafeMessage) || !validProviderCode(result.ProviderCode) {
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

func contextCancelled(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return ctx != nil && ctx.Err() != nil
}

func validSafeMessage(message string) bool {
	if message == "" {
		return true
	}
	if utf8.RuneCountInString(message) > 240 || strings.ContainsAny(message, "\r\n") {
		return false
	}
	for _, char := range message {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
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
