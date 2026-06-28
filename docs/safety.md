# Vanish Safety Model

Vanish helps people review and plan cleanup of social activity. It does not make
anyone invisible online, and the current alpha does not apply changes to any
platform.

## Current Alpha Guarantees

- Dry-run only.
- Local files only.
- No cloud backend.
- No telemetry by default.
- No platform login.
- No browser automation.
- No private API calls.
- No deletion or account changes.

## Data Vanish Avoids Storing

Plan files and normalized activity items must not store:

- Passwords.
- Cookies.
- Access tokens.
- Session IDs.
- Recovery codes.
- Raw private message bodies.
- Raw comment bodies when a safe hash is enough.

The Instagram importer normalizes records into safe item metadata, target
references, timestamps, and optional safe text hashes.

## Local Workspace Privacy Rules

The v0.2 local workspace stores only Vanish-managed app state on the user's
machine. This includes configuration, recent import history, recent cleanup plan
history, and audit records for local workspace events.

Allowed local history and audit metadata:

- Local file paths selected by the user.
- Import timestamps and summary counts.
- Plan timestamps and summary counts.
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
- Session IDs.
- Authorization headers, OAuth grants, or other authorization data.

If future features need more sensitive data, they should require a separate,
explicit design review and user-facing disclosure before implementation.

## Plan Files

Cleanup plans are local JSON documents. In v0.1.0-alpha they are dry-run only:

- They can be exported.
- They can be loaded and inspected.
- They cannot be executed by Vanish.

Treat plan files as review artifacts. They may still include usernames, target
URLs, target IDs, timestamps, and action intent.

## Future Apply Risk

Future apply modes may carry platform and account risk:

- Platforms can rate-limit or flag automated behavior.
- Some cleanup actions may be irreversible.
- Login/session handling must stay local and explicit.
- Users must be able to inspect, revoke, or wipe stored sessions.

Any future apply mode should be clearly labeled as experimental until proven
safe.

## Reporting Safety Issues

Do not paste secrets, raw exports, cookies, tokens, or private messages into
GitHub issues. Use fake data or describe the shape of the problem.
