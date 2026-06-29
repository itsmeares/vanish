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

| Platform | Status | Comments/posts | Saved items | Votes | Dry-run plans | Apply cleanup | OAuth | Network/API access |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Instagram Export | prototype | n/a | n/a | n/a | prototype | no | no | no |
| Reddit | planned | planned | planned | planned | planned | later | planned | not implemented in v0.4 |

## Instagram Export

Instagram Export is the only prototype platform in v0.4. Vanish can read a
local export ZIP, normalize supported activity records, show parsed items for
review, and generate a local dry-run cleanup plan.

Vanish does not log in to Instagram, call Instagram APIs, automate a browser,
delete platform content, or apply account changes.

## Reddit

Official API planner planned for v0.5.

Reddit is planned only in v0.4. The TUI shows integration notes and disabled
placeholder actions, but there is no Reddit OAuth, token storage, API client,
network call, browser automation, scraping path, importer, or planner.

Planned v0.5 directions:

- Scan own comments/posts.
- Scan saved items.
- Scan votes.
- Generate dry-run plans.
- Use OAuth for official API access.

Apply cleanup is later. It is not part of the v0.4 placeholder and should not
be implied to work today.

## Safety Boundaries

- Vanish reads local files you choose.
- Vanish stores only local app metadata and dry-run plan files.
- Vanish does not collect credentials, cookies, tokens, sessions, OAuth grants,
  or authorization headers.
- Vanish does not delete platform content or apply account changes.
- Local data wipe only clears Vanish-managed local app metadata in the active
  app directory. It does not delete platform content.
