import { useEffect, useState } from "react";

import type { AppState, DesktopApi } from "../shared/api";
import type { Kind } from "../shared/platforms";
import type { Item } from "../shared/rule";

type Bridge = {
  invoke: (name: string, ...args: unknown[]) => Promise<unknown>;
  on: (
    channel: "state" | "items",
    listener: (value: never) => void,
  ) => () => void;
};
const bridge = (window as unknown as { vanishBridge: Bridge }).vanishBridge;

/** The main process's calls, typed from the shared contract. */
export const vanish = new Proxy({} as DesktopApi, {
  get: (_target, name: string) => {
    if (name === "onState")
      return (listener: (state: AppState) => void) =>
        bridge.on("state", listener);
    if (name === "onItems")
      return (listener: (change: never) => void) =>
        bridge.on("items", listener);
    return (...args: unknown[]) => bridge.invoke(name, ...args);
  },
});

/** How the main process drew the window's top row: "darwin", "win32", "linux" or "none". */
export const titleBar =
  new URLSearchParams(location.search).get("titleBar") ?? "none";

export const useAppState = () => {
  const [state, setState] = useState<AppState | null>(null);
  useEffect(() => {
    const off = vanish.onState(setState);
    void vanish.getState().then(setState);
    return off;
  }, []);
  return state;
};

export const useItems = (accountId: number | null, kind: Kind) => {
  const [items, setItems] = useState<{ key: string; items: Item[] } | null>(
    null,
  );
  const key = `${accountId}:${kind}`;
  useEffect(() => {
    if (accountId === null) return;
    let current = true;
    const load = () =>
      void vanish.items(accountId, kind).then((next) => {
        if (current) setItems({ key, items: next });
      });
    load();
    const off = vanish.onItems((change) => {
      if (change.accountId === accountId && change.kind === kind) load();
    });
    return () => {
      current = false;
      off();
    };
  }, [accountId, kind, key]);
  // Never show the previous account's list while the next one loads.
  return items?.key === key ? items.items : null;
};
