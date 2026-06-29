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

Instagram Export is the only prototype platform in v0.4. Vanish can read a
local export ZIP, normalize supported activity records, show parsed items for
review, and generate a local dry-run cleanup plan.

Vanish does not log in to Instagram, call Instagram APIs, automate a browser,
delete platform content, or apply account changes.

## Reddit

Official API planner prototype targets v0.5.

Reddit now has foundation code for installed-app OAuth, secure refresh-token
storage, official API requests, own comments/posts scanning, and local dry-run
planning. The TUI connect and scan actions remain disabled until the interactive
workflow is wired.

Implemented prototype foundations:

- Use installed-app OAuth with `identity history` scopes.
- Store refresh tokens through the Vanish secret store, not normal config.
- Scan own comments and submitted posts through Reddit's official API.
- Normalize supported activity into Vanish activity items.
- Generate local dry-run plans with Reddit-specific planned actions.

Deferred directions:

- Scan saved items.
- Scan vote history.
- Wire TUI connect/scan flows.

Apply cleanup is later. It is not part of the v0.5 planner prototype and should
not be implied to work today.

## Safety Boundaries

- Vanish reads local files you choose.
- Vanish stores only local app metadata and dry-run plan files.
- Vanish stores OAuth refresh tokens only through the configured secret store.
  Normal config stores only non-secret metadata.
- Vanish does not collect passwords, cookies, pasted sessions, private API data,
  or authorization headers.
- Vanish does not delete platform content or apply account changes.
- Local data wipe only clears Vanish-managed local app metadata in the active
  app directory. It does not delete platform content.
