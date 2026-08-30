# Vanish

Vanish is a local-first desktop app for finding and cleaning up Instagram activity. The current app supports one complete workflow: Instagram likes.

## What works

- Separate Instagram accounts with persistent local browser sessions
- Scanning and deduplicating liked posts, reels, and carousels into SQLite
- Search, type and date filters over large libraries
- Virtualized browsing for 100,000+ items
- Explicit selection or all-filtered selection
- An immutable cleanup snapshot at confirmation
- Real unlike requests through the user's authenticated Instagram page context
- Pause, resume, rate-limit waits, sign-in recovery, and restart recovery
- Read-only reconciliation before retrying any unlike with an uncertain result
- Local result summaries

Vanish has no server, telemetry, or automatic crash upload. It does not ask for Instagram passwords, cookies, or tokens. Reddit and X are not part of this rewrite.

## Development

Requirements: Node.js 24 and npm 12.

```sh
npm install
npm run dev
```

Validation:

```sh
npm run lint
npm run typecheck
npm test
npm run build
npm run package
git diff --check
```

`npm run package` creates an unpacked application under `release/`. It does not publish or create a release.

## Instagram compatibility

Instagram does not publish the web interfaces used by its own frontend for this workflow. Vanish calls those interfaces only inside the user's isolated, authenticated Instagram browser session. The implementation was checked against Instagram's public web client on 30 August 2026, including its current `PolarisAPIUnlikePost` mutation and media-info request. No authenticated account was available in the development environment, so maintainers must complete the short [real-account smoke test](docs/instagram-smoke-test.md) before treating the current build as live-verified.

Vanish does not bypass verification, CAPTCHA, or rate limits. When Instagram asks for attention, Vanish pauses and shows the normal Instagram page.

## Local data

Electron stores `vanish.sqlite` and each account's persistent browser partition in the operating system's normal app-data directory. Account rows, activity rows, and cleanup rows are keyed by local account ID. Confirmed cleanup targets cannot be inserted, deleted, or retargeted by the application after confirmation.

See [safety and recovery](docs/safety.md) for the failure rules.

## License

AGPL-3.0-only. Vanish is not affiliated with Instagram or Meta.
