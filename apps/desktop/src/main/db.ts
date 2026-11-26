import { DatabaseSync } from "node:sqlite";

import type { Mode, Run, RunStatus, Settings } from "../shared/api";
import { KINDS, type Kind, type PlatformId } from "../shared/platforms";
import { DEFAULT_TEXT } from "../shared/replacement";
import type { Item } from "../shared/rule";

const SCHEMA = `
CREATE TABLE accounts (
  id INTEGER PRIMARY KEY,
  platform TEXT NOT NULL,
  name TEXT NOT NULL,
  added_at INTEGER NOT NULL,
  scanned_at INTEGER
);
CREATE TABLE items (
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  id TEXT NOT NULL,
  community TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  url TEXT NOT NULL,
  score INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, kind, id)
);
CREATE TABLE runs (
  id INTEGER PRIMARY KEY,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT,
  frozen INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  finished_at INTEGER
);
-- What a run removed and when. The text itself is gone with the item.
CREATE TABLE run_items (
  run_id INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  item_id TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'queued',
  error TEXT,
  done_at INTEGER,
  PRIMARY KEY (run_id, item_id)
);
-- The runner asks for the next queued item after every item.
CREATE INDEX run_items_queue ON run_items (run_id, state, position);
-- The list you confirmed is the list that runs: nothing joins it later.
CREATE TRIGGER run_items_frozen BEFORE INSERT ON run_items
WHEN (SELECT frozen FROM runs WHERE id = NEW.run_id) = 1
BEGIN
  SELECT RAISE(ABORT, 'this run''s list is frozen');
END;
CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`;

const DEFAULT_SETTINGS: Settings = {
  text: DEFAULT_TEXT,
  randomWords: true,
  mode: "overwrite-delete",
};

type Row = Record<string, string | number | null>;

const toItem = (row: Row): Item => ({
  id: String(row.id),
  kind: row.kind as Kind,
  community: String(row.community),
  title: String(row.title),
  body: String(row.body),
  url: String(row.url),
  score: Number(row.score),
  createdAt: Number(row.created_at),
});

const RUN_SELECT = `
SELECT runs.*,
  (SELECT count(*) FROM run_items WHERE run_id = runs.id) AS total,
  (SELECT count(*) FROM run_items WHERE run_id = runs.id AND state = 'done') AS done,
  (SELECT count(*) FROM run_items WHERE run_id = runs.id AND state = 'failed') AS failed
FROM runs`;

const toRun = (row: Row): Run => ({
  id: Number(row.id),
  accountId: Number(row.account_id),
  kind: row.kind as Kind,
  mode: row.mode as Mode,
  status: row.status as RunStatus,
  total: Number(row.total),
  done: Number(row.done),
  failed: Number(row.failed),
  error: row.error === null ? null : String(row.error),
  createdAt: Number(row.created_at),
  finishedAt: row.finished_at === null ? null : Number(row.finished_at),
});

export type Db = ReturnType<typeof openDb>;

export const openDb = (file: string) => {
  const db = new DatabaseSync(file);
  db.exec("PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;");
  const version = Number(
    (db.prepare("PRAGMA user_version").get() as Row).user_version,
  );
  if (version === 0)
    db.exec(`BEGIN; ${SCHEMA} PRAGMA user_version = 1; COMMIT;`);

  const transaction = <T>(work: () => T): T => {
    db.exec("BEGIN");
    try {
      const result = work();
      db.exec("COMMIT");
      return result;
    } catch (error) {
      db.exec("ROLLBACK");
      throw error;
    }
  };

  const insertItem = db.prepare(`
    INSERT INTO items (account_id, kind, id, community, title, body, url, score, created_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT (account_id, kind, id) DO UPDATE SET
      community = excluded.community, title = excluded.title, body = excluded.body,
      url = excluded.url, score = excluded.score, created_at = excluded.created_at`);

  return {
    close: () => db.close(),

    accounts: () =>
      (db.prepare("SELECT * FROM accounts ORDER BY id").all() as Row[]).map(
        (row) => {
          const counts = Object.fromEntries(
            KINDS.map((kind) => [kind, 0]),
          ) as Record<Kind, number>;
          const rows = db
            .prepare(
              "SELECT kind, count(*) AS n FROM items WHERE account_id = ? GROUP BY kind",
            )
            .all(row.id) as Row[];
          for (const count of rows)
            counts[count.kind as Kind] = Number(count.n);
          return {
            id: Number(row.id),
            platform: row.platform as PlatformId,
            name: String(row.name),
            scannedAt: row.scanned_at === null ? null : Number(row.scanned_at),
            counts,
          };
        },
      ),

    addAccount: (platform: PlatformId, name: string) =>
      Number(
        db
          .prepare(
            "INSERT INTO accounts (platform, name, added_at) VALUES (?, ?, ?)",
          )
          .run(platform, name, Date.now()).lastInsertRowid,
      ),

    removeAccount: (accountId: number) =>
      db.prepare("DELETE FROM accounts WHERE id = ?").run(accountId),

    saveItems: (accountId: number, items: Item[]) =>
      transaction(() => {
        for (const item of items) {
          insertItem.run(
            accountId,
            item.kind,
            item.id,
            item.community,
            item.title,
            item.body,
            item.url,
            item.score,
            item.createdAt,
          );
        }
        db.prepare("UPDATE accounts SET scanned_at = ? WHERE id = ?").run(
          Date.now(),
          accountId,
        );
      }),

    items: (accountId: number, kind: Kind) =>
      (
        db
          .prepare(
            "SELECT * FROM items WHERE account_id = ? AND kind = ? ORDER BY created_at DESC",
          )
          .all(accountId, kind) as Row[]
      ).map(toItem),

    item: (accountId: number, kind: Kind, itemId: string) => {
      const row = db
        .prepare(
          "SELECT * FROM items WHERE account_id = ? AND kind = ? AND id = ?",
        )
        .get(accountId, kind, itemId) as Row | undefined;
      return row ? toItem(row) : null;
    },

    deleteItem: (accountId: number, kind: Kind, itemId: string) =>
      db
        .prepare(
          "DELETE FROM items WHERE account_id = ? AND kind = ? AND id = ?",
        )
        .run(accountId, kind, itemId),

    setBody: (accountId: number, kind: Kind, itemId: string, body: string) =>
      db
        .prepare(
          "UPDATE items SET body = ? WHERE account_id = ? AND kind = ? AND id = ?",
        )
        .run(body, accountId, kind, itemId),

    /** Creates a run with its list and freezes the list in one step. */
    createRun: (accountId: number, kind: Kind, mode: Mode, itemIds: string[]) =>
      transaction(() => {
        const runId = Number(
          db
            .prepare(
              "INSERT INTO runs (account_id, kind, mode, status, created_at) VALUES (?, ?, ?, 'running', ?)",
            )
            .run(accountId, kind, mode, Date.now()).lastInsertRowid,
        );
        const insert = db.prepare(
          "INSERT INTO run_items (run_id, position, item_id) VALUES (?, ?, ?)",
        );
        itemIds.forEach((itemId, position) =>
          insert.run(runId, position, itemId),
        );
        db.prepare("UPDATE runs SET frozen = 1 WHERE id = ?").run(runId);
        return runId;
      }),

    run: (runId: number) => {
      const row = db.prepare(`${RUN_SELECT} WHERE runs.id = ?`).get(runId) as
        | Row
        | undefined;
      return row ? toRun(row) : null;
    },

    activeRuns: () =>
      (
        db
          .prepare(
            `${RUN_SELECT} WHERE status IN ('running', 'paused') ORDER BY runs.id`,
          )
          .all() as Row[]
      ).map(toRun),

    history: () =>
      (
        db
          .prepare(
            `${RUN_SELECT.replace("SELECT runs.*", "SELECT runs.*, accounts.name AS account_name, accounts.platform AS platform")}
             JOIN accounts ON accounts.id = runs.account_id
             WHERE status IN ('stopped', 'done') ORDER BY runs.id DESC LIMIT 200`,
          )
          .all() as Row[]
      ).map((row) => ({
        ...toRun(row),
        accountName: String(row.account_name),
        platform: row.platform as PlatformId,
      })),

    setRunStatus: (
      runId: number,
      status: RunStatus,
      error: string | null = null,
    ) =>
      db
        .prepare(
          "UPDATE runs SET status = ?, error = ?, finished_at = CASE WHEN ? IN ('stopped', 'done') THEN ? ELSE NULL END WHERE id = ?",
        )
        .run(status, error, status, Date.now(), runId),

    nextQueued: (runId: number) => {
      const row = db
        .prepare(
          "SELECT item_id FROM run_items WHERE run_id = ? AND state = 'queued' ORDER BY position LIMIT 1",
        )
        .get(runId) as Row | undefined;
      return row ? String(row.item_id) : null;
    },

    finishItem: (runId: number, itemId: string, error: string | null) =>
      db
        .prepare(
          "UPDATE run_items SET state = ?, error = ?, done_at = ? WHERE run_id = ? AND item_id = ?",
        )
        .run(
          error === null ? "done" : "failed",
          error,
          Date.now(),
          runId,
          itemId,
        ),

    requeueFailed: (runId: number) =>
      db
        .prepare(
          "UPDATE run_items SET state = 'queued', error = NULL WHERE run_id = ? AND state = 'failed'",
        )
        .run(runId),

    settings: (): Settings => {
      const rows = db.prepare("SELECT key, value FROM settings").all() as Row[];
      const saved = Object.fromEntries(
        rows.map((row) => [row.key, JSON.parse(String(row.value))]),
      );
      return { ...DEFAULT_SETTINGS, ...saved };
    },

    saveSettings: (change: Partial<Settings>) =>
      transaction(() => {
        const upsert = db.prepare(
          "INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value",
        );
        for (const [key, value] of Object.entries(change)) {
          if (value !== undefined) upsert.run(key, JSON.stringify(value));
        }
      }),
  };
};
