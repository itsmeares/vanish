import { z } from "zod";

import { KINDS, PLATFORMS, type Kind, type PlatformId } from "./platforms";
import { ruleSchema, type Item } from "./rule";

export const MODES = ["overwrite-delete", "overwrite", "delete"] as const;
export type Mode = (typeof MODES)[number];

export type Account = {
  id: number;
  platform: PlatformId;
  name: string;
  scannedAt: number | null;
  counts: Record<Kind, number>;
  /** False until the platform's scanner exists. */
  canScan: boolean;
};

export type RunStatus = "running" | "paused" | "stopped" | "done";
export type Run = {
  id: number;
  accountId: number;
  kind: Kind;
  mode: Mode;
  status: RunStatus;
  total: number;
  done: number;
  failed: number;
  error: string | null;
  createdAt: number;
  finishedAt: number | null;
};

export type Settings = { text: string; randomWords: boolean; mode: Mode };

export type Update = { version: string; ready: boolean; manual: boolean };

export type AppState = {
  accounts: Account[];
  /** Runs that are running or paused. */
  runs: Run[];
  recording: { accountId: number; count: number } | null;
  update: Update | null;
  demo: boolean;
  version: string;
};

const id = z.number().int().positive();
const kind = z.enum(KINDS);
const job = z.object({ accountId: id, kind, rule: ruleSchema });

/** Every call the window can make, with the arguments the main process accepts. */
export const inputs = {
  getState: z.tuple([]),
  items: z.tuple([id, kind]),
  addAccount: z.tuple([z.enum(Object.keys(PLATFORMS) as [PlatformId])]),
  removeAccount: z.tuple([id]),
  openBrowser: z.tuple([id, z.string().url().max(2000).optional()]),
  scan: z.tuple([id, kind]),
  startRun: z.tuple([job.extend({ mode: z.enum(MODES) })]),
  pauseRun: z.tuple([id]),
  resumeRun: z.tuple([id]),
  stopRun: z.tuple([id]),
  saveBackup: z.tuple([job]),
  history: z.tuple([]),
  getSettings: z.tuple([]),
  updateSettings: z.tuple([
    z
      .object({
        text: z.string().max(5000),
        randomWords: z.boolean(),
        mode: z.enum(MODES),
      })
      .partial(),
  ]),
  startRecording: z.tuple([id]),
  stopRecording: z.tuple([z.object({ save: z.boolean() })]),
  installUpdate: z.tuple([]),
  checkForUpdates: z.tuple([]),
} satisfies Record<string, z.ZodTuple>;

export type CallName = keyof typeof inputs;

type Results = {
  getState: AppState;
  items: Item[];
  addAccount: number;
  removeAccount: void;
  openBrowser: void;
  scan: void;
  startRun: number;
  pauseRun: void;
  resumeRun: void;
  stopRun: void;
  saveBackup: { saved: boolean; count: number };
  history: (Run & { accountName: string; platform: PlatformId })[];
  getSettings: Settings;
  updateSettings: Settings;
  startRecording: void;
  stopRecording: { saved: boolean; count: number };
  installUpdate: void;
  checkForUpdates: "found" | "current" | "failed";
};

export type Calls = {
  [Name in CallName]: (
    ...args: z.input<(typeof inputs)[Name]>
  ) => Promise<Results[Name]>;
};

export type DesktopApi = Calls & {
  onState: (listener: (state: AppState) => void) => () => void;
  onItems: (
    listener: (change: { accountId: number; kind: Kind }) => void,
  ) => () => void;
};
