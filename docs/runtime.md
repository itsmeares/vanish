# Cleanup Runtime

Vanish separates persisted action status from richer runtime outcomes. Cleanup
plans remain format version 1 and store only the existing statuses: `pending`,
`running`, `done`, `failed`, `skipped`, `stopped`, and `cancelled`. Outcomes,
attempts, retry timing, and provider codes exist in runtime events, durable
execution journals, and safe audit metadata. They never enter cleanup-plan JSON.

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

A valid typed result returned without an executor error remains authoritative
for its action. If runner cancellation races with that return, Vanish records
the result first, then cancels only untouched remaining actions.

## Durable Journal Format

The execution journal has its own format version, currently `1`, independent of
cleanup-plan format version `1`. Each execution gets a random ID and an immutable
manifest containing a deep copy of the plan, the simulation route, normalized
run policy, creation time, and a deterministic fingerprint of that identity.
An identical plan, route, and policy cannot silently create a second execution.

The append-only JSONL journal is authoritative. Its records use contiguous
sequence numbers and contain only runtime-owned fields. A derived summary file
makes the Local Data list cheap to open, but replay never trusts that cache as
the source of truth.

Durability ordering is part of the safety contract:

1. The manifest and `execution_started` record are synced before an executor is
   available to run.
2. `action_attempt_started` is appended and synced before each executor call.
3. `action_result_recorded` is appended and synced before another executor call.
4. Terminal, halt, stop, resume, and abandon transitions are appended in order.

Manifest and summary replacement use a synced temporary file and atomic rename.
Journal appends are synced before returning. When `journal.jsonl` is first
created, its execution directory is also synced where supported.

## Replay and Safe Resume

Replay validates the manifest, identity fingerprint, event schema, sequence,
timestamps, route, action order, attempt numbers, outcomes, statuses, and
terminal transitions. It builds an action index once, so validation is linear in
the number of actions plus journal events. A malformed newline-terminated record
is corruption. Read-only replay may ignore one unterminated final record with a
visible recovery warning. Before appending, a locked writer truncates only that
partial tail to the last complete record boundary, syncs the repair, and replays
the journal. Terminated and interior corruption is never repaired.

If an attempt-start record exists without a durable result, the action's outcome
is unknown. Vanish never retries that action and never infers success or failure.
The execution becomes `resolution_required`; the only forward action is explicit
abandonment. Abandonment records that the execution ended without making a claim
about the unknown platform result and does not invoke the provider.

Otherwise, explicit resume continues from the first eligible action and never
repeats a completed action. Attempt numbers continue from the journal. Retry
deadlines and authentication prerequisites are rechecked before new executor
calls. Completed, failed, cancelled, and abandoned terminal executions cannot be
resumed. Vanish never resumes automatically during startup.

One writer lock protects each execution across processes. A separate identity
lock serializes creation of matching manifests. Locked executions remain
readable in Local Data but Resume is disabled until the other process exits.

## Safety and Deferred Work

Provider results contain only an outcome, optional structured message ID,
optional retry-after, and optional closed diagnostic code ID. User-facing
messages and diagnostic identifiers are runtime-owned; arbitrary provider text
is never copied into normalized results, events, audit, or the TUI. The runner
owns action identity, route identity,
attempts, retry decisions, and domain status. Raw platform responses, raw
errors, target URLs, credentials, tokens, cookies, sessions, authorization
values, and private content do not enter execution audit metadata.

This runtime still uses no-op simulation executors and enables no platform
mutation or new network behavior. Idempotency keys and remote-state
reconciliation remain deferred to PR 15. Runtime simulator and fault scenarios
remain deferred to PR 16. Real platform execution remains deferred.
