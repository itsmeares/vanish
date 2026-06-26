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
