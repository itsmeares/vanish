import { Button } from "@vanish/ui/components/button";
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@vanish/ui/components/toggle-group";
import { useMemo, useRef, useState } from "react";

import { AGE_PRESETS, ageLabel, type Item, type Rule } from "../shared/rule";
import { count, month } from "./format";

type Age = Rule["age"];

const monthStart = (time: number) => {
  const date = new Date(time);
  return new Date(date.getFullYear(), date.getMonth(), 1).getTime();
};
const nextMonth = (time: number) => {
  const date = new Date(time);
  return new Date(date.getFullYear(), date.getMonth() + 1, 1).getTime();
};

const HEIGHT = 96;

/** Your activity by month. Drag across it to pick a period. */
export const TimeChart = ({
  items,
  age,
  onAge,
}: {
  items: Item[];
  age: Age;
  onAge: (age: Age) => void;
}) => {
  const months = useMemo(() => {
    if (items.length === 0) return [];
    let first = Infinity;
    let last = -Infinity;
    for (const item of items) {
      first = Math.min(first, item.createdAt);
      last = Math.max(last, item.createdAt);
    }
    const starts: number[] = [];
    for (let at = monthStart(first); at <= last; at = nextMonth(at))
      starts.push(at);
    const tally = new Map(starts.map((start) => [start, 0]));
    for (const item of items) {
      const start = monthStart(item.createdAt);
      tally.set(start, (tally.get(start) ?? 0) + 1);
    }
    return starts.map((start) => ({ start, total: tally.get(start) ?? 0 }));
  }, [items]);

  const [drag, setDrag] = useState<{ from: number; to: number } | null>(null);
  const chart = useRef<HTMLDivElement>(null);
  const tallest = Math.max(1, ...months.map((row) => row.total));
  const now = Date.now();

  const monthAt = (clientX: number) => {
    const box = chart.current?.getBoundingClientRect();
    if (!box || months.length === 0) return null;
    const index = Math.min(
      months.length - 1,
      Math.max(
        0,
        Math.floor(((clientX - box.left) / box.width) * months.length),
      ),
    );
    return months[index].start;
  };

  const shownAge: Age = drag
    ? {
        type: "between",
        from: Math.min(drag.from, drag.to),
        to: nextMonth(Math.max(drag.from, drag.to)),
      }
    : age;
  const range = (shown: Age): [number, number] =>
    shown.type === "any"
      ? [-Infinity, Infinity]
      : shown.type === "older"
        ? [-Infinity, now - shown.days * 86_400_000]
        : [shown.from, shown.to];
  const [from, to] = range(shownAge);
  // A month is drawn as picked when it starts inside the period.
  const inside = (start: number) => start >= from && start < to;
  const picked = items.filter(
    (item) => item.createdAt >= from && item.createdAt < to,
  ).length;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-meta text-muted-foreground">Older than</span>
        <ToggleGroup
          variant="outline"
          size="sm"
          value={age.type === "older" ? [String(age.days)] : []}
          onValueChange={(next) =>
            next[0] && onAge({ type: "older", days: Number(next[0]) })
          }
        >
          {AGE_PRESETS.map((preset) => (
            <ToggleGroupItem key={preset.days} value={String(preset.days)}>
              {preset.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Button
          size="sm"
          variant={age.type === "any" ? "secondary" : "ghost-muted"}
          onClick={() => onAge({ type: "any" })}
        >
          Any time
        </Button>
      </div>
      {months.length > 0 && (
        <div>
          <div
            ref={chart}
            role="slider"
            aria-label="Pick a period by dragging across the months"
            aria-valuetext={ageLabel(age)}
            aria-valuenow={picked}
            tabIndex={0}
            className="flex cursor-col-resize touch-none items-end gap-px rounded-md select-none"
            style={{ height: HEIGHT }}
            onPointerDown={(event) => {
              const start = monthAt(event.clientX);
              if (start === null) return;
              event.currentTarget.setPointerCapture(event.pointerId);
              setDrag({ from: start, to: start });
            }}
            onPointerMove={(event) => {
              if (!drag) return;
              const start = monthAt(event.clientX);
              if (start !== null) setDrag({ ...drag, to: start });
            }}
            onPointerUp={() => {
              if (!drag) return;
              onAge({
                type: "between",
                from: Math.min(drag.from, drag.to),
                to: nextMonth(Math.max(drag.from, drag.to)),
              });
              setDrag(null);
            }}
          >
            {months.map((row) => (
              <div
                key={row.start}
                title={`${month(row.start)}: ${count(row.total)}`}
                className={
                  inside(row.start)
                    ? "flex-1 rounded-t-[2px] bg-primary"
                    : "flex-1 rounded-t-[2px] bg-line-strong"
                }
                style={{
                  height: `${Math.max(2, (row.total / tallest) * HEIGHT)}px`,
                }}
              />
            ))}
          </div>
          <div className="mt-1.5 flex justify-between text-label text-muted-foreground">
            <span>{month(months[0].start)}</span>
            <span>{month(months[months.length - 1].start)}</span>
          </div>
        </div>
      )}
      <p className="text-meta text-muted-foreground">
        <span className="font-medium text-foreground">{count(picked)}</span>{" "}
        {ageLabel(shownAge)}. Drag across the chart to pick a period.
      </p>
    </div>
  );
};
