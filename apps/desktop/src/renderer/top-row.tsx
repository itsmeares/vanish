import { cn } from "@vanish/ui/utils";
import type { ReactNode } from "react";

import { titleBar } from "./bridge";

/**
 * The top row of a column. With a custom title bar it drags the window, and
 * keeps room for the Mac traffic lights on the left edge or the Windows and
 * Linux window buttons on the right edge. The room is its own element, so a
 * caller's padding cannot take it away.
 */
export const TopRow = ({
  edge,
  className,
  children,
}: {
  edge?: "left" | "right";
  className?: string;
  children?: ReactNode;
}) => (
  <div
    className={cn(
      "flex h-titlebar shrink-0 items-center gap-2 px-4 [&_a]:no-drag [&_button]:no-drag [&_input]:no-drag",
      titleBar !== "none" && "drag-region",
      className,
    )}
  >
    {edge === "left" && titleBar === "darwin" && (
      <span className="w-17 shrink-0" />
    )}
    {children}
    {edge === "right" && (titleBar === "win32" || titleBar === "linux") && (
      <span className="w-34 shrink-0" />
    )}
  </div>
);
