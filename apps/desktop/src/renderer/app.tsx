import { Button } from "@vanish/ui/components/button";
import { Spinner } from "@vanish/ui/components/spinner";
import { ToastProvider } from "@vanish/ui/components/toast";
import { useCallback, useEffect, useRef, useState } from "react";

import type { AppState, Run, Settings } from "../shared/api";
import { PLATFORMS, type Kind } from "../shared/platforms";
import { DEFAULT_RULE, type Rule } from "../shared/rule";
import { useAppState, vanish } from "./bridge";
import { Cleanup } from "./cleanup";
import {
  HistoryDialog,
  RecordDialog,
  RecordingBar,
  RemoveAccountDialog,
  SettingsDialog,
} from "./dialogs";
import { plural } from "./format";
import { Logo } from "./logo";
import { attempt, notify } from "./notify";
import { Sidebar } from "./sidebar";
import { TopRow } from "./top-row";

const Welcome = () => (
  <div className="flex h-screen flex-col bg-background">
    <TopRow edge="left" />
    <div className="flex flex-1 flex-col items-center justify-center gap-6 px-8 pb-20 text-center">
      <Logo className="size-12 text-primary" />
      <div className="flex max-w-md flex-col gap-3">
        <h1 className="text-headline font-semibold">
          Clean up your Reddit history
        </h1>
        <p className="text-muted-foreground">
          Sign in to Reddit inside Vanish, pick what to remove with one
          sentence, and watch it go. Your accounts and everything Vanish finds
          stay on this computer.
        </p>
      </div>
      <Button
        size="lg"
        onClick={() => void attempt(() => vanish.addAccount("reddit"))}
      >
        Add a Reddit account
      </Button>
    </div>
  </div>
);

/** Says when a run ends, with its final numbers. */
const useRunEndings = (state: AppState | null) => {
  const previous = useRef<Run[]>([]);
  useEffect(() => {
    if (!state) return;
    const active = new Set(state.runs.map((run) => run.id));
    const ended = previous.current.filter((run) => !active.has(run.id));
    previous.current = state.runs;
    if (ended.length === 0) return;
    void vanish.history().then((history) => {
      for (const run of history.filter((row) =>
        ended.some((end) => end.id === row.id),
      )) {
        const names = PLATFORMS[run.platform].kinds[run.kind];
        notify(
          run.status === "done"
            ? `Cleanup finished for ${run.accountName}`
            : `Cleanup stopped for ${run.accountName}`,
          `${plural(run.done, names.one, names.many)} ${names.done}.`,
        );
      }
    });
  }, [state]);
};

const Shell = ({ state }: { state: AppState }) => {
  const [accountId, setAccountId] = useState<number | null>(null);
  const [kind, setKind] = useState<Kind>("comments");
  const [rules, setRules] = useState<Record<string, Rule>>({});
  const [settings, setSettings] = useState<Settings | null>(null);
  const [dialog, setDialog] = useState<
    "settings" | "history" | "record" | "remove" | null
  >(null);
  useRunEndings(state);

  useEffect(() => {
    void vanish.getSettings().then(setSettings);
  }, []);

  const account =
    state.accounts.find((row) => row.id === accountId) ??
    state.accounts[0] ??
    null;
  useEffect(() => {
    if (account && account.id !== accountId) setAccountId(account.id);
  }, [account, accountId]);

  const changeSettings = useCallback((change: Partial<Settings>) => {
    void attempt(() => vanish.updateSettings(change)).then(
      (next) => next && setSettings(next),
    );
  }, []);

  if (!account) return <Welcome />;
  if (!settings)
    return (
      <div className="grid h-screen place-items-center">
        <Spinner className="size-5 text-muted-foreground" />
      </div>
    );

  const platform = PLATFORMS[account.platform];
  const key = `${account.id}:${kind}`;
  const run =
    state.runs.find(
      (row) => row.accountId === account.id && row.kind === kind,
    ) ?? null;

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar
        state={state}
        account={account}
        kind={kind}
        onAccount={setAccountId}
        onKind={setKind}
        onAddAccount={() =>
          void attempt(() => vanish.addAccount(account.platform)).then(
            (id) => id && setAccountId(id),
          )
        }
        onRemoveAccount={() => setDialog("remove")}
        onRecord={() => setDialog("record")}
        onHistory={() => setDialog("history")}
        onSettings={() => setDialog("settings")}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <RecordingBar state={state} />
        <div className="flex min-h-0 flex-1">
          <Cleanup
            account={account}
            platform={platform}
            kind={kind}
            rule={rules[key] ?? DEFAULT_RULE}
            run={run}
            settings={settings}
            onKind={setKind}
            onRule={(rule) => setRules((all) => ({ ...all, [key]: rule }))}
            onSettings={changeSettings}
          />
        </div>
      </div>

      <SettingsDialog
        open={dialog === "settings"}
        onOpenChange={(open) => setDialog(open ? "settings" : null)}
        settings={settings}
        state={state}
        onSettings={changeSettings}
      />
      <HistoryDialog
        open={dialog === "history"}
        onOpenChange={(open) => setDialog(open ? "history" : null)}
      />
      <RecordDialog
        open={dialog === "record"}
        onOpenChange={(open) => setDialog(open ? "record" : null)}
        account={account}
      />
      <RemoveAccountDialog
        open={dialog === "remove"}
        onOpenChange={(open) => setDialog(open ? "remove" : null)}
        account={account}
      />
    </div>
  );
};

export const App = () => {
  const state = useAppState();
  return (
    <ToastProvider>
      {state ? (
        <Shell state={state} />
      ) : (
        <div className="grid h-screen place-items-center">
          <Spinner className="size-5 text-muted-foreground" />
        </div>
      )}
    </ToastProvider>
  );
};
