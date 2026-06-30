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
| Reddit | planned | planned | planned | planned | planned | later | planned | no |

## Instagram Export

Instagram Export remains a prototype platform. Vanish can read a local export
ZIP, normalize supported activity records, show parsed items for review, and
generate a local dry-run cleanup plan.

Vanish does not log in to Instagram, call Instagram APIs, automate a browser,
delete platform content, or apply account changes.

## Reddit

Official API planner planned for v0.5.

Reddit is a planned platform in v0.4. Vanish does not implement Reddit OAuth,
tokens, API clients, network access, scraping, browser automation, or cleanup
apply behavior.

Planned capability labels:

- Scan own comments/posts: planned.
- Scan saved items: planned.
- Scan votes: planned.
- Generate dry-run plans: planned.
- Apply cleanup: later.
- OAuth: planned.
- Network/API access: not implemented in v0.4.

The intended v0.5 direction is the official Reddit API planner. Do not imply
that Reddit works today.

## Safety Boundaries

- Vanish reads local files you choose.
- Vanish stores only local app metadata and dry-run plan files.
- Vanish does not store Reddit OAuth refresh tokens in v0.4 because Reddit
  OAuth is not implemented.
- Vanish does not collect passwords, cookies, pasted sessions, private API data,
  or authorization headers.
- Vanish does not delete platform content or apply account changes.
- Local data wipe only clears Vanish-managed local app metadata in the active
  app directory. It does not delete platform content.
