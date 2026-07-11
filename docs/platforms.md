# Platform Support

Vanish uses a small platform registry so the TUI can show current and planned
support honestly.

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

| Platform | Status | Comments/posts | Saved items | Votes | Dry-run plans | Apply cleanup | OAuth | Network/API access |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Instagram Export | prototype | n/a | n/a | n/a | prototype | no | no | no |
| Reddit | prototype | prototype | planned | planned | prototype | later | prototype | prototype |

## Instagram Export

Instagram Export remains a prototype platform. Vanish can read a local export
ZIP, normalize supported activity records, show parsed items for review, and
generate a local dry-run cleanup plan. It can also guide export requests and
step through supported plan actions for manual completion in the user's browser.

Vanish does not log in to Instagram, call Instagram APIs, automate a browser,
delete platform content, or automatically apply account changes.

## Reddit

Official API planner prototype targets v0.5.

Reddit now has a TUI-accessible prototype for installed-app OAuth, secure
refresh-token storage, official API requests, own comments/posts scanning, and
local dry-run planning.

Implemented prototype flow:

- Use installed-app OAuth with `identity history` scopes.
- Connect through the TUI with manual OAuth: Vanish shows the authorization URL
  and accepts the returned code or redirect URL.
- Store refresh tokens through the Vanish secret store, not normal config.
- Scan own comments and submitted posts through Reddit's official API.
- Normalize supported activity into Vanish activity items for the existing
  review, filter, and selection screens.
- Generate local dry-run plans with Reddit-specific planned actions.

Deferred directions:

- Scan saved items.
- Scan vote history.
- Apply, delete, edit, save, vote, submit, or permission-changing behavior.

Real apply cleanup is later. The v0.6 foundation can preview and confirm a plan
through a no-op executor, but it must not be implied to delete or change
platform content today.

## Safety Boundaries

- Vanish reads local files you choose.
- Vanish stores only local app metadata and dry-run plan files.
- Vanish stores OAuth refresh tokens only through the configured secret store.
  Normal config stores only non-secret metadata.
- Vanish does not collect passwords, cookies, pasted sessions, private API data,
  or authorization headers.
- Vanish does not delete platform content or apply account changes.
- Apply preview/no-op execution records safe lifecycle events only.
- Local data wipe only clears Vanish-managed local app metadata in the active
  app directory. It does not delete platform content.
