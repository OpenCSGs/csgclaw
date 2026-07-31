import fs from "node:fs";
import path from "node:path";
import { app, autoUpdater } from "electron";
import type { DesktopUpdateStatus } from "../shared/desktopBridge.types";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import { usesMicrosoftStoreUpdates } from "./updatePolicy";

export class DesktopUpdater {
  private downloaded = false;
  private status: DesktopUpdateStatus;

  constructor(
    private readonly publishStatus: (status: DesktopUpdateStatus) => void,
    private readonly beforeInstall: () => Promise<void>,
  ) {
    this.status = {
      state: "idle",
      currentVersion: app.getVersion(),
    };
    this.bindEvents();
  }

  currentStatus(): DesktopUpdateStatus {
    return { ...this.status };
  }

  async checkForUpdates(): Promise<void> {
    if (this.downloaded) {
      this.publishStatus({ ...this.status });
      return;
    }
    if (usesMicrosoftStoreUpdates(process.platform, process.windowsStore)) {
      this.updateStatus({
        state: "unsupported",
        currentVersion: app.getVersion(),
        message: "Updates are managed automatically by Microsoft Store.",
      });
      return;
    }
    if (process.platform === DesktopPlatform.Linux) {
      this.updateStatus({
        state: "unsupported",
        currentVersion: app.getVersion(),
        message: "Linux desktop updates are provided by the system package manager.",
      });
      return;
    }
    const updateURL = resolveUpdateURL();
    if (!app.isPackaged || !updateURL) {
      this.updateStatus({
        state: "unsupported",
        currentVersion: app.getVersion(),
        message: app.isPackaged ? "Desktop update feed is not configured." : "Updates are disabled in development.",
      });
      return;
    }
    autoUpdater.setFeedURL({ url: updateURL });
    autoUpdater.checkForUpdates();
  }

  async installDownloadedUpdate(): Promise<void> {
    if (!this.downloaded) {
      throw new Error("No desktop update has been downloaded.");
    }
    await this.beforeInstall();
    autoUpdater.quitAndInstall();
  }

  private bindEvents(): void {
    autoUpdater.on("checking-for-update", () => {
      this.updateStatus({ state: "checking", currentVersion: app.getVersion() });
    });
    autoUpdater.on("update-available", () => {
      this.updateStatus({
        state: "available",
        currentVersion: app.getVersion(),
      });
    });
    autoUpdater.on("update-not-available", () => {
      this.updateStatus({ state: "not-available", currentVersion: app.getVersion() });
    });
    autoUpdater.on("update-downloaded", (_event, _releaseNotes, releaseName) => {
      this.downloaded = true;
      this.updateStatus({
        state: "downloaded",
        currentVersion: app.getVersion(),
        availableVersion: typeof releaseName === "string" ? releaseName : undefined,
      });
    });
    autoUpdater.on("error", (error) => {
      this.updateStatus({
        state: "error",
        currentVersion: app.getVersion(),
        message: error.message,
      });
    });
  }

  private updateStatus(status: DesktopUpdateStatus): void {
    this.status = status;
    this.publishStatus({ ...status });
  }
}

function resolveUpdateURL(): string {
  const configured = process.env.CSGCLAW_DESKTOP_UPDATE_URL?.trim();
  if (configured) {
    return normalizeHTTPSURL(configured);
  }
  try {
    const source = JSON.parse(
      fs.readFileSync(path.join(process.resourcesPath, "desktop-update.json"), "utf8"),
    ) as unknown;
    if (!source || typeof source !== "object" || Array.isArray(source)) {
      return "";
    }
    const baseURL = String((source as Record<string, unknown>).base_url || "").trim().replace(/\/+$/, "");
    if (!baseURL) {
      return "";
    }
    return normalizeHTTPSURL(`${baseURL}/${process.platform}/${process.arch}`);
  } catch {
    return "";
  }
}

function normalizeHTTPSURL(rawURL: string): string {
  try {
    const parsed = new URL(rawURL);
    if (parsed.protocol !== "https:" || parsed.username || parsed.password || parsed.search || parsed.hash) {
      return "";
    }
    return parsed.toString().replace(/\/+$/, "");
  } catch {
    return "";
  }
}
