import { mkdtempSync, rmSync } from "node:fs";
import { DatabaseSync } from "node:sqlite";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vite-plus/test";

import { DEFAULT_RULE, type Item } from "../shared/rule";
import { openDb, type Db } from "./db";
import { createRunner, type Action } from "./runner";

const item = (id: string, overrides: Partial<Item> = {}): Item => ({
  id,
  kind: "comments",
  community: "AskReddit",
  title: "A thread",
  body: `Old text ${id}`,
  url: `https://www.reddit.com/c/${id}`,
  score: 1,
  createdAt: Date.UTC(2020, 0, 1),
  ...overrides,
});

let dir: string;
let file: string;
let db: Db;
let accountId: number;

beforeEach(() => {
  dir = mkdtempSync(path.join(tmpdir(), "vanish-runner-"));
  file = path.join(dir, "vanish.db");
  db = openDb(file);
  accountId = db.addAccount("reddit", "u/test");
  db.saveItems(accountId, [
    item("a"),
    item("b"),
    item("c", { community: "buildapc" }),
  ]);
});

afterEach(() => {
  db.close();
  rmSync(dir, { recursive: true, force: true });
});

const settle = () =>
  vi.waitFor(
    () => {
      const [run] = db.activeRuns();
      if (run?.status === "running") throw new Error("still running");
    },
    { timeout: 2000 },
  );

const setup = (remove: (item: Item, action: Action) => Promise<void>) =>
  createRunner({
    db,
    remover: () => ({ remove }),
    pauseMs: 0,
    onChange: () => {},
  });

describe("runner", () => {
  it("overwrites, then deletes, and keeps only a record without the text", async () => {
    const calls: [string, Action][] = [];
    const runner = setup(
      async (target, action) => void calls.push([target.id, action]),
    );
    const runId = runner.start(
      accountId,
      "comments",
      DEFAULT_RULE,
      "overwrite-delete",
    );
    await vi.waitFor(() => expect(db.run(runId)?.status).toBe("done"));

    expect(calls.map(([id]) => id).sort()).toEqual(["a", "b", "c"]);
    for (const [, action] of calls) {
      expect(action.remove).toBe(true);
      expect(action.overwrite).toContain("This was removed with [Vanish]");
    }
    expect(db.items(accountId, "comments")).toEqual([]);
    expect(db.run(runId)).toMatchObject({ total: 3, done: 3, failed: 0 });
  });

  it("only runs what the rule puts on the list", async () => {
    const runner = setup(async () => {});
    const rule = {
      ...DEFAULT_RULE,
      keep: [{ type: "community" as const, value: "buildapc" }],
    };
    const runId = runner.start(accountId, "comments", rule, "delete");
    await vi.waitFor(() => expect(db.run(runId)?.status).toBe("done"));
    expect(db.items(accountId, "comments").map((row) => row.id)).toEqual(["c"]);
  });

  it("leaves the new text in place in overwrite-only mode", async () => {
    const runner = setup(async () => {});
    const runId = runner.start(
      accountId,
      "comments",
      DEFAULT_RULE,
      "overwrite",
    );
    await vi.waitFor(() => expect(db.run(runId)?.status).toBe("done"));
    const rows = db.items(accountId, "comments");
    expect(rows).toHaveLength(3);
    for (const row of rows) expect(row.body).toContain("This was removed with");
  });

  it("stops on an error and retries that item on resume", async () => {
    let failing = true;
    const runner = setup(async (target) => {
      if (target.id === "b" && failing) throw new Error("Reddit answered 500");
    });
    const runId = runner.start(accountId, "comments", DEFAULT_RULE, "delete");
    await settle();
    expect(db.run(runId)).toMatchObject({
      status: "paused",
      error: "Reddit answered 500",
      failed: 1,
    });

    failing = false;
    runner.resume(runId);
    await vi.waitFor(() => expect(db.run(runId)?.status).toBe("done"));
    expect(db.run(runId)).toMatchObject({ done: 3, failed: 0, error: null });
  });

  it("pauses between items and does nothing more until resumed", async () => {
    const seen: string[] = [];
    const runner = setup(async (target) => {
      seen.push(target.id);
      if (seen.length === 1) runner.pause(db.activeRuns()[0].id);
    });
    const runId = runner.start(accountId, "comments", DEFAULT_RULE, "delete");
    await settle();
    expect(seen).toHaveLength(1);
    expect(db.run(runId)?.status).toBe("paused");
    runner.resume(runId);
    await vi.waitFor(() => expect(db.run(runId)?.status).toBe("done"));
    expect(seen).toHaveLength(3);
  });

  it("refuses a second run on the same account", () => {
    const runner = setup(() => new Promise(() => {}));
    runner.start(accountId, "comments", DEFAULT_RULE, "delete");
    expect(() =>
      runner.start(accountId, "comments", DEFAULT_RULE, "delete"),
    ).toThrow(/already has a cleanup/);
  });

  it("pauses a run that was going when the app closed", () => {
    const runner = setup(() => new Promise(() => {}));
    const runId = runner.start(accountId, "comments", DEFAULT_RULE, "delete");
    runner.recover();
    expect(db.run(runId)?.status).toBe("paused");
  });
});

describe("database", () => {
  it("does not let anything join a run's list after it starts", () => {
    const runId = db.createRun(accountId, "comments", "delete", ["a"]);
    const raw = new DatabaseSync(file);
    try {
      expect(() =>
        raw
          .prepare(
            "INSERT INTO run_items (run_id, position, item_id) VALUES (?, 1, 'b')",
          )
          .run(runId),
      ).toThrow(/frozen/);
    } finally {
      raw.close();
    }
  });

  it("forgets an account's items and runs when the account is removed", () => {
    db.createRun(accountId, "comments", "delete", ["a"]);
    db.removeAccount(accountId);
    expect(db.items(accountId, "comments")).toEqual([]);
    expect(db.activeRuns()).toEqual([]);
  });
});
