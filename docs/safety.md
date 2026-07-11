# Vanish Safety Model

Vanish helps people review and plan cleanup of social activity. It does not make
anyone invisible online, and it does not automatically apply Instagram changes.

## Current Alpha Guarantees

- Dry-run only.
- Local files by default, with the explicit Reddit official API prototype
  exception described below.
- No cloud backend.
- No telemetry by default.
- No platform password login.
- No browser automation.
- No private API calls.
- No automatic Instagram deletion or account changes.
- Assisted Instagram cleanup opens one trusted target after explicit selection;
  the user performs any change in their browser.
- Apply preview and confirmation may run a no-op executor for lifecycle testing.

## Reddit v0.5 Official API Boundary

Vanish v0.5 adds an official Reddit API planner prototype. This is the only
exception to the current no-network rule, and it stays narrow:

- Official Reddit OAuth and OAuth API calls only.
- Network code only in reviewed Reddit OAuth/API files.
- Installed-app OAuth only, with an environment-provided client ID.
- Minimum read-only scopes for the prototype: `identity` and `history`.
- No Reddit content or account activity mutations.
- No real apply, delete, edit, save, unsave, vote, submit, or
  permission-changing behavior.
- No browser automation, embedded browser, webview, scraping, private APIs,
  password collection, or cookie/session paste flow.
- OAuth token revoke is allowed only as an explicit disconnect/auth cleanup
  action. It must not run silently and is not a content cleanup action.

Reddit tokens must never be stored in normal config, logs, audit logs, cleanup
plans, recent history, or errors. If the system credential store is available,
Vanish must use it. If unavailable, a local token file fallback may be used only
after explicit user confirmation, only inside the Vanish app directory, with a
`0700` directory and `0600` file. If the credential store later becomes
available, the fallback token must migrate to it and the fallback file must be
deleted after successful migration.

Allowed non-secret Reddit metadata in config includes the Reddit username,
connection timestamp, requested scopes, token storage mode, credential store
mode, and expiry metadata.

## Data Vanish Avoids Storing

Plan files and normalized activity items must not store:

- Passwords.
- Cookies.
- Access tokens.
- Login or browser session IDs.
- Recovery codes.
- Raw private message bodies.
- Raw comment bodies when a safe hash is enough.

The Instagram importer normalizes records into safe item metadata, target
references, timestamps, and optional safe text hashes. Short comment previews
remain memory-only.

## Local Workspace Privacy Rules

The v0.2 local workspace stores only Vanish-managed app state on the user's
machine. This includes configuration, recent import history, recent cleanup plan
history, assisted manual-cleanup progress, and audit records for local workspace
events.

Allowed local history and audit metadata:

- Local file paths selected by the user.
- Import timestamps, total counts, per-type counts, skipped counts, and warning
  counts.
- Plan creation timestamps, last-used timestamps, last local operations, and
  summary counts.
- Platform, source type, action type, and warning/error summaries.
- Stable IDs or hashes when needed to reconnect app state without storing raw
  content.

The local workspace must not store:

- Raw parsed items.
- Raw export files.
- Raw comment text.
- Raw private messages.
- Credentials.
- Cookies.
- Access tokens.
- Login or browser session IDs.
- Authorization headers, OAuth grants, or other authorization data.

If future features need more sensitive data, they should require a separate,
explicit design review and user-facing disclosure before implementation.

## Plan Files

Cleanup plans are local JSON documents. In v0.1.0-alpha they are dry-run only:

- They can be exported.
- They can be loaded and inspected.
- They can be previewed through the no-op apply foundation.
- No no-op execution changes platform content or account state.

Treat plan files as review artifacts. They may still include usernames, target
URLs, target IDs, timestamps, and action intent.

Assisted manual cleanup does not mutate plan files. Its separate local progress
contains safe action references and done/skipped outcomes, never raw comment
text.

## Future Apply Risk

Future real apply modes may carry platform and account risk:

- Platforms can rate-limit or flag automated behavior.
- Some cleanup actions may be irreversible.
- Login/session handling must stay local and explicit.
- Users must be able to inspect, revoke, or wipe stored sessions.

Any future real apply mode should be clearly labeled as experimental until
proven safe.

## Reporting Safety Issues

Do not paste secrets, raw exports, cookies, tokens, or private messages into
GitHub issues. Use fake data or describe the shape of the problem.
