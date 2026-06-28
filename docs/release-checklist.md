# v0.1.0-alpha Release Checklist

Use this checklist before publishing the first alpha.

## Local Verification

- [ ] `go test ./...` passes.
- [ ] `go run ./cmd/vanish` starts.
- [ ] Demo import works.
- [ ] Parsed item scrolling works.
- [ ] Type filters work.
- [ ] Actor and target filters work.
- [ ] Older/newer date filters work.
- [ ] Item selection works with `Enter`.
- [ ] Item selection works with `Space`.
- [ ] Selection summary can select visible items.
- [ ] Selection summary can deselect visible items.
- [ ] Plan generation includes supported actions and skipped unsupported items.
- [ ] Plan export writes readable JSON.
- [ ] Plan load shows summary and actions.
- [ ] `Ctrl+Q` and `Ctrl+C` open quit confirmation.
- [ ] Plain `q` does not quit.
- [ ] `Esc` goes back.
- [ ] Backspace goes back when no text input is focused.
- [ ] Backspace edits focused text inputs.
- [ ] `?` opens help.

## Safety Verification

- [ ] No network calls were added.
- [ ] No browser automation was added.
- [ ] No credentials are collected.
- [ ] No cookies, tokens, sessions, or passwords are stored.
- [ ] No raw private messages are stored.
- [ ] No apply/deletion logic was added.
- [ ] README matches the actual app behavior.
- [ ] Safety docs match the actual app behavior.

Suggested static check:

```bash
rg -n "net/http|http\\.Get|http\\.Post|NewRequest|chromedp|selenium|playwright|agouti|rod|webdriver" --glob "*.go"
```

## v0.2 Local Workspace Verification

Use this section for local workspace validation work. It is not a v0.2 release
announcement.

- [ ] Local data screens match the documented app directory behavior.
- [ ] Recent import history records only allowed path and summary metadata.
- [ ] Recent plan history records only allowed path and summary metadata.
- [ ] Audit hooks record local workspace events without raw parsed content.
- [ ] Wipe flow clears Vanish-managed config, history, and audit records from
  the active app directory.
- [ ] Wipe flow does not remove user export ZIPs or plan JSON files outside the
  active app directory.
- [ ] `VANISH_APP_DIR` points the app at an isolated development workspace.
- [ ] No network calls were added for local workspace features.
- [ ] No browser automation was added for local workspace features.
- [ ] No credentials, cookies, tokens, sessions, or authorization data are
  collected or stored.
- [ ] No raw parsed items, raw exports, or raw comments are stored in the app
  directory.
- [ ] Static checks cover network, browser automation, credential, and
  authorization handling.

Suggested v0.2 static checks:

```bash
rg -n "net/http|http\\.Get|http\\.Post|NewRequest|chromedp|selenium|playwright|agouti|rod|webdriver" --glob "*.go"
rg -n "password|cookie|token|session|authorization|Authorization|oauth|credential" --glob "*.go"
rg -n "VANISH_APP_DIR|UserConfigDir|UserHomeDir|XDG_CONFIG_HOME|Application Support|APPDATA" --glob "*.go"
```

## GitHub Release

- [ ] Commit alpha polish work on `main`.
- [ ] Push `main` to `origin`.
- [ ] Tag `v0.1.0-alpha`.
- [ ] Push `v0.1.0-alpha`.
- [ ] Create GitHub release titled `v0.1.0-alpha`.
- [ ] Release notes mention alpha status, dry-run-only behavior, and unsupported features.
- [ ] GitHub README renders screenshots/GIF.
- [ ] Install/run docs render correctly.
- [ ] Issue templates are visible.

## Release Notes Skeleton

```markdown
Vanish v0.1.0-alpha is the first local-only dry-run planning release.

Supported:
- Instagram export ZIP import.
- Demo import with fake local data.
- Item browsing, filtering, selection, dry-run plan export, and plan loading.

Safety limits:
- No deletion/apply mode.
- No login.
- No browser automation.
- No network requests.
- No credentials collected.
```
