import { strFromU8, unzipSync } from "fflate";

import type { Kind } from "../../shared/platforms";
import type { Item } from "../../shared/rule";

type Row = Record<string, string>;

/** RFC 4180: a quoted field may hold commas, line breaks and doubled quotes. */
export const parseCsv = (text: string): Row[] => {
  const rows: string[][] = [];
  let row: string[] = [];
  let field = "";
  let quoted = false;
  const source = text.replace(/^﻿/, "");
  for (let index = 0; index < source.length; index++) {
    const char = source[index];
    if (quoted) {
      if (char === '"' && source[index + 1] === '"') {
        field += '"';
        index++;
      } else if (char === '"') quoted = false;
      else field += char;
    } else if (char === '"') quoted = true;
    else if (char === ",") {
      row.push(field);
      field = "";
    } else if (char === "\n" || char === "\r") {
      if (char === "\r" && source[index + 1] === "\n") index++;
      row.push(field);
      rows.push(row);
      row = [];
      field = "";
    } else field += char;
  }
  if (field || row.length > 0) rows.push([...row, field]);
  const [header = [], ...body] = rows;
  return body.map((cells) =>
    Object.fromEntries(header.map((name, at) => [name, cells[at] ?? ""])),
  );
};

// Each file lists one kind. Ids get Reddit's type prefix (t1_ comment,
// t3_ post), which is how Reddit names them in its own requests.
const FILES: { file: string; kind: Kind; prefix: "t1_" | "t3_" }[] = [
  { file: "comments.csv", kind: "comments", prefix: "t1_" },
  { file: "posts.csv", kind: "posts", prefix: "t3_" },
  { file: "post_votes.csv", kind: "votes", prefix: "t3_" },
  { file: "comment_votes.csv", kind: "votes", prefix: "t1_" },
  { file: "saved_posts.csv", kind: "saved", prefix: "t3_" },
  { file: "saved_comments.csv", kind: "saved", prefix: "t1_" },
  { file: "hidden_posts.csv", kind: "hidden", prefix: "t3_" },
];
// Only these files are opened. The export also holds email addresses, IP
// logs and messages, which Vanish has no use for.
const ACCOUNT_FILE = "statistics.csv";

/** "2022-04-08 14:36:44 UTC" */
const time = (value: string) => {
  const parsed = Date.parse(value.replace(" UTC", "Z").replace(" ", "T"));
  return Number.isNaN(parsed) ? null : parsed;
};

/** /r/{community}/comments/{post}/{title_words}/… */
const fromLink = (permalink: string) => {
  try {
    const parts = new URL(permalink).pathname.split("/").filter(Boolean);
    const slug = parts[2] === "comments" ? parts[4] : undefined;
    return {
      community: parts[0] === "r" ? (parts[1] ?? "") : "",
      title: slug && slug !== "comment" ? slug.replaceAll("_", " ") : "",
    };
  } catch {
    return { community: "", title: "" };
  }
};

const toItem = (row: Row, kind: Kind, prefix: string): Item | null => {
  if (!row.id) return null;
  // A vote file can list votes already taken back.
  if (kind === "votes" && row.direction !== "up" && row.direction !== "down")
    return null;
  const link = fromLink(row.permalink ?? "");
  return {
    id: `${prefix}${row.id}`,
    kind,
    community: row.subreddit || link.community,
    title: row.title || link.title,
    body: row.body ?? "",
    url: row.permalink ?? "",
    // The export has no points, and votes, saves and hides have no date.
    score: null,
    createdAt: row.date ? time(row.date) : null,
  };
};

/** Reads the items and the account name out of a Reddit data export zip. */
export const readExport = (zip: Uint8Array) => {
  const wanted = new Set([ACCOUNT_FILE, ...FILES.map(({ file }) => file)]);
  const base = (name: string) => name.split("/").pop() ?? name;
  const files = new Map(
    Object.entries(
      unzipSync(zip, { filter: (file) => wanted.has(base(file.name)) }),
    ).map(([name, data]) => [base(name), strFromU8(data)]),
  );
  if (!FILES.some(({ file }) => files.has(file)))
    throw new Error("This zip is not a Reddit data export.");
  const account =
    parseCsv(files.get(ACCOUNT_FILE) ?? "").find(
      (row) => row.statistic === "account name",
    )?.value || null;
  const items = FILES.flatMap(({ file, kind, prefix }) =>
    parseCsv(files.get(file) ?? "").flatMap(
      (row) => toItem(row, kind, prefix) ?? [],
    ),
  );
  return { account, items };
};
