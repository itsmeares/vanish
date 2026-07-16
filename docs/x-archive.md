# X Archive

Vanish can import an official X/Twitter archive ZIP and keep a narrowed,
read-only post dataset for local browsing after restart. It does not connect to
X, authenticate, scrape, automate a browser, open remote post targets, create
cleanup plans, or change the account.

## Request and Import

Choose **X / Twitter Archive** on Home. The available actions are:

- **Request archive** opens X's official archive settings page after explicit
  selection.
- **Choose archive ZIP** opens Vanish's local ZIP picker.
- **Try demo archive** generates a synthetic archive at runtime and exercises
  the same parser and storage path.

The original ZIP remains user-owned, unmodified, and outside Vanish's app
directory. It can be moved or deleted after a successful import without
breaking the retained dataset.

## Supported Data

The current compatibility boundary is intentionally narrow:

- `data/account.js` using `window.YTD.account.part0`.
- `data/tweets.js` using `window.YTD.tweets.part0`.
- Optional `data/community-tweet.js` using
  `window.YTD.community_tweet.part0`.
- Photo, MP4 video, and MP4 animation files referenced by accepted records in
  the corresponding current-post media directories.

Vanish normalizes accepted activity as a post, reply, quote post, or repost.
Reply classification takes precedence over quote classification while retaining
the quote relationship. Reposts require the archive's structural leading
`RT @account:` form and matching mention metadata. Quote URLs use trusted
X/Twitter status URL shapes and UTF-16 entity offsets.

Deleted posts are not imported. Notes, articles, likes, follows, direct
messages, ads, and every other unsupported archive section are deferred and not
retained.

## Local Storage and Browsing

Each deterministic dataset lives under
`x-archives/<dataset-sha256>/` in the Vanish app directory. Manifests and the
browser index contain no post text. Full normalized text is confined to
`posts.jsonl`; referenced media is content-addressed under `media/`.

The Review tab opens the newest retained X dataset when no in-memory Instagram
or Reddit review is active. Local Data > X archives lists all retained datasets.
The list is newest-first and read-only. Enter opens a scrollable full-text
detail; Tab exposes explicit **Open photo**, **Open video**, or **Open
animation** actions for retained local files. Vanish does not expose selection,
filters, plans, or remote post actions on these screens.

Local-data wipe deletes retained X datasets and in-memory X state. It never
deletes the original archive ZIP outside the app directory.

## Security and Resource Limits

Import rejects unsafe absolute, traversal, backslash, drive-qualified,
duplicate, case-colliding, and symbolic-link ZIP entries. Supported entries are
CRC-checked by the ZIP reader and constrained by these upper bounds:

- Archive file: 32 GiB.
- ZIP entries: 250,000.
- Accepted activities: 5,000,000.
- Supported current-post JS entry: 2 GiB.
- Full text per activity: 1 MiB.
- Retained media item: 1 GiB.
- Retained media references per activity: 64.
- Total retained unique media: 20 GiB.
- Supported-entry uncompressed/compressed ratio: 200:1.

Imports stage files inside the app directory and publish a completed dataset by
atomic directory rename. Each normalized record and the index are hashed;
records are revalidated when selected after restart. Warning groups store only
source categories, structural reasons, units, and counts.
