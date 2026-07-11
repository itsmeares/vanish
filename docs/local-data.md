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
- Passwords, credentials, cookies, login sessions, or authorization headers.
- Reddit token values in normal configuration, history, audit, plans, or errors.

Reddit refresh tokens, when that experimental prototype is available, use the
configured secret store. Non-secret Reddit connection metadata may remain in
local configuration.

## Wipe Behavior

Local-data wipe clears Vanish-managed configuration, recent import/plan history,
audit records, and manual-cleanup progress from the active app directory. It
does not delete user-owned export ZIPs or plan JSON files outside that
directory.

For development or tests, point Vanish at an isolated workspace:

```bash
VANISH_APP_DIR=/tmp/vanish-dev go run ./cmd/vanish
```

```powershell
$env:VANISH_APP_DIR = "$env:TEMP\vanish-dev"
go run ./cmd/vanish
```
