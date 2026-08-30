# Safety and recovery

Instagram pages run in sandboxed Electron renderers with Node integration off, context isolation on, no preload bridge, denied permission requests, and restricted navigation. The Vanish renderer gets named IPC methods only. The main process validates its sender and every identifier, filter, selection, and resolution crossing that boundary.

Each Instagram account uses its own persistent Electron session partition. Vanish never copies session cookies into its database or renderer. Sessions remain local in Electron's app-data directory.

## Confirmed cleanup

Confirmation creates a job and copies the selected activity IDs, media IDs, shortcodes, permalinks, and order into `job_items` inside one SQLite transaction. SQLite triggers reject later target inserts, deletes, or target-field changes. A later scan cannot widen that job.

The runner processes one item at a time:

1. Save `in_flight` and the attempt number before calling Instagram.
2. Call Instagram from that account's authenticated page context.
3. Save a definite success before moving to the next item.

If the app closes after step 1, startup changes that item to `ambiguous` and the job to `needs_reconciliation`. Resume checks the exact media ID with Instagram's read-only media-info request. It marks an unliked item complete, retries only when Instagram still reports it liked, and stops for a user decision when it cannot tell.

Rate limits become a timed wait. Authentication and verification open the normal Instagram window. Network loss pauses the job. Vanish never retries an uncertain destructive request without reconciliation.

## Local-only defaults

There is no telemetry, hosted account, automatic crash upload, credential form, cookie import, or session export. Thumbnails load from Instagram CDN hosts with no referrer. Vanish does not attempt CAPTCHA solving, stealth, fingerprint changes, or rate-limit evasion.
