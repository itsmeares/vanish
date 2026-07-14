# Local Data

Vanish keeps its workspace on your machine. It does not upload the workspace to
a hosted account or cloud backend.

## App Directory

Default locations:

- Windows: `%APPDATA%\vanish`
- macOS: `$HOME/Library/Application Support/vanish`
- Linux: `${XDG_CONFIG_HOME:-$HOME/.config}/vanish`

Imported Instagram export ZIPs and exported cleanup plans stay at the paths you
choose. Vanish records limited metadata about those paths but does not copy raw
exports into its app directory.

## What Vanish Stores

- Local configuration and workspace timestamps.
- Recent import metadata: selected path, time, platform/source type, counts,
  skipped records, warnings, and safe error summaries.
- Recent plan metadata: selected path, plan creation time, last local action,
  and summary counts.
- Audit events for imports, plans, manual cleanup, local-data views, and wipes.
- Manual-cleanup progress: safe action/target references, timestamps, position,
  and done/skipped outcomes so a session can resume after restart.
- Durable no-op execution progress: an immutable cleanup-plan snapshot, execution
  route and policy, append-only runtime events, and a derived list summary.

## Durable Executions

Durable simulation state lives under `executions/` in the app directory. Each
execution uses a SHA-256-derived directory name rather than accepting a path from
the plan or user input. Its files are:

- `manifest.json`: immutable execution identity and cleanup-plan snapshot.
- `journal.jsonl`: authoritative, append-only lifecycle and action records.
- `summary.json`: a derived cache used for fast Local Data listing.
- `writer.lock`: the cross-process exclusive-writer lock.

Creation identity locks and retained fingerprint guards live in
`executions/.identity-locks/`. On POSIX systems, Vanish creates execution
directories with `0700` permissions and files with
`0600` permissions. It rejects symlinked execution roots, session directories,
journals, manifests, summaries, and lock files rather than following them.

The Local Data > Executions screen classifies entries as resumable, waiting for
a retry time, waiting for provider readiness, resolution required, terminal,
corrupt, or active in another process. Resume is always explicit. An attempt
whose result was not durably recorded can only be abandoned, never retried.
Terminal and corrupt entries can be explicitly deleted from this screen.
Deleting a terminal execution retains its identity guard, so the same
plan/route/policy fingerprint cannot silently start again after journal removal.
Corrupt deletion requires a trustworthy manifest or retained summary fingerprint;
otherwise the execution remains stored.

## What Vanish Does Not Store

- Raw parsed items, export files, comment text, or private messages.
- Passwords, cookies, login sessions, or authorization headers.
- Reddit token values in normal configuration, history, audit, plans, or errors.

## Reddit Secret Storage

When the experimental Reddit prototype is available, the operating-system
credential store is the primary refresh-token store. If that store is
unavailable, Vanish can use a local file fallback only after explicit user
confirmation. Fallback secrets live in `secrets/` under the active app
directory. Vanish creates that directory with `0700` permissions and its secret
files with `0600` permissions.

When the credential store becomes available, Vanish migrates fallback secrets to
it and removes the local fallback copy. Normal config, recent history, audit
logs, cleanup plans, and errors never contain token values. Non-secret Reddit
connection metadata may remain in local configuration.

## Wipe Behavior

Local-data wipe clears Vanish-managed configuration, recent import/plan history,
audit records, manual-cleanup progress, durable execution manifests, journals,
summaries, locks, and any confirmed local-file fallback secrets from the active
app directory. Wipe fails safely while a durable execution writer is active and
prevents new writers for the duration of the wipe. It does not delete user-owned
export ZIPs or plan JSON files
outside that directory. Operating-system credential-store secrets are outside
the app directory and are not removed by local-data wipe.

For development or tests, point Vanish at an isolated workspace:

```bash
VANISH_APP_DIR=/tmp/vanish-dev go run ./cmd/vanish
```

```powershell
$env:VANISH_APP_DIR = "$env:TEMP\vanish-dev"
go run ./cmd/vanish
```
