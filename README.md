# Vanish

An open-source, local-first TUI for cleaning up your social media footprint.

Vanish helps you review and remove old posts, comments, likes, reposts, follows, saves, and other activity across supported social platforms — from your terminal.

It is designed as a transparent, open-source alternative to Redact.

> Vanish does not make you invisible on the internet.
> It helps you find, review, and reduce your old online traces.

## Why?

Most social cleanup tools are closed-source, expensive, or require too much trust.

Vanish is different:

* Local-first
* Open-source
* Terminal-native
* No cloud backend
* No telemetry by default
* No password or cookie upload

## How it works

```txt
scan → review → plan → apply → audit
```

Vanish scans or imports your activity, lets you review and filter it, generates a cleanup plan, and applies supported actions only after confirmation.

Nothing should happen silently.

## Platform support

| Platform    | Status  | Method                               |
| ----------- | ------- | ------------------------------------ |
| Instagram   | Planned | Data export + assisted local cleanup |
| Reddit      | Planned | Official API                         |
| X / Twitter | Planned | Official API where available         |
| YouTube     | Planned | Official API / export import         |

## Current focus

The first version focuses on the local planning flow:

* Import social data exports
* Parse supported activity
* Review items in a TUI
* Filter old activity
* Generate a dry-run `plan.json`

Early versions may not delete anything yet. That is intentional.

## Install

Vanish is not ready for general use yet.

For development:

```bash
git clone https://github.com/itsmeares/vanish.git
cd vanish
go mod tidy
go run ./cmd/vanish
```

## Development

Built with:

* Go
* Bubble Tea
* Bubbles
* Lip Gloss

Run tests:

```bash
go test ./...
```

## Safety

Vanish should never upload credentials or store them in plain text.

Cleanup plans and audit logs should not contain:

* Passwords
* Raw cookies
* Access tokens
* Session data
* Recovery codes
* Private message content

If a future integration needs a logged-in session, it should be handled locally and explicitly:

* The user logs in manually
* The session stays on the user’s machine
* Secrets are stored only in the OS keychain or an encrypted local vault
* The user can inspect, revoke, or wipe stored sessions
* Sessions are never synced or sent to a remote location

Some platforms do not provide official APIs for every cleanup action. Those integrations will be clearly marked as experimental.

## Roadmap

* Local export import
* TUI review and filtering
* Dry-run cleanup plans
* Local audit logs
* Reddit official API support
* Safe apply engine
* Experimental assisted cleanup modes

## License

AGPL-3.0.

Vanish is not affiliated with Redact.
