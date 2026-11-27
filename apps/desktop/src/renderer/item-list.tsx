import { Badge } from "@vanish/ui/components/badge";
import { Checkbox } from "@vanish/ui/components/checkbox";
import { cn } from "@vanish/ui/utils";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useRef } from "react";

import type { Platform } from "../shared/platforms";
import type { Item, Verdict } from "../shared/rule";
import { month, points } from "./format";

export type Row = { item: Item; verdict: Exclude<Verdict, { type: "out" }> };

export const itemText = (item: Item) =>
  item.body.trim() ? item.body : item.title;

export const ItemList = ({
  platform,
  rows,
  selected,
  locked,
  onSelect,
  onToggle,
}: {
  platform: Platform;
  rows: Row[];
  selected: string | null;
  /** While a run goes, the list is frozen and can only be read. */
  locked: boolean;
  onSelect: (id: string) => void;
  onToggle: (row: Row) => void;
}) => {
  const scroller = useRef<HTMLDivElement>(null);
  const list = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => 64,
    overscan: 12,
  });
  const selectedIndex = rows.findIndex((row) => row.item.id === selected);
  useEffect(() => {
    if (selectedIndex >= 0)
      list.scrollToIndex(selectedIndex, { align: "auto" });
  }, [list, selectedIndex]);

  return (
    <div
      ref={scroller}
      className="min-h-0 flex-1 scrollbar-thin overflow-y-auto"
      role="list"
    >
      <div className="relative" style={{ height: list.getTotalSize() }}>
        {list.getVirtualItems().map((virtual) => {
          const { item, verdict } = rows[virtual.index];
          const kept = verdict.type === "kept";
          // Rule-kept items change with the rule; only your own picks toggle here.
          const toggleable =
            !locked &&
            (verdict.type === "goes" || verdict.reason === "kept by you");
          return (
            <div
              key={item.id}
              role="listitem"
              aria-selected={item.id === selected}
              className={cn(
                "absolute inset-x-0 flex h-16 cursor-default items-start gap-3 border-b border-hairline px-6 pt-3",
                item.id === selected ? "bg-selected" : "hover:bg-hover",
              )}
              style={{ transform: `translateY(${virtual.start}px)` }}
              onClick={() => onSelect(item.id)}
            >
              <Checkbox
                className="mt-0.5"
                checked={!kept}
                disabled={!toggleable}
                aria-label={kept ? "Put back on the list" : "Keep this one"}
                onClick={(event) => event.stopPropagation()}
                onCheckedChange={() => onToggle(rows[virtual.index])}
              />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-3 text-meta text-muted-foreground">
                  <span className="font-medium text-foreground">
                    {platform.community.prefix}
                    {item.community}
                  </span>
                  {item.createdAt !== null && (
                    <span>{month(item.createdAt)}</span>
                  )}
                  {item.score !== null && <span>{points(item.score)}</span>}
                  {kept && (
                    <Badge variant="outline" size="sm">
                      Kept: {verdict.reason}
                    </Badge>
                  )}
                </div>
                <p
                  className={cn(
                    "mt-0.5 truncate",
                    kept ? "text-muted-foreground" : "text-foreground",
                  )}
                >
                  {itemText(item)}
                </p>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
