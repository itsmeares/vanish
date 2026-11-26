// Keeps global CSS small and text readable. Global CSS lives only in
// packages/ui/styles/ under a line budget; each app's entry stylesheet only
// imports; nothing uses !important; and text uses foreground at 80% or more,
// or muted-foreground, because faded text depends on the surface behind it.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

const repo = path.resolve(import.meta.dirname, "../../..");
const stylesDir = path.join(repo, "packages/ui/styles");
const ENTRY_STYLESHEETS = [
  "apps/desktop/src/renderer/app.css",
  "apps/site/src/styles/site.css",
];
const SOURCE_DIRS = [
  "packages/ui/src",
  "packages/ui/styles",
  "apps/desktop/src",
  "apps/site/src",
];
const STYLES_LINE_BUDGET = 400;
const TEXT_OPACITY_FLOOR = 80;
const problems = [];

// Utility classes like `hover:text-foreground/60`. `before:`/`after:` content
// is decorative and exempt.
const fadedTextClass =
  /(?<![\w/-])((?:[\w-]+:)*)text-(muted-)?foreground\/(\d+)/g;
// `color-mix(in oklab, var(--foreground) 60%, …)` or Tailwind's
// `--alpha(var(--foreground) / 60%)`.
const fadedTextCss =
  /(?<![\w-])color:\s*(?:color-mix\(in oklab, |--alpha\()var\(--(muted-)?foreground\)(?: \/)? (\d+)%/g;
const isFaded = (muted, amount) =>
  Boolean(muted) || Number(amount) < TEXT_OPACITY_FLOOR;

const walk = (dir) =>
  readdirSync(dir).flatMap((name) => {
    const full = path.join(dir, name);
    return statSync(full).isDirectory() ? walk(full) : [full];
  });

for (const entry of ENTRY_STYLESHEETS) {
  const file = path.join(repo, entry);
  if (!existsSync(file)) {
    problems.push(`${entry} is missing`);
    continue;
  }
  for (const line of readFileSync(file, "utf8").split("\n")) {
    const trimmed = line.trim();
    if (
      trimmed &&
      !trimmed.startsWith("@import ") &&
      !trimmed.startsWith("@source ")
    ) {
      problems.push(
        `${entry} may only contain @import and @source lines: "${trimmed}"`,
      );
    }
  }
}

const styleLines = readdirSync(stylesDir)
  // fonts.css only lists @font-face files, so it does not count.
  .filter((name) => name.endsWith(".css") && name !== "fonts.css")
  .reduce(
    (sum, name) =>
      sum + readFileSync(path.join(stylesDir, name), "utf8").split("\n").length,
    0,
  );
if (styleLines > STYLES_LINE_BUDGET) {
  problems.push(
    `packages/ui/styles/*.css has ${styleLines} lines, over the ${STYLES_LINE_BUDGET} line budget`,
  );
}

const entries = new Set(
  ENTRY_STYLESHEETS.map((entry) => path.join(repo, entry)),
);
for (const dir of SOURCE_DIRS) {
  for (const file of walk(path.join(repo, dir))) {
    const rel = path.relative(repo, file);
    if (
      file.endsWith(".css") &&
      !file.startsWith(stylesDir) &&
      !entries.has(file)
    ) {
      problems.push(`${rel}: global CSS belongs in packages/ui/styles/`);
    }
    if (!/\.(css|tsx?|astro)$/.test(file)) continue;
    const source = readFileSync(file, "utf8");
    if (source.includes("!important")) problems.push(`${rel}: uses !important`);
    for (const [match, variants, muted, amount] of source.matchAll(
      fadedTextClass,
    )) {
      if (/(^|:)(before|after):$/.test(variants)) continue;
      if (isFaded(muted, amount)) {
        problems.push(
          `${rel}: ${match} fades text below the contrast floor; use text-muted-foreground, or text-foreground/${TEXT_OPACITY_FLOOR} and up`,
        );
      }
    }
    for (const [match, muted, amount] of source.matchAll(fadedTextCss)) {
      if (isFaded(muted, amount)) {
        problems.push(
          `${rel}: ${match} fades text below the contrast floor; use var(--muted-foreground), or --foreground at ${TEXT_OPACITY_FLOOR}% and up`,
        );
      }
    }
  }
}

if (problems.length > 0) {
  console.error(problems.join("\n"));
  process.exit(1);
}
