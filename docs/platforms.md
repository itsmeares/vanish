# Platform Support

Vanish v0.4 uses a small platform registry so the TUI can show current and
planned support honestly.

## Status Labels

- `prototype`: usable local behavior exists, but the surface is still early.
- `planned`: visible roadmap item only; no working importer, account flow, or
  cleanup planner exists yet.

## Capability Labels

- `yes`: supported in the current local app.
- `prototype`: supported enough for local review or dry-run planning, with
  alpha limits.
- `planned`: intended, but not implemented.
- `later`: intentionally deferred behind earlier safety or design work.
- `no`: not supported in v0.4.

## Matrix

| Platform | Status | Local scan | Review | Dry-run plan | Apply / execution | Login / account auth | Network / API access |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Instagram Export | prototype | prototype local ZIP scan | yes | prototype | no | no | no |
| Reddit | planned | planned | planned | later | no | no | no |

## Instagram Export

Instagram Export is the only prototype platform in v0.4. Vanish can read a
local export ZIP, normalize supported activity records, show parsed items for
review, and generate a local dry-run cleanup plan.

Vanish does not log in to Instagram, call Instagram APIs, automate a browser,
delete platform content, or apply account changes.

## Reddit

Reddit is planned only. The v0.4 TUI shows integration notes and disabled
placeholder actions, but there is no Reddit client, OAuth flow, token storage,
API call, browser automation, scraping path, importer, or planner.

## Safety Boundaries

- Vanish reads local files you choose.
- Vanish stores only local app metadata and dry-run plan files.
- Vanish does not collect credentials, cookies, tokens, sessions, OAuth grants,
  or authorization headers.
- Vanish does not delete platform content or apply account changes.
- Local data wipe only clears Vanish-managed local app metadata in the active
  app directory. It does not delete platform content.
