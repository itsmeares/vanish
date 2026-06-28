# Local Data

This page documents the v0.2 local workspace behavior. It does not mean v0.2
has been released. Until a v0.2 release is published, the released app remains
the current alpha and keeps the same dry-run limits.

## App Directory

Vanish stores local workspace state in a Vanish app directory on the user's
machine. This directory is for app-managed state only. It is not a cloud sync
folder, and Vanish does not upload it.

Default app directory locations:

- Windows: `%APPDATA%\vanish`
- macOS: `$HOME/Library/Application Support/vanish`
- Linux: `${XDG_CONFIG_HOME:-$HOME/.config}/vanish`

Imported Instagram export ZIPs and exported cleanup plans stay at the local
paths selected by the user. Vanish records limited metadata about those paths,
but it does not copy raw exports into the app directory.

## Configuration

Configuration in the app directory is limited to local app settings needed to
restore the workspace. It must not include platform credentials, cookies,
tokens, sessions, authorization headers, OAuth grants, or account secrets.

## Recent Import History

Recent import history helps users return to prior local review work. History may
store:

- The local path selected by the user.
- Import time.
- Platform and source type.
- Item counts and warning counts.
- Error summaries.

Recent import history must not store raw parsed items, raw export files, raw
comments, raw private messages, credentials, cookies, tokens, sessions, or
authorization data.

## Recent Plan History

Recent plan history helps users reopen dry-run cleanup plans. History may store:

- The local plan path selected by the user.
- Plan load or export time.
- Action counts and skipped item counts.
- Platform and plan summary metadata.

Cleanup plan JSON files are still dry-run review artifacts. Vanish does not
execute them in the current alpha.

## Audit Log

The audit log records local workspace events so users can inspect what Vanish
did locally. Audit entries may include events such as:

- Import started, completed, or failed.
- Plan exported.
- Plan loaded.
- Local data viewed.
- Local data wiped.

Audit entries should use timestamps, event names, paths, counts, and safe
summaries. They must not include raw parsed content, raw comments, raw exports,
credentials, cookies, tokens, sessions, or authorization data.

## Wipe Behavior

The local data wipe flow clears Vanish-managed state from the active app
directory:

- Local configuration.
- Recent import history.
- Recent plan history.
- Audit log records.

The wipe flow does not delete user-owned Instagram export ZIPs or cleanup plan
JSON files outside the active app directory. If a developer points
`VANISH_APP_DIR` at a disposable directory and writes files inside that
directory, those files should be treated as part of the disposable workspace.

## Development Override

Set `VANISH_APP_DIR` to override the default app directory during development,
manual testing, or automated tests. Use a disposable path so wipe testing cannot
touch normal user state.

```bash
VANISH_APP_DIR=/tmp/vanish-dev go run ./cmd/vanish
```

PowerShell:

```powershell
$env:VANISH_APP_DIR = "$env:TEMP\vanish-dev"
go run ./cmd/vanish
```

When `VANISH_APP_DIR` is set, all local workspace reads, writes, history, audit,
and wipe behavior should be scoped to that directory.

## Privacy Boundary

The local workspace boundary is intentionally narrow. Vanish may remember where
the user worked and safe summaries of what happened, but it must not persist raw
social content or authentication material.

Allowed metadata includes local paths, timestamps, counts, warning summaries,
platform names, source types, action types, and stable IDs or hashes when needed
to reconnect app state.

Disallowed data includes raw parsed items, raw exports, raw comments, raw private
messages, passwords, credentials, cookies, tokens, sessions, authorization
headers, OAuth grants, and account secrets.
