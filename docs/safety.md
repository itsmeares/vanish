# Vanish Safety Model

Vanish helps people review social activity and work through supported cleanup
steps. It does not make anyone invisible online, and it does not automatically
apply Instagram changes.

## Current Alpha Guarantees

- Local-first workspace data; no cloud backend or default telemetry.
- No Instagram password login, API calls, scraping, private APIs, browser
  automation, embedded browsers, or webviews.
- Meta export URLs and Instagram targets open only after explicit user action in
  the system browser.
- Assisted Instagram cleanup guides unfollow, unlike, and own-comment deletion;
  the user performs every platform change.
- No automatic platform mutations.
- No password, cookie, browser-session, or raw-comment persistence.
- Local plans, history, audit, progress, and wipe behavior stay within the
  documented local-data boundary.

## Reddit Official API Boundary

Reddit support is an experimental, read-only official API planner prototype.
Developer access is pending and may not be usable in public builds. It is the
only exception to the local-file-only model:

- Official Reddit OAuth/API calls only, limited to reviewed Reddit code.
- Installed-app OAuth only, with the minimum `identity` and `history` scopes.
- No Reddit content/account mutation, including delete, edit, save, unsave,
  vote, submit, or permission-changing behavior.
- No scraping, browser automation, embedded browser, webview, private APIs,
  password collection, or cookie/session paste flow.
- Refresh tokens use the configured secret store. Normal config, plans,
  history, audit logs, and errors must not store token values or authorization
  headers.

## Reddit Secret Storage

The operating-system credential store is the primary Reddit refresh-token store.
If it is unavailable, Vanish uses a local-file fallback only after explicit user
confirmation. The fallback is under the Vanish app directory in `secrets/`;
Vanish creates that directory with `0700` permissions and secret files with
`0600` permissions.

When the credential store becomes available, Vanish saves the token there and
removes any fallback copy. Normal config, recent history, audit logs, cleanup
plans, errors, and diagnostics never contain token values.

## Data Vanish Avoids Storing

Plans and normalized activity must not store passwords, cookies, access tokens,
login/browser session IDs, recovery codes, raw private messages, or raw comment
bodies when a safe hash is enough. Short comment previews are memory-only.

## Local Workspace Privacy Rules

The app directory can retain user-selected local paths, timestamps, counts,
safe warning/error summaries, local plan metadata, audit events, safe
manual-cleanup progress, and an explicitly confirmed `secrets/` fallback when
the operating-system credential store is unavailable. It must not retain raw
parsed items, raw exports, raw comments, private messages, passwords, cookies,
session data, or authorization headers.

The local-data wipe flow clears Vanish-managed configuration, recent history,
audit records, manual-cleanup progress, and local-file fallback secrets from the
active app directory. It does not delete a user-owned export ZIP or plan JSON
outside that directory, and it does not remove secrets held by the
operating-system credential store.

## Reporting Safety Issues

Do not paste secrets, raw exports, cookies, tokens, usernames, comments, or
private messages into GitHub issues. Use synthetic data or describe the shape of
the problem.
