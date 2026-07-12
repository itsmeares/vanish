# Platform Support

Vanish tracks discovery, review, planning, and cleanup separately. A simulated
execution never changes platform content and is not automatic cleanup.

See [Cleanup Runtime](runtime.md) for typed outcomes, bounded retry, and halt
behavior.

| Platform | Local import | Official scan | Review | Cleanup planning | Assisted cleanup | Simulated execution | Automatic cleanup |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Instagram Export | Supported | Unsupported | Supported | Supported | Supported | No-op only | Unsupported |
| Reddit | Unsupported | Prototype, access pending | Supported | Prototype | Unsupported | No-op only | Unsupported |

## Capability Terms

- **Local import** reads an export file selected from disk.
- **Official scan** reads activity through a platform's official API.
- **Review** browses, filters, and selects discovered activity locally.
- **Cleanup planning** creates inspectable local actions from selections.
- **Assisted cleanup** guides a person through changes they perform themselves.
- **Simulated execution** runs the apply lifecycle without platform changes.
- **Automatic cleanup** would perform platform changes; Vanish does not support it.

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

Reddit remains an experimental read-only official API planner prototype. A
developer access request has been submitted, access remains pending, and
approval has not been granted. Public builds may therefore be unable to use it.

The prototype uses installed-app OAuth with `identity history` scopes, official
API access for own comments and submitted posts, and local review and planning.
It has no assisted cleanup or platform mutations.

Reddit does not block the Instagram alpha release. It must not add scraping,
browser automation, private APIs, password collection, cookie/session paste, or
automatic platform actions.

## Safety Boundaries

- Instagram import reads local files selected by the user.
- Reddit network activity, where access is available, stays inside the documented read-only official OAuth/API boundary.
- Simulation uses provider-specific routing but performs no platform changes.
- Local data wipe clears Vanish-managed metadata only.
- No supported platform has automatic cleanup.
