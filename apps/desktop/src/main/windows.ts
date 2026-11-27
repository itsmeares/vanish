import path from "node:path";

import {
  app,
  BrowserWindow,
  nativeTheme,
  session,
  shell,
  type WebContents,
} from "electron";

const PRELOAD = path.join(__dirname, "../preload/index.cjs");
const PAGE = path.join(__dirname, "../renderer/index.html");
const ICON = app.isPackaged
  ? path.join(process.resourcesPath, "icon.png")
  : path.join(__dirname, "../../resources/icon.png");

// The canvas colors from packages/ui/styles/tokens.css, so a window never flashes white.
const background = () =>
  nativeTheme.shouldUseDarkColors ? "#0f141b" : "#f3f5f8";
const symbols = () => (nativeTheme.shouldUseDarkColors ? "#e6ebf2" : "#121a26");

const TITLE_BAR_HEIGHT = 44;
const OVERLAY_DESKTOPS = new Set(["gnome", "ubuntu", "kde"]);

/**
 * Whether the app draws its own top row. On Linux only desktops known to show
 * the window buttons qualify: tiling desktops manage windows themselves, and an
 * unknown desktop keeps its own bar so a window can always be closed.
 */
export const customTitleBar =
  process.platform !== "linux" ||
  (process.env.XDG_CURRENT_DESKTOP ?? "")
    .split(":")
    .some((desktop) => OVERLAY_DESKTOPS.has(desktop.toLowerCase()));

const titleBar = (): Electron.BrowserWindowConstructorOptions => {
  if (!customTitleBar) return {};
  if (process.platform === "darwin")
    return {
      titleBarStyle: "hiddenInset",
      trafficLightPosition: { x: 16, y: 15 },
    };
  return {
    titleBarStyle: "hidden",
    titleBarOverlay: {
      color: background(),
      symbolColor: symbols(),
      height: TITLE_BAR_HEIGHT,
    },
  };
};

export const createMainWindow = () => {
  const window = new BrowserWindow({
    width: 1320,
    height: 860,
    minWidth: 980,
    minHeight: 620,
    show: false,
    title: "Vanish",
    icon: ICON,
    backgroundColor: background(),
    ...titleBar(),
    webPreferences: {
      preload: PRELOAD,
      contextIsolation: true,
      sandbox: true,
      spellcheck: false,
    },
  });
  nativeTheme.on("updated", () => {
    if (window.isDestroyed()) return;
    window.setBackgroundColor(background());
    if (customTitleBar && process.platform !== "darwin")
      window.setTitleBarOverlay({
        color: background(),
        symbolColor: symbols(),
      });
  });
  // The app page never navigates; links open in the system browser.
  window.webContents.on("will-navigate", (event) => event.preventDefault());
  window.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith("https://")) void shell.openExternal(url);
    return { action: "deny" };
  });
  window.once("ready-to-show", () => window.show());
  void window.loadFile(PAGE, {
    query: { titleBar: customTitleBar ? process.platform : "none" },
  });
  return window;
};

export const accountPartition = (accountId: number) =>
  `persist:account-${accountId}`;

const hardened = new Set<string>();
const accountSession = (accountId: number) => {
  const partition = accountPartition(accountId);
  const browsing = session.fromPartition(partition);
  if (!hardened.has(partition)) {
    hardened.add(partition);
    // Sites get no camera, microphone, location or notifications.
    browsing.setPermissionRequestHandler((_contents, _permission, allow) =>
      allow(false),
    );
    browsing.setPermissionCheckHandler(() => false);
  }
  return browsing;
};

const browsers = new Map<number, BrowserWindow>();

/**
 * A plain browser window signed in as one account, with no access to Vanish
 * itself. `onOpen` runs for it and for every pop-up it opens, before a page
 * loads, so a recording can start in time to see the first page.
 */
export const openAccountBrowser = async (
  accountId: number,
  title: string,
  url: string,
  onOpen: (contents: WebContents) => Promise<void> | void,
) => {
  const existing = browsers.get(accountId);
  if (existing && !existing.isDestroyed()) {
    await onOpen(existing.webContents);
    existing.focus();
    void existing.loadURL(url).catch(() => {});
    return;
  }
  const browsing = accountSession(accountId);
  const webPreferences = {
    session: browsing,
    contextIsolation: true,
    sandbox: true,
    nodeIntegration: false,
  };
  const window = new BrowserWindow({
    width: 1180,
    height: 860,
    title,
    icon: ICON,
    backgroundColor: background(),
    autoHideMenuBar: true,
    webPreferences,
  });
  // Sign-in pages open pop-ups; they stay in this account's session.
  window.webContents.setWindowOpenHandler(() => ({
    action: "allow",
    overrideBrowserWindowOptions: { autoHideMenuBar: true, webPreferences },
  }));
  window.on("page-title-updated", (event) => {
    event.preventDefault();
  });
  window.webContents.on("did-create-window", (child) => {
    void onOpen(child.webContents);
  });
  window.on("closed", () => browsers.delete(accountId));
  browsers.set(accountId, window);
  // A window answers the recorder only once it shows a page.
  await window.loadURL("about:blank");
  await onOpen(window.webContents);
  // A failed load shows Chromium's error page in the window; nothing to do here.
  void window.loadURL(url).catch(() => {});
};

/** Signs the account out everywhere on this computer and forgets its browsing data. */
export const forgetAccountSession = async (accountId: number) => {
  browsers.get(accountId)?.destroy();
  browsers.delete(accountId);
  const browsing = accountSession(accountId);
  await browsing.clearStorageData();
  await browsing.clearCache();
};
