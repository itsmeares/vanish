# Platform Support

Vanish tracks platform support separately from the product vision so supported
work and future plans stay clear.

| Platform | Import or scan | Cleanup planning | Assisted cleanup | Automatic cleanup |
| --- | --- | --- | --- | --- |
| Instagram Export | Supported | Supported | Supported | No |
| Reddit | Official API, access pending | Prototype | No | No |
| Other platforms | Planned | Planned | Planned | Planned |

## Instagram Export

Instagram Export is the current alpha path. Vanish opens Meta's official export
page only after explicit user selection, reads a local JSON export ZIP chosen by
the user, and accepts supported records from partial exports.

The app can review, filter, and select activity; create a local cleanup plan;
and guide manual unfollow, unlike, and own-comment deletion steps. It can save
safe local progress so a stopped session can resume after restart.

Vanish does not log in to Instagram, call Instagram APIs, scrape, automate a
browser, delete platform content, or automatically apply account changes.

## Reddit

Reddit is an experimental read-only official API planner prototype. A developer
access request has been submitted to Reddit, and the project is awaiting a
response. The integration may therefore not be usable in public builds yet.
Approval has not been granted.

The prototype's documented boundary is installed-app OAuth with `identity
history` scopes, official API access for own comments and submitted posts, and
local review/filter/select/plan work. It has no assisted cleanup or mutations.
Saved items, vote history, and all content or account changes remain planned.

Reddit does not block the Instagram alpha release. It must not add scraping,
browser automation, private APIs, password collection, cookie/session paste, or
automatic platform actions.

## Other Platforms

Other platforms are planned only. They have no importer, account flow, cleanup
planner, or execution path in this release.

## Safety Boundaries

- Instagram import reads local files selected by the user.
- Reddit network activity, where access is available, is limited to the
  documented official OAuth/API planner boundary.
- Vanish stores local workspace metadata and cleanup plan files; secret handling
  is limited to the documented Reddit credential-store path.
- Local data wipe clears Vanish-managed local metadata only.
- No supported platform has automatic cleanup.
