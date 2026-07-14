# Cleanup Runtime

Vanish separates persisted action status from richer runtime outcomes. Cleanup
plans remain format version 1 and store only the existing statuses: `pending`,
`running`, `done`, `failed`, `skipped`, `stopped`, and `cancelled`. Outcomes,
attempts, retry timing, and provider codes exist in runtime events, durable
execution journals, and safe audit metadata. Stable idempotency identities and
reconciliation records are also runtime-owned. They never enter cleanup-plan
JSON.

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

## Stable Action Identity

Every durable execution action receives an opaque `ActionIdempotencyKey`. Vanish
derives it from the durable execution ID and action ID using a versioned,
length-delimited SHA-256 identity. Retries, restart, replay, and explicit resume
therefore deliver the same key for the same logical action. Different actions or
different execution IDs produce different keys. The key is passed through the
runtime `ActionRequest`; it is not stored in cleanup-plan JSON or audit output.

Providers may expose a read-only reconciler. Its request contains the unresolved
action, original attempt metadata, and the same idempotency key. Provider
outcomes are closed: `already_applied`, `not_applied`, `conflicting_state`,
`unknown`, and `temporarily_unavailable`. Missing reconcilers normalize to
`unsupported`; provider errors normalize to `temporarily_unavailable`; invalid
values normalize to `invalid`. Raw provider responses and errors are discarded.

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
Reconciliation adds validated event kinds without rewriting manifests or prior
records. Existing version 1 histories replay with their original meaning; an
old unresolved attempt remains unresolved until the user explicitly reconciles
or abandons it.

The append-only JSONL journal is authoritative. Its records use contiguous
sequence numbers and contain only runtime-owned fields. A derived summary file
makes the Local Data list cheap to open, but replay never trusts that cache as
the source of truth.

Durability ordering is part of the safety contract:

1. The manifest and `execution_started` record are synced before an executor is
   available to run.
2. `action_attempt_started` is appended and synced before each executor call.
3. `action_result_recorded` is appended and synced before another executor call.
4. `action_reconciliation_started` is appended and synced before each provider
   reconciliation call.
5. `action_reconciliation_result_recorded` is appended and synced before a
   resolved action can become resumable or terminal.
6. Terminal, halt, stop, resume, and abandon transitions are appended in order.

Manifest and summary replacement use a synced temporary file and atomic rename.
New execution directories are synced into the execution-store directory.
Journal appends are synced before returning. When `journal.jsonl` is first
created, its execution directory is also synced where supported. Appends clamp
wall-clock rollback to the writer's last logical timestamp.

## Replay and Safe Resume

Replay validates the manifest, identity fingerprint, event schema, sequence,
timestamp presence, route, action order, attempt numbers, outcomes, statuses, and
terminal transitions. Reconciliation event order, attempts, identity, and closed
outcomes use the same validation. Replay remains linear in the number of actions
plus journal events. A malformed newline-terminated record
is corruption. Read-only replay may ignore one unterminated final record with a
visible recovery warning. Before appending, a locked writer truncates only that
partial tail to the last complete record boundary, syncs the repair, and replays
the journal. Terminated and interior corruption is never repaired. Sequence
remains authoritative when ordinary wall-clock rollback makes a later record
carry an earlier timestamp.

If an attempt-start record exists without a durable result, the action's outcome
is unknown. Vanish never retries that action and never infers success or failure.
The execution becomes `resolution_required`. The user may explicitly Reconcile,
Abandon, or go Back; startup never reconciles automatically.

Reconciliation is available only for `resolution_required` work. Its start and
closed result are durable journal events. `already_applied` marks the unresolved
action done without calling the executor. `not_applied` marks the attempt failed
and may expose the existing explicit Resume flow when retry limits or later work
permit it; reconciliation never invokes Resume automatically. Conflict, unknown,
unavailable, unsupported, and invalid results remain blocked and cannot trigger
executor entry. Repeating blocked or interrupted reconciliation reuses the same
action key and increments only the reconciliation-attempt counter. Abandonment
remains available and makes no claim about remote state.

Otherwise, explicit resume continues from the first eligible action and never
repeats a completed action. Attempt numbers continue from the journal. Retry
deadlines and authentication prerequisites are rechecked before new executor
calls. Completed, failed, cancelled, and abandoned terminal executions cannot be
resumed. Vanish never resumes automatically during startup.

One writer lock protects each execution across processes. A stable session lock
outside the removable execution directory excludes new writer acquisition for
the full deletion sequence, including validation, guard creation, removal, and
parent sync. A separate identity lock serializes creation of matching manifests,
and deletion retains a durable fingerprint guard. Corrupt deletion uses a
validated retained summary when the manifest is unreadable and fails closed if
no trustworthy fingerprint remains.
A shared workspace-use lease blocks local-data wipe while any
durable writer is active; wipe holds the exclusive lease so no writer can start
during removal. Locked executions remain readable in Local Data but Resume is
disabled until the other process exits.

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
mutation or new network behavior. Built-in providers do not expose production
reconcilers; reconciliation behavior is exercised through scripted test-only
providers. Runtime simulator and fault scenarios remain deferred to PR 16. Real
platform execution and real provider reconciliation integrations remain
deferred.
