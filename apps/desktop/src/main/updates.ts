import { app, shell } from "electron";
import electronUpdater from "electron-updater";

import type { Update } from "../shared/api";
import { isNewer } from "../shared/version";

const RELEASES = "https://github.com/itsmeares/vanish/releases";
const CHECK_MS = 6 * 60 * 60_000;

export type UpdateCheck = "found" | "current" | "failed";

/**
 * Windows and Linux update themselves from GitHub Releases. Mac updates need
 * a paid Apple certificate, which Vanish does not have, so the Mac app only
 * says a new version exists and opens the download page.
 */
export const startUpdates = (onChange: (update: Update | null) => void) => {
  if (!app.isPackaged)
    return {
      install: async () => {},
      check: async (): Promise<UpdateCheck> => "current",
    };

  if (process.platform === "darwin") {
    let latest: string | null = null;
    const check = async (): Promise<UpdateCheck> => {
      try {
        const response = await fetch(
          "https://api.github.com/repos/itsmeares/vanish/releases/latest",
          { headers: { accept: "application/vnd.github+json" } },
        );
        if (!response.ok) return "failed";
        const { tag_name: tag } = (await response.json()) as {
          tag_name: string;
        };
        const version = tag.replace(/^v/, "");
        if (!isNewer(version, app.getVersion())) return "current";
        latest = version;
        onChange({ version, ready: true, manual: true });
        return "found";
      } catch {
        return "failed";
      }
    };
    void check();
    setInterval(check, CHECK_MS);
    return {
      install: async () => {
        await shell.openExternal(
          latest ? `${RELEASES}/tag/v${latest}` : RELEASES,
        );
      },
      check,
    };
  }

  const { autoUpdater } = electronUpdater;
  autoUpdater.autoInstallOnAppQuit = true;
  autoUpdater.on("update-available", (info) =>
    onChange({ version: info.version, ready: false, manual: false }),
  );
  autoUpdater.on("update-downloaded", (info) =>
    onChange({ version: info.version, ready: true, manual: false }),
  );
  autoUpdater.on("error", () => {
    // Offline, or no release for this system yet; the next check retries.
  });
  const check = async (): Promise<UpdateCheck> => {
    try {
      const result = await autoUpdater.checkForUpdates();
      return result?.isUpdateAvailable ? "found" : "current";
    } catch {
      return "failed";
    }
  };
  void check();
  setInterval(check, CHECK_MS);
  return { install: async () => autoUpdater.quitAndInstall(), check };
};
