import { Badge } from "@vanish/ui/components/badge";
import { Button } from "@vanish/ui/components/button";
import { Kbd } from "@vanish/ui/components/kbd";
import { ExternalLinkIcon, XIcon } from "lucide-react";

import type { Platform } from "../shared/platforms";
import { day, points } from "./format";
import type { Row } from "./item-list";
import { TopRow } from "./top-row";

export const ReadingPane = ({
  platform,
  row,
  locked,
  onToggle,
  onOpen,
  onClose,
}: {
  platform: Platform;
  row: Row;
  locked: boolean;
  onToggle: () => void;
  onOpen: () => void;
  onClose: () => void;
}) => {
  const { item, verdict } = row;
  const kept = verdict.type === "kept";
  const byRule = kept && verdict.reason !== "kept by you";
  const showTitle = item.title && item.title !== item.body;
  return (
    <aside
      className="flex w-pane shrink-0 flex-col border-l bg-background"
      aria-label="Selected item"
    >
      <TopRow edge="right" className="px-6">
        <span className="text-meta text-muted-foreground">
          {platform.community.prefix}
          {item.community}
        </span>
        <span className="flex-1" />
        <Button
          variant="ghost-muted"
          size="icon-sm"
          className="no-drag"
          aria-label="Close"
          onClick={onClose}
        >
          <XIcon />
        </Button>
      </TopRow>
      <div className="flex min-h-0 flex-1 scrollbar-thin flex-col gap-4 overflow-y-auto px-6 pb-6">
        <div className="flex flex-wrap items-center gap-3 text-meta text-muted-foreground">
          {item.createdAt !== null && <span>{day(item.createdAt)}</span>}
          {item.score !== null && <span>{points(item.score)}</span>}
          {kept ? (
            <Badge variant="outline">Kept: {verdict.reason}</Badge>
          ) : (
            <Badge variant="info">On the list</Badge>
          )}
        </div>
        {showTitle &&
          (item.kind === "comments" ? (
            <p className="text-meta text-muted-foreground">
              In the thread{" "}
              <span className="font-medium text-foreground">{item.title}</span>
            </p>
          ) : (
            <h2 className="text-lg font-semibold text-pretty">{item.title}</h2>
          ))}
        {item.body && (
          <p className="leading-relaxed whitespace-pre-wrap">{item.body}</p>
        )}
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={locked || byRule}
            onClick={onToggle}
          >
            {kept ? "Put back on the list" : "Keep this one"}
          </Button>
          <Button variant="ghost-muted" onClick={onOpen}>
            <ExternalLinkIcon />
            Open on {platform.name}
          </Button>
        </div>
        {byRule && (
          <p className="text-meta text-muted-foreground">
            A keep rule saves this one. Remove the rule in the sentence to put
            it back.
          </p>
        )}
      </div>
      <p className="flex items-center gap-1.5 border-t px-6 py-3 text-meta text-muted-foreground">
        <Kbd>J</Kbd>
        <Kbd>K</Kbd>
        move, <Kbd>X</Kbd> keep or put back, <Kbd>Esc</Kbd> close
      </p>
    </aside>
  );
};
