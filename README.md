# Vanish

Vanish is an open-source, local-first terminal app for reviewing social media
activity and building safe cleanup plans.

Current status: **v0.1.0-alpha**. The app is useful for local review and dry-run
planning, but it does not delete anything yet.

![Vanish home screen](docs/assets/home.svg)

![Vanish parsed items screen](docs/assets/parsed-items.svg)

![Vanish alpha demo flow](docs/assets/vanish-alpha-demo.gif)

## What Vanish Is

- A local terminal UI for reviewing exported social activity.
- A dry-run planner for cleanup actions you may want to take later.
- A privacy-focused alternative to tools that require account access too early.
- Open source and designed around inspectable local files.

## What Vanish Is Not

- Vanish does not make you invisible on the internet.
- Vanish does not log in to Instagram.
- Vanish does not use browser automation, scraping, private APIs, or cloud jobs.
- Vanish does not delete, unlike, unfollow, or apply changes in this alpha.

## Local-First Safety

- No cloud backend.
- No telemetry by default.
- No credentials collected.
- No passwords, cookies, tokens, sessions, or raw private messages in plan files.
- Local Instagram export ZIPs are read from disk only.
- Cleanup plans are dry-run JSON files you can inspect before doing anything else.

See [docs/safety.md](docs/safety.md) for the longer safety model.

## Supported Today

- Instagram export ZIP import.
- Demo import with fake local data.
- Parsed item browsing.
- Filtering by item type, actor, target, and date.
- Review selection.
- Dry-run plan generation.
- Plan export to JSON.
- Plan loading and viewing.

## Not Supported Yet

- Automatic deletion or apply/execution.
- Instagram login.
- Browser automation.
- Reddit, X, YouTube, or other platform integrations.
- Cloud sync or hosted accounts.

## Install And Run

Requirements:

- Go 1.26 or newer.
- A terminal that supports interactive TUI apps.

Run from source:

```bash
git clone https://github.com/itsmeares/vanish.git
cd vanish
go run ./cmd/vanish
```

Run tests:

```bash
go test ./...
```

## Try The Demo Import

The demo import creates a temporary fake Instagram export ZIP on your machine.
It includes fake likes, comments, following records, follower records, and
unsupported files so you can test warnings, filters, selection, and plan
generation without using a real export.

```bash
go run ./cmd/vanish
```

Then choose **Demo import with fake local data**.

## Use A Real Instagram Export ZIP

1. Download your Instagram data export from Instagram.
2. Keep the ZIP on your local machine.
3. Run `go run ./cmd/vanish`.
4. Choose **Import Instagram export ZIP**.
5. Type the local path to the ZIP.
6. Review parsed items, warnings, filters, and selection.

Vanish reads local JSON files from the ZIP. It does not contact Instagram.

## Export And Load Plans

To export a dry-run plan:

1. Import demo data or a local Instagram ZIP.
2. Open parsed items.
3. Toggle items with `Enter` or `Space`.
4. Open **Review selection**.
5. Choose **Generate dry-run plan**.
6. Choose **Export JSON**.

The default output path is `vanish-plan.json`.

To load a plan:

1. Start Vanish.
2. Choose **Load cleanup plan**.
3. Enter the local JSON path.
4. Review the plan summary and action list.

## Keybindings

- `Up` / `Down` or `j` / `k`: move.
- `Enter`: primary action; toggles the highlighted parsed item.
- `Space`: toggles the highlighted parsed item.
- `Esc`: back.
- `Backspace`: back when no text input is focused.
- `?`: help screen.
- `Ctrl+Q` or `Ctrl+C`: quit confirmation.

Plain `q` does not quit.

## Release Prep

See [docs/release-checklist.md](docs/release-checklist.md) for the v0.1.0-alpha
release checklist.

## License

AGPL-3.0.

Vanish is not affiliated with Instagram, Meta, Redact, Reddit, X, YouTube, or
any supported or planned platform.
