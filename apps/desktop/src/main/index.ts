import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { app, type BrowserWindow, dialog, ipcMain } from "electron";
import type { z } from "zod";

import {
  inputs,
  type AppState,
  type CallName,
  type Calls,
  type Update,
} from "../shared/api";
import {
  keepsHost,
  PLATFORMS,
  type Kind,
  type PlatformId,
} from "../shared/platforms";
import { openDb } from "./db";
import { demoRemover, seedDemo } from "./demo";
import { createRecorder } from "./recorder";
import { createRunner, matching, type Remover } from "./runner";
import { startUpdates } from "./updates";
import {
  createMainWindow,
  forgetAccountSession,
  openAccountBrowser,
} from "./windows";

const demo = process.argv.includes("--demo");
// The demo never sees real data: it gets a fresh folder every launch.
if (demo)
  app.setPath("userData", mkdtempSync(path.join(tmpdir(), "vanish-demo-")));

if (!app.requestSingleInstanceLock()) app.quit();

/** Platform code by platform. Reddit's scanner and remover arrive with Reddit support. */
const SITES: Partial<
  Record<PlatformId, { remover: Remover; scan?: () => Promise<void> }>
> = demo ? { reddit: { remover: demoRemover } } : {};

const PAUSE_MS = demo ? 250 : 1500;

const stamp = () => new Date().toISOString().slice(0, 10);

void app.whenReady().then(() => {
  const db = openDb(path.join(app.getPath("userData"), "vanish.db"));
  if (demo) seedDemo(db);

  let window: BrowserWindow | null = null;
  let update: Update | null = null;
  let recording: {
    accountId: number;
    recorder: ReturnType<typeof createRecorder>;
  } | null = null;

  const account = (accountId: number) => {
    const found = db.accounts().find((row) => row.id === accountId);
    if (!found) throw new Error("That account no longer exists.");
    return { ...found, platform: PLATFORMS[found.platform] };
  };

  const state = (): AppState => ({
    accounts: db
      .accounts()
      .map((row) => ({ ...row, canScan: Boolean(SITES[row.platform]?.scan) })),
    runs: db.activeRuns(),
    recording: recording && {
      accountId: recording.accountId,
      count: recording.recorder.count(),
    },
    update,
    demo,
    version: app.getVersion(),
  });

  const push = () => window?.webContents.send("state", state());
  const itemsChanged = (change: { accountId: number; kind: Kind }) => {
    window?.webContents.send("items", {
      accountId: change.accountId,
      kind: change.kind,
    });
    push();
  };

  const runner = createRunner({
    db,
    remover: (accountId) =>
      SITES[account(accountId).platform.id]?.remover ?? null,
    pauseMs: PAUSE_MS,
    onChange: itemsChanged,
  });
  runner.recover();

  const updates = startUpdates((next) => {
    update = next;
    push();
  });

  const browse = async (accountId: number, url?: string) => {
    const { platform, name } = account(accountId);
    if (url && !keepsHost(platform, url))
      throw new Error(`Vanish only opens ${platform.name} here.`);
    await openAccountBrowser(
      accountId,
      `${name} · ${platform.name}`,
      url ?? platform.homeUrl,
      async (contents) => {
        if (recording?.accountId === accountId)
          await recording.recorder.attach(contents);
      },
    );
  };

  const saveJson = async (defaultName: string, data: unknown) => {
    if (!window) return false;
    const { canceled, filePath } = await dialog.showSaveDialog(window, {
      defaultPath: path.join(app.getPath("documents"), defaultName),
      filters: [{ name: "JSON", extensions: ["json"] }],
    });
    if (canceled || !filePath) return false;
    writeFileSync(filePath, JSON.stringify(data, null, 2));
    return true;
  };

  const handlers: {
    [Name in CallName]: (
      ...args: z.output<(typeof inputs)[Name]>
    ) => ReturnType<Calls[Name]>;
  } = {
    getState: async () => state(),
    items: async (accountId, kind) => db.items(accountId, kind),
    addAccount: async (platformId) => {
      const platform = PLATFORMS[platformId];
      const count = db
        .accounts()
        .filter((row) => row.platform === platformId).length;
      const accountId = db.addAccount(
        platformId,
        `${platform.name} account ${count + 1}`,
      );
      push();
      await browse(accountId, platform.loginUrl);
      return accountId;
    },
    removeAccount: async (accountId) => {
      runner.stopAccount(accountId);
      if (recording?.accountId === accountId) {
        recording.recorder.stop();
        recording = null;
      }
      await forgetAccountSession(accountId);
      db.removeAccount(accountId);
      push();
    },
    openBrowser: async (accountId, url) => browse(accountId, url),
    scan: async (accountId) => {
      const scan = SITES[account(accountId).platform.id]?.scan;
      if (!scan)
        throw new Error("Scanning is not built yet for this platform.");
      await scan();
    },
    startRun: async ({ accountId, kind, rule, mode, now }) =>
      runner.start(accountId, kind, rule, mode, now),
    pauseRun: async (runId) => runner.pause(runId),
    resumeRun: async (runId) => runner.resume(runId),
    stopRun: async (runId) => runner.stop(runId),
    saveBackup: async ({ accountId, kind, rule, now }) => {
      const { platform, name } = account(accountId);
      const items = matching(db, accountId, kind, rule, now);
      const saved = await saveJson(
        `vanish-backup-${platform.id}-${kind}-${stamp()}.json`,
        {
          format: "vanish-backup",
          version: 1,
          platform: platform.id,
          account: name,
          kind,
          savedAt: new Date().toISOString(),
          items,
        },
      );
      return { saved, count: items.length };
    },
    history: async () => db.history(),
    getSettings: async () => db.settings(),
    updateSettings: async (change) => {
      db.saveSettings(change);
      return db.settings();
    },
    startRecording: async (accountId) => {
      if (recording) throw new Error("A recording is already going.");
      const { platform } = account(accountId);
      recording = { accountId, recorder: createRecorder(platform, push) };
      push();
      await browse(accountId, platform.homeUrl);
    },
    stopRecording: async ({ save }) => {
      if (!recording) return { saved: false, count: 0 };
      const { accountId, recorder } = recording;
      recording = null;
      const result = recorder.stop();
      push();
      if (!save) return { saved: false, count: result.entries.length };
      const saved = await saveJson(
        `vanish-recording-${account(accountId).platform.id}-${stamp()}.json`,
        { ...result, app: app.getVersion() },
      );
      return { saved, count: result.entries.length };
    },
    installUpdate: () => updates.install(),
    checkForUpdates: () => updates.check(),
  };

  for (const name of Object.keys(inputs) as CallName[]) {
    ipcMain.handle(name, (event, ...args: unknown[]) => {
      // Only the Vanish window may call in. Account browsers have no preload,
      // but a check here costs nothing.
      if (!window || event.sender !== window.webContents)
        throw new Error("Unknown window");
      const parsed = inputs[name].parse(args) as never[];
      return (handlers[name] as (...values: never[]) => unknown)(...parsed);
    });
  }

  const open = () => {
    window = createMainWindow();
    window.on("closed", () => {
      window = null;
      // Account browsers are part of the app, so they close with it.
      if (process.platform !== "darwin") app.quit();
    });
  };
  open();

  app.on("second-instance", () => {
    if (!window) return open();
    if (window.isMinimized()) window.restore();
    window.focus();
  });
  app.on("activate", () => {
    if (!window) open();
  });
});
