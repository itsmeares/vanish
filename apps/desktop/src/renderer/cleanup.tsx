import { Button } from "@vanish/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogPopup,
  DialogTitle,
} from "@vanish/ui/components/dialog";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@vanish/ui/components/empty";
import {
  Menu,
  MenuPopup,
  MenuRadioGroup,
  MenuRadioItem,
  MenuTrigger,
} from "@vanish/ui/components/menu";
import { Progress } from "@vanish/ui/components/progress";
import { Spinner } from "@vanish/ui/components/spinner";
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@vanish/ui/components/toggle-group";
import {
  Tooltip,
  TooltipPopup,
  TooltipTrigger,
} from "@vanish/ui/components/tooltip";
import {
  ChevronDownIcon,
  ExternalLinkIcon,
  FileArchiveIcon,
  PauseIcon,
  PlayIcon,
  RefreshCwIcon,
  SquareIcon,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import type { Account, Mode, Run, Settings } from "../shared/api";
import { hasText, type Kind, type Platform } from "../shared/platforms";
import { judge, type Rule } from "../shared/rule";
import { useItems, vanish } from "./bridge";
import { count, doneWord, plural } from "./format";
import { ItemList, type Row } from "./item-list";
import { attempt, notify } from "./notify";
import { ReadingPane } from "./reading-pane";
import { Sentence } from "./sentence";
import { TopRow } from "./top-row";

const MODE_LABELS: Record<Mode, { action: string; detail: string }> = {
  "overwrite-delete": {
    action: "Overwrite, then delete",
    detail:
      "Replaces the text first, so the original is gone from the site too.",
  },
  overwrite: {
    action: "Overwrite only",
    detail: "Replaces the text and leaves the comment or post up.",
  },
  delete: {
    action: "Delete only",
    detail: "Deletes without replacing the text first.",
  },
};

const actionLabel = (
  platform: Platform,
  kind: Kind,
  mode: Mode,
  total: number,
) =>
  hasText(kind)
    ? `${MODE_LABELS[mode].action} ${count(total)}`
    : `${platform.kinds[kind].verb} ${count(total)}`;

const RunBar = ({ platform, run }: { platform: Platform; run: Run }) => {
  const names = platform.kinds[run.kind];
  const finished = run.done + run.failed;
  const paused = run.status === "paused";
  return (
    <div className="flex flex-col gap-3 rounded-xl border bg-background px-5 py-4">
      <div className="flex items-center gap-4">
        {paused ? null : <Spinner className="size-4 text-primary" />}
        <p className="flex-1">
          <span className="font-medium">
            {paused
              ? "Paused"
              : `${hasText(run.kind) && run.mode === "overwrite" ? "Overwriting" : names.doing} ${names.many}`}
          </span>
          <span className="text-muted-foreground">
            {": "}
            {count(run.done)} of {count(run.total)}{" "}
            {doneWord(platform, run.kind, run.mode)}
            {run.failed > 0 && `, ${count(run.failed)} failed`}
          </span>
        </p>
        {paused ? (
          <Button onClick={() => void attempt(() => vanish.resumeRun(run.id))}>
            <PlayIcon />
            {run.error ? "Try again" : "Resume"}
          </Button>
        ) : (
          <Button
            variant="outline"
            onClick={() => void attempt(() => vanish.pauseRun(run.id))}
          >
            <PauseIcon />
            Pause
          </Button>
        )}
        <Button
          variant="ghost-muted"
          onClick={() => void attempt(() => vanish.stopRun(run.id))}
        >
          <SquareIcon />
          Stop
        </Button>
      </div>
      <Progress value={run.total === 0 ? 0 : (finished / run.total) * 100} />
      {run.error && (
        <p className="text-meta text-destructive-foreground">
          Stopped on an error {platform.name} sent: {run.error}
        </p>
      )}
    </div>
  );
};

export const Cleanup = ({
  account,
  platform,
  kind,
  rule,
  run,
  settings,
  onKind,
  onRule,
  onSettings,
  onReadExport,
}: {
  account: Account;
  platform: Platform;
  kind: Kind;
  rule: Rule;
  run: Run | null;
  settings: Settings;
  onKind: (kind: Kind) => void;
  onRule: (rule: Rule) => void;
  onSettings: (change: Partial<Settings>) => void;
  onReadExport: () => void;
}) => {
  const items = useItems(account.id, kind);
  const [filter, setFilter] = useState<"goes" | "kept" | "all">("goes");
  const [selected, setSelected] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const locked = run !== null;
  const names = platform.kinds[kind];
  const mode = hasText(kind) ? settings.mode : "delete";

  const { rows, goes, kept, now } = useMemo(() => {
    const judgedAt = Date.now();
    const verdict = judge(rule, judgedAt, platform.community.prefix);
    const all: Row[] = [];
    let goesCount = 0;
    for (const item of items ?? []) {
      const result = verdict(item);
      if (result.type === "out") continue;
      if (result.type === "goes") goesCount++;
      all.push({ item, verdict: result });
    }
    return {
      rows: all,
      goes: goesCount,
      kept: all.length - goesCount,
      now: judgedAt,
    };
  }, [items, rule, platform]);

  const shown = useMemo(
    () =>
      filter === "all"
        ? rows
        : rows.filter(
            (row) => (filter === "goes") === (row.verdict.type === "goes"),
          ),
    [rows, filter],
  );
  const current = rows.find((row) => row.item.id === selected) ?? null;

  useEffect(() => setSelected(null), [account.id, kind]);

  const toggle = (row: Row) => {
    if (locked) return;
    if (row.verdict.type === "goes")
      onRule({ ...rule, keepIds: [...rule.keepIds, row.item.id] });
    else if (row.verdict.reason === "kept by you")
      onRule({
        ...rule,
        keepIds: rule.keepIds.filter((id) => id !== row.item.id),
      });
  };

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      if (target.closest("input, textarea, [role=dialog], [role=menu]")) return;
      const index = shown.findIndex((row) => row.item.id === selected);
      if (event.key === "j" || event.key === "ArrowDown") {
        const next = shown[Math.min(shown.length - 1, index + 1)];
        if (next) setSelected(next.item.id);
      } else if (event.key === "k" || event.key === "ArrowUp") {
        const next = shown[Math.max(0, index - 1)];
        if (next) setSelected(next.item.id);
      } else if (event.key === "x" && current) {
        toggle(current);
      } else if (event.key === "Escape") {
        setSelected(null);
      } else return;
      event.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const start = async () => {
    setBusy(true);
    const runId = await attempt(() =>
      vanish.startRun({ accountId: account.id, kind, rule, mode, now }),
    );
    setBusy(false);
    setConfirming(false);
    if (runId !== undefined) setSelected(null);
  };

  const backup = async () => {
    const result = await attempt(() =>
      vanish.saveBackup({ accountId: account.id, kind, rule, now }),
    );
    if (result?.saved)
      notify(
        `Saved a backup of ${plural(result.count, names.one, names.many)}.`,
      );
  };

  const total = account.counts[kind];

  return (
    <>
      <main className="flex min-w-0 flex-1 flex-col bg-card">
        <TopRow edge={current ? undefined : "right"} className="gap-3 px-8">
          <h1 className="text-headline font-semibold">{names.label}</h1>
          <span className="text-muted-foreground">{count(total)} found</span>
          <span className="flex-1" />
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost-muted"
                  size="sm"
                  disabled={!account.canScan || locked}
                  onClick={() =>
                    void attempt(() => vanish.scan(account.id, kind))
                  }
                />
              }
            >
              <RefreshCwIcon />
              Scan again
            </TooltipTrigger>
            {!account.canScan && (
              <TooltipPopup>
                Scanning {platform.name} arrives with {platform.name} support.
              </TooltipPopup>
            )}
          </Tooltip>
          <Button
            variant="ghost-muted"
            size="sm"
            onClick={() => void attempt(() => vanish.openBrowser(account.id))}
          >
            <ExternalLinkIcon />
            Open {platform.name}
          </Button>
        </TopRow>

        {items === null ? (
          <div className="grid flex-1 place-items-center">
            <Spinner className="size-5 text-muted-foreground" />
          </div>
        ) : total === 0 ? (
          <Empty className="flex-1">
            <EmptyHeader>
              <EmptyTitle>No {names.many} found yet</EmptyTitle>
              <EmptyDescription>
                {account.scannedAt
                  ? `There are none in what Vanish has read so far.`
                  : `Read your ${platform.name} data export to find all your ${names.many}, back to the first one.`}
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent className="flex-row justify-center">
              <Button onClick={onReadExport}>
                <FileArchiveIcon />
                Read data export
              </Button>
              <Button
                variant="outline"
                onClick={() =>
                  void attempt(() =>
                    vanish.openBrowser(account.id, platform.exportUrl),
                  )
                }
              >
                Ask {platform.name} for it
              </Button>
            </EmptyContent>
          </Empty>
        ) : (
          <>
            <div className="flex flex-col gap-5 px-8 pt-4 pb-5">
              <Sentence
                platform={platform}
                kind={kind}
                counts={account.counts}
                items={items}
                rule={rule}
                disabled={locked}
                onKind={onKind}
                onRule={onRule}
              />
              {run ? (
                <RunBar platform={platform} run={run} />
              ) : (
                <div className="flex items-center gap-4 rounded-xl border bg-background py-3 pr-3 pl-5">
                  <p className="flex-1">
                    <span className="font-semibold">
                      {plural(goes, names.one, names.many)}
                    </span>{" "}
                    will be {doneWord(platform, kind, mode)}.{" "}
                    {kept > 0 && (
                      <span className="text-muted-foreground">
                        {count(kept)} kept by your rules and picks.
                      </span>
                    )}
                  </p>
                  <Button
                    variant="outline"
                    disabled={goes === 0}
                    onClick={() => void backup()}
                  >
                    Save backup first
                  </Button>
                  <div className="flex">
                    <Button
                      className={hasText(kind) ? "rounded-e-none" : undefined}
                      disabled={goes === 0}
                      onClick={() => setConfirming(true)}
                    >
                      {actionLabel(platform, kind, mode, goes)}
                    </Button>
                    {hasText(kind) && (
                      <Menu>
                        <MenuTrigger
                          render={
                            <Button
                              className="rounded-s-none border-s-primary-foreground/20"
                              size="icon"
                              aria-label="Choose how to remove"
                            />
                          }
                        >
                          <ChevronDownIcon />
                        </MenuTrigger>
                        <MenuPopup align="end" className="w-80">
                          <MenuRadioGroup
                            value={mode}
                            onValueChange={(value) =>
                              onSettings({ mode: value as Mode })
                            }
                          >
                            {(Object.keys(MODE_LABELS) as Mode[]).map(
                              (option) => (
                                <MenuRadioItem
                                  key={option}
                                  value={option}
                                  className="items-start py-2"
                                >
                                  <span className="flex flex-col gap-0.5">
                                    <span>{MODE_LABELS[option].action}</span>
                                    <span className="text-meta text-muted-foreground">
                                      {MODE_LABELS[option].detail}
                                    </span>
                                  </span>
                                </MenuRadioItem>
                              ),
                            )}
                          </MenuRadioGroup>
                        </MenuPopup>
                      </Menu>
                    )}
                  </div>
                </div>
              )}
            </div>
            <div className="flex items-center gap-3 border-y bg-background px-8 py-2">
              <ToggleGroup
                value={[filter]}
                onValueChange={(next) =>
                  next[0] && setFilter(next[0] as typeof filter)
                }
                size="sm"
              >
                <ToggleGroupItem value="goes">
                  On the list{" "}
                  <span className="text-muted-foreground">{count(goes)}</span>
                </ToggleGroupItem>
                <ToggleGroupItem value="kept">
                  Kept{" "}
                  <span className="text-muted-foreground">{count(kept)}</span>
                </ToggleGroupItem>
                <ToggleGroupItem value="all">
                  All{" "}
                  <span className="text-muted-foreground">
                    {count(rows.length)}
                  </span>
                </ToggleGroupItem>
              </ToggleGroup>
            </div>
            {shown.length === 0 ? (
              <Empty className="flex-1">
                <EmptyHeader>
                  <EmptyTitle>Nothing here</EmptyTitle>
                  <EmptyDescription>
                    {rows.length === 0
                      ? `No ${names.many} match the sentence. Change a part of it above.`
                      : filter === "kept"
                        ? "Nothing is kept. Add a keep rule, or press X on a row."
                        : "Everything that matches is kept."}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <ItemList
                platform={platform}
                rows={shown}
                selected={selected}
                locked={locked}
                onSelect={setSelected}
                onToggle={toggle}
              />
            )}
          </>
        )}
      </main>

      {current && (
        <ReadingPane
          platform={platform}
          row={current}
          locked={locked}
          onToggle={() => toggle(current)}
          onOpen={() =>
            void attempt(() => vanish.openBrowser(account.id, current.item.url))
          }
          onClose={() => setSelected(null)}
        />
      )}

      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogPopup>
          <DialogHeader>
            <DialogTitle>
              {hasText(kind) && mode === "overwrite"
                ? `Overwrite ${plural(goes, names.one, names.many)}?`
                : `${names.verb} ${plural(goes, names.one, names.many)}?`}
            </DialogTitle>
            <DialogDescription>
              {hasText(kind) ? `${MODE_LABELS[mode].detail} ` : ""}
              Vanish works through them one at a time, with a short pause
              between each, so a big list takes a while. You can pause or stop
              at any time. What is done on {platform.name} cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose render={<Button variant="ghost" />}>
              Cancel
            </DialogClose>
            <Button loading={busy} onClick={() => void start()}>
              {actionLabel(platform, kind, mode, goes)}
            </Button>
          </DialogFooter>
        </DialogPopup>
      </Dialog>
    </>
  );
};
