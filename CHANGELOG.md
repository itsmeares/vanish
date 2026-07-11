# Changelog

All notable changes to Vanish are documented here.

## v0.1.0-alpha

### Highlights

- Local Instagram export ZIP import with support for usable partial exports.
- Large-export performance work and bounded warnings.
- Activity review, filtering, and selection.
- Local cleanup-plan generation, export, and loading.
- Assisted manual unfollow, unlike, and own-comment deletion flows.
- Stop-and-resume cleanup sessions.
- Local workspace metadata and audit history.

### Instagram workflow

- Request Meta export guidance after explicit user selection.
- Read selected local JSON export ZIPs without Instagram login, API calls,
  scraping, or browser automation.
- Open supported cleanup targets only when the user chooses to continue; the
  user performs each change in their browser.

### Local-first behavior

- No cloud backend or default telemetry.
- Local history, audit, workspace management, and local-data wipe.
- No raw comment persistence; comment previews remain memory-only.

### Reddit prototype

- Experimental read-only official API planner prototype for own comments and
  submitted posts.
- Developer access is pending, so it may not be usable in public builds.
- No Reddit mutations or automated cleanup.

### Known limitations

- Instagram changes are performed manually in the browser.
- Instagram export formats may change.
- Comment previews are memory-only and disappear after restart.
- Reddit remains an experimental read-only prototype awaiting developer access.
- No automatic platform mutations or browser automation.
