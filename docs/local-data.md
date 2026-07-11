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
audit records, manual-cleanup progress, and any confirmed local-file fallback
secrets from the active app directory. It does not delete user-owned export ZIPs
or plan JSON files outside that directory. Operating-system credential-store
secrets are outside the app directory and are not removed by local-data wipe.

For development or tests, point Vanish at an isolated workspace:

```bash
VANISH_APP_DIR=/tmp/vanish-dev go run ./cmd/vanish
```

```powershell
$env:VANISH_APP_DIR = "$env:TEMP\vanish-dev"
go run ./cmd/vanish
```
