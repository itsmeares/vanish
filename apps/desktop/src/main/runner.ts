import { setTimeout as sleep } from "node:timers/promises";

import type { Mode } from "../shared/api";
import { hasText, PLATFORMS, type Kind } from "../shared/platforms";
import { replacementText } from "../shared/replacement";
import { judge, type Item, type Rule } from "../shared/rule";
import type { Db } from "./db";

/** What to do to one item: write new text over it, remove it, or both. */
export type Action = { overwrite: string | null; remove: boolean };

/** The platform code that talks to the site for one signed-in account. */
export type Remover = { remove: (item: Item, action: Action) => Promise<void> };

const actionFor = (kind: Kind, mode: Mode, text: () => string): Action => {
  if (!hasText(kind)) return { overwrite: null, remove: true };
  if (mode === "delete") return { overwrite: null, remove: true };
  return { overwrite: text(), remove: mode === "overwrite-delete" };
};

/**
 * The items a rule puts on the delete list, computed here rather than trusted
 * from the window. `now` is the moment the list was shown, so an "older than"
 * rule freezes exactly the items the person confirmed.
 */
export const matching = (
  db: Db,
  accountId: number,
  kind: Kind,
  rule: Rule,
  now: number,
) => {
  const account = db.accounts().find((row) => row.id === accountId);
  if (!account) throw new Error("That account no longer exists.");
  const verdict = judge(
    rule,
    Math.min(now, Date.now()),
    PLATFORMS[account.platform].community.prefix,
  );
  return db
    .items(accountId, kind)
    .filter((item) => verdict(item).type === "goes");
};

export const createRunner = ({
  db,
  remover,
  pauseMs,
  onChange,
}: {
  db: Db;
  remover: (accountId: number) => Remover | null;
  /** The wait between two items. Slow on purpose, so the account stays safe. */
  pauseMs: number;
  onChange: (change: { accountId: number; kind: Kind }) => void;
}) => {
  const looping = new Set<number>();
  // Accounts with a request on its way to the site, which Stop cannot recall.
  const inFlight = new Set<number>();

  const loop = async (runId: number) => {
    if (looping.has(runId)) return;
    looping.add(runId);
    try {
      for (;;) {
        const run = db.run(runId);
        if (!run || run.status !== "running") return;
        const site = remover(run.accountId);
        if (!site) {
          db.setRunStatus(
            runId,
            "paused",
            "Vanish cannot remove items on this platform yet.",
          );
          return onChange(run);
        }
        const itemId = db.nextQueued(runId);
        if (itemId === null) {
          db.setRunStatus(runId, "done");
          return onChange(run);
        }
        const item = db.item(run.accountId, run.kind, itemId);
        if (!item) {
          // A later scan found it already gone, which is what the run wanted.
          db.finishItem(runId, itemId, null);
          continue;
        }
        const { text, randomWords } = db.settings();
        const action = actionFor(run.kind, run.mode, () =>
          replacementText({ text, randomWords }),
        );
        inFlight.add(run.accountId);
        try {
          await site.remove(item, action);
        } catch (error) {
          // Stop on anything unexpected. Resuming retries the item, which is
          // safe: deleting, unvoting or unsaving twice changes nothing.
          const message =
            error instanceof Error ? error.message : String(error);
          db.finishItem(runId, itemId, message);
          // A run stopped while the request was out stays stopped.
          if (db.run(runId)?.status !== "stopped")
            db.setRunStatus(runId, "paused", message);
          return onChange(run);
        } finally {
          inFlight.delete(run.accountId);
        }
        if (action.remove) db.deleteItem(run.accountId, run.kind, itemId);
        else if (action.overwrite !== null)
          db.setBody(run.accountId, run.kind, itemId, action.overwrite);
        db.finishItem(runId, itemId, null);
        onChange(run);
        await sleep(pauseMs);
      }
    } finally {
      looping.delete(runId);
    }
  };

  const change = (runId: number, status: "running" | "paused" | "stopped") => {
    const run = db.run(runId);
    if (!run || run.status === "done" || run.status === "stopped") return;
    if (status === "running") db.requeueFailed(runId);
    db.setRunStatus(runId, status);
    onChange(run);
    if (status === "running") void loop(runId);
  };

  return {
    start: (
      accountId: number,
      kind: Kind,
      rule: Rule,
      mode: Mode,
      now: number,
    ) => {
      if (
        inFlight.has(accountId) ||
        db.activeRuns().some((run) => run.accountId === accountId)
      )
        throw new Error(
          "This account already has a cleanup running. Finish or stop it first.",
        );
      const items = matching(db, accountId, kind, rule, now);
      if (items.length === 0)
        throw new Error("Nothing matches, so there is nothing to remove.");
      const runId = db.createRun(
        accountId,
        kind,
        mode,
        items.map((item) => item.id),
      );
      onChange({ accountId, kind });
      void loop(runId);
      return runId;
    },
    pause: (runId: number) => change(runId, "paused"),
    resume: (runId: number) => change(runId, "running"),
    stop: (runId: number) => change(runId, "stopped"),
    /** A run that was going when the app closed waits for you to resume it. */
    recover: () => {
      for (const run of db.activeRuns()) {
        if (run.status === "running") db.setRunStatus(run.id, "paused", null);
      }
    },
    /** Stops an account's run before the account is removed. */
    stopAccount: (accountId: number) => {
      for (const run of db.activeRuns()) {
        if (run.accountId === accountId) db.setRunStatus(run.id, "stopped");
      }
    },
  };
};
