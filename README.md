# Vanish

Vanish is a free, open-source desktop app that removes your social media
footprint. You sign in to Reddit inside Vanish, say what to remove in one
sentence, check the list, and Vanish works through it one item at a time.

Everything stays on your computer. There is no account, no server and no
tracking.

> Vanish is being rebuilt. The first release, with Reddit support, is not out
> yet. Earlier versions are in the git history.

## What it removes on Reddit

Comments, posts, upvotes and downvotes, saved items, and hidden items. Before
deleting a comment or text post, Vanish can overwrite it with replacement
text, so the last version Reddit holds is not your words.

## Installing

Installers for Windows, macOS and Linux will be on the
[releases page](https://github.com/itsmeares/vanish/releases).

- **macOS:** Vanish is not signed with a paid Apple certificate. The first
  time you open it, go to System Settings, then Privacy & Security, and
  choose Open Anyway. Updates are not automatic on macOS; Vanish tells you
  when a new version is out.
- **Windows:** Vanish is not code-signed yet, so Windows may say it protected
  your PC. Choose More info, then Run anyway.

## Development

You need Node.js 24 and pnpm.

```sh
pnpm install
pnpm desktop:demo   # the app with made-up data, safe to click around
pnpm desktop:dev    # the app with your own data folder
pnpm site:dev       # the website
```

Read [AGENTS.md](AGENTS.md) before changing code. It covers how the code is
laid out and the rules that keep people's accounts safe.

## License

AGPL-3.0-only.
