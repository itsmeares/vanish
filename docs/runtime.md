# Cleanup Runtime

Vanish separates persisted action status from richer runtime outcomes. Cleanup
plans remain format version 1 and store only the existing statuses: `pending`,
`running`, `done`, `failed`, `skipped`, `stopped`, and `cancelled`. Outcomes,
attempts, retry timing, and provider codes exist only in the active execution,
its events, and safe audit metadata.

## Outcomes and Statuses

- `succeeded` and `already_satisfied` map to `done`. Already satisfied means the
  requested final state already exists and is not an error.
- `retryable_failure`, `permanent_failure`, `rate_limited`, and
  `authentication_required` map to `failed` when final.
- `stopped` and `cancelled` retain their matching action statuses and existing
  execution behavior.

`retryable_failure` is a provider safety declaration: the provider has decided
that another attempt cannot repeat an ambiguous mutation. Vanish never infers
retry safety from an unknown result or a generic error.

## Bounded Retry and Halts

The default policy performs one attempt per action and continues after an
ordinary final failure. A caller may configure up to five attempts and may stop
after the first final failed action. Larger values are clamped to five. Only
`retryable_failure` can be attempted again automatically, and the effective
maximum is never exceeded.

Vanish does not sleep or wait in the background. A positive retry-after value,
rate limit, or authentication loss halts the execution and leaves untouched
actions pending. Retry timing is shown for a later explicit action. Authentication
loss requires reconnection; this runtime does not add a credential flow.

## Safety and Deferred Work

Provider results contain only an outcome, optional structured message ID,
optional retry-after, and optional diagnostic code. User-facing messages are
runtime-owned; arbitrary provider text is never copied into normalized results,
events, audit, or the TUI. The runner owns action identity, route identity,
attempts, retry decisions, and domain status. Raw platform responses, raw
errors, target URLs, credentials, tokens, cookies, sessions, authorization
values, and private content do not enter execution audit metadata.

This runtime still uses no-op simulation executors and enables no platform
mutation. Durable execution journals and restart/resume belong to PR 14.
Idempotency keys and remote-state reconciliation remain deferred to later work.
