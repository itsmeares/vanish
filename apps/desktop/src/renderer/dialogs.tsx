import { Badge } from "@vanish/ui/components/badge";
import { Button } from "@vanish/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogPanel,
  DialogPopup,
  DialogTitle,
} from "@vanish/ui/components/dialog";
import {
  Field,
  FieldDescription,
  FieldLabel,
} from "@vanish/ui/components/field";
import { Switch } from "@vanish/ui/components/switch";
import { Textarea } from "@vanish/ui/components/textarea";
import { useEffect, useMemo, useState } from "react";

import type { Account, AppState, Settings } from "../shared/api";
import { PLATFORMS } from "../shared/platforms";
import { DEFAULT_TEXT, replacementText } from "../shared/replacement";
import { vanish } from "./bridge";
import { day, plural } from "./format";
import { attempt, notify } from "./notify";

type Open = { open: boolean; onOpenChange: (open: boolean) => void };

export const SettingsDialog = ({
  open,
  onOpenChange,
  settings,
  state,
  onSettings,
}: Open & {
  settings: Settings;
  state: AppState;
  onSettings: (change: Partial<Settings>) => void;
}) => {
  const [text, setText] = useState(settings.text);
  const [checking, setChecking] = useState(false);
  useEffect(() => {
    if (open) setText(settings.text);
  }, [open, settings.text]);
  const preview = useMemo(
    () => replacementText({ text, randomWords: settings.randomWords }),
    [text, settings.randomWords],
  );

  const check = async () => {
    setChecking(true);
    const result = await attempt(() => vanish.checkForUpdates());
    setChecking(false);
    if (result === "current")
      notify(`Vanish ${state.version} is the newest version.`);
    if (result === "failed")
      notify("Could not reach GitHub to check for updates.");
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
        </DialogHeader>
        <DialogPanel className="flex flex-col gap-6">
          <Field>
            <FieldLabel>Replacement text</FieldLabel>
            <Textarea
              value={text}
              rows={3}
              onChange={(event) => setText(event.target.value)}
              onBlur={() => onSettings({ text })}
            />
            <FieldDescription>
              Written over each comment and text post before it is deleted.
              Reddit shows a deleted comment as [deleted], so this text only
              stays visible with &ldquo;Overwrite only&rdquo;. Leave it empty to
              write nothing but the random words.
            </FieldDescription>
            {text !== DEFAULT_TEXT && (
              <Button
                variant="link"
                size="sm"
                className="self-start px-0"
                onClick={() => {
                  setText(DEFAULT_TEXT);
                  onSettings({ text: DEFAULT_TEXT });
                }}
              >
                Use the default text
              </Button>
            )}
          </Field>
          <label className="flex items-start gap-3">
            <Switch
              checked={settings.randomWords}
              onCheckedChange={(randomWords) => onSettings({ randomWords })}
            />
            <span className="flex flex-col gap-1">
              <span className="font-medium">Start with a few random words</span>
              <span className="text-meta text-muted-foreground">
                Every replacement reads a little different, so a thousand edits
                do not look the same.
              </span>
            </span>
          </label>
          <div className="flex flex-col gap-2">
            <span className="text-meta text-muted-foreground">
              An example of what gets written
            </span>
            <p className="rounded-lg border bg-background px-4 py-3 whitespace-pre-wrap">
              {preview || (
                <span className="text-muted-foreground">Nothing</span>
              )}
            </p>
          </div>
          <div className="flex items-center gap-3 border-t pt-5">
            <span className="flex-1 text-meta text-muted-foreground">
              Vanish {state.version}
            </span>
            <Button
              variant="outline"
              size="sm"
              loading={checking}
              onClick={() => void check()}
            >
              Check for updates
            </Button>
          </div>
        </DialogPanel>
      </DialogPopup>
    </Dialog>
  );
};

type HistoryRun = Awaited<ReturnType<typeof vanish.history>>[number];

export const HistoryDialog = ({ open, onOpenChange }: Open) => {
  const [runs, setRuns] = useState<HistoryRun[] | null>(null);
  useEffect(() => {
    if (open)
      void attempt(() => vanish.history()).then((rows) => setRuns(rows ?? []));
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-xl">
        <DialogHeader>
          <DialogTitle>Past cleanups</DialogTitle>
          <DialogDescription>
            Vanish keeps what it removed and when, never the text itself.
          </DialogDescription>
        </DialogHeader>
        <DialogPanel>
          {runs?.length === 0 && (
            <p className="py-6 text-center text-muted-foreground">
              No cleanups yet.
            </p>
          )}
          <ul className="flex flex-col">
            {runs?.map((run) => {
              const names = PLATFORMS[run.platform].kinds[run.kind];
              return (
                <li
                  key={run.id}
                  className="flex items-center gap-4 border-b py-3 last:border-b-0"
                >
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="font-medium">
                      {plural(run.done, names.one, names.many)} {names.done}
                    </span>
                    <span className="text-meta text-muted-foreground">
                      {run.accountName}, {day(run.createdAt)}
                      {run.failed > 0 && `, ${run.failed} failed`}
                    </span>
                  </span>
                  <Badge
                    variant={run.status === "done" ? "success" : "outline"}
                  >
                    {run.status === "done"
                      ? "Finished"
                      : `Stopped at ${run.done} of ${run.total}`}
                  </Badge>
                </li>
              );
            })}
          </ul>
        </DialogPanel>
      </DialogPopup>
    </Dialog>
  );
};

export const RecordDialog = ({
  open,
  onOpenChange,
  account,
}: Open & { account: Account }) => {
  const platform = PLATFORMS[account.platform];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Record how {platform.name} works</DialogTitle>
          <DialogDescription>
            This is how new platform support gets built: from what the site
            really does, not from guesses.
          </DialogDescription>
        </DialogHeader>
        <DialogPanel>
          <ol className="flex list-decimal flex-col gap-2 ps-5">
            <li>
              Vanish opens {platform.name} as {account.name}. Use a throwaway
              account, not your real one.
            </li>
            <li>
              Click around: open your profile, comments, posts and saved items,
              and delete one comment.
            </li>
            <li>
              Vanish keeps a copy of the requests {platform.name} makes.
              Cookies, tokens, passwords and email addresses are removed before
              anything is kept, and images, scripts and other sites are skipped.
            </li>
            <li>
              When you stop, you pick where the file goes. Read it before you
              share it.
            </li>
          </ol>
        </DialogPanel>
        <DialogFooter>
          <DialogClose render={<Button variant="ghost" />}>Cancel</DialogClose>
          <Button
            onClick={() => {
              onOpenChange(false);
              void attempt(() => vanish.startRecording(account.id));
            }}
          >
            Start recording
          </Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  );
};

export const RecordingBar = ({ state }: { state: AppState }) => {
  const recording = state.recording;
  if (!recording) return null;
  const account = state.accounts.find((row) => row.id === recording.accountId);
  const stop = async (save: boolean) => {
    const result = await attempt(() => vanish.stopRecording({ save }));
    if (result?.saved)
      notify(`Saved ${plural(result.count, "request", "requests")}.`);
  };
  return (
    <div
      className="flex items-center gap-3 border-b bg-destructive/8 px-8 py-2.5"
      role="status"
    >
      <span className="size-2 animate-pulse rounded-full bg-destructive" />
      <p className="flex-1">
        <span className="font-medium">Recording {account?.name}</span>
        <span className="text-muted-foreground">
          : {plural(recording.count, "request", "requests")} kept so far
        </span>
      </p>
      <Button variant="ghost" size="sm" onClick={() => void stop(false)}>
        Discard
      </Button>
      <Button size="sm" onClick={() => void stop(true)}>
        Stop and save
      </Button>
    </div>
  );
};

export const RemoveAccountDialog = ({
  open,
  onOpenChange,
  account,
}: Open & { account: Account }) => (
  <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogPopup className="max-w-md">
      <DialogHeader>
        <DialogTitle>Remove {account.name}?</DialogTitle>
        <DialogDescription>
          Vanish signs it out on this computer and forgets everything it found.
          Nothing changes on {PLATFORMS[account.platform].name}.
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <DialogClose render={<Button variant="ghost" />}>Cancel</DialogClose>
        <Button
          variant="destructive"
          onClick={() => {
            onOpenChange(false);
            void attempt(() => vanish.removeAccount(account.id));
          }}
        >
          Remove account
        </Button>
      </DialogFooter>
    </DialogPopup>
  </Dialog>
);
