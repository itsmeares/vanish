import { cn } from "@vanish/ui/utils";
import type { ReactNode } from "react";

import { titleBar } from "./bridge";

/**
 * The top row of a column. With a custom title bar it drags the window, and
 * leaves room for the Mac traffic lights on the left edge or the Windows and
 * Linux window buttons on the right edge.
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
      edge === "left" && titleBar === "darwin" && "pl-21",
      edge === "right" &&
        (titleBar === "win32" || titleBar === "linux") &&
        "pr-38",
      className,
    )}
  >
    {children}
  </div>
);
