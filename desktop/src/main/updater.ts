import fs from "node:fs";
import path from "node:path";
import { app, autoUpdater, net } from "electron";
import type {
  DesktopUpdateChannel,
  DesktopUpdateStatus,
} from "../shared/desktopBridge.types";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import { compareDesktopReleaseVersions, inferDesktopUpdateChannel } from "../shared/releaseVersion";
import {
  desktopInstallerArtifact,
  desktopDownloadsManifestURL,
  desktopUpdateFeedOptions,
  desktopUpdateFeedURL,
  DEFAULT_UPDATE_CHANNELS_BASE_URL,
  parseDesktopDownloadsManifest,
  type DesktopDownloadsManifest,
} from "./updateFeed";
import {
  downloadVerifiedArtifact,
  isVerifiedArtifact,
  launchWindowsChannelInstaller,
  missingSignedMacUpdatePackageError,
  parseMacChannelUpdate,
  startMacChannelUpdateFeed,
  type LocalUpdateFeed,
} from "./channelSwitchUpdate";
import { logDesktopInfo } from "./desktopLogger";
import {
  shouldInstallDesktopVersion,
  usesMicrosoftStoreUpdates,
} from "./updatePolicy";

const STARTUP_CHECK_DELAY_MS = 5_000;
const PERIODIC_CHECK_INTERVAL_MS = 60 * 60 * 1000;
const UPDATE_PREFERENCES_FILE = "desktop-update-preferences.json";

export class DesktopUpdater {
  private channel: DesktopUpdateChannel;
  private checkActive = false;
  private downloaded = false;
  private expectedVersion = "";
  private installWhenDownloaded = false;
  private installing = false;
  private channelSwitchActive = false;
  private channelBeforeSwitch: DesktopUpdateChannel | null = null;
  private macChannelFeed: LocalUpdateFeed | null = null;
  private windowsChannelInstallerPath = "";
  private periodicTimer: ReturnType<typeof setInterval> | null = null;
  private startupTimer: ReturnType<typeof setTimeout> | null = null;
  private status: DesktopUpdateStatus;

  constructor(
    private readonly publishStatus: (status: DesktopUpdateStatus) => void,
    private readonly beforeInstall: () => Promise<void>,
  ) {
    this.channel = loadUpdateChannel();
    this.status = {
      state: "idle",
      channel: this.channel,
      currentVersion: app.getVersion(),
    };
    this.bindEvents();
  }

  currentStatus(): DesktopUpdateStatus {
    return { ...this.status };
  }

  startBackgroundChecks(): void {
    if (this.startupTimer === null) {
      this.startupTimer = setTimeout(() => {
        this.startupTimer = null;
        void this.checkForUpdates().catch(() => undefined);
      }, STARTUP_CHECK_DELAY_MS);
    }
    if (this.periodicTimer === null) {
      this.periodicTimer = setInterval(() => {
        void this.checkForUpdates().catch(() => undefined);
      }, PERIODIC_CHECK_INTERVAL_MS);
    }
  }

  stopBackgroundChecks(): void {
    if (this.startupTimer !== null) {
      clearTimeout(this.startupTimer);
      this.startupTimer = null;
    }
    if (this.periodicTimer !== null) {
      clearInterval(this.periodicTimer);
      this.periodicTimer = null;
    }
  }

  async setChannel(channel: DesktopUpdateChannel): Promise<void> {
    const inferredChannel = inferDesktopUpdateChannel(app.getVersion());
    if (channel === this.channel && inferredChannel === channel) {
      this.publishStatus({ ...this.status });
      return;
    }
    if (this.checkActive || this.installing) {
      throw new Error(
        "Wait for the current desktop update before changing channels.",
      );
    }
    if (this.downloaded) {
      await this.discardPendingUpdate();
    }
    this.channelBeforeSwitch = this.channel;
    this.channel = channel;
    this.expectedVersion = "";
    this.channelSwitchActive = true;
    this.installWhenDownloaded = true;
    this.updateStatus({
      state: "idle",
      channel,
      currentVersion: app.getVersion(),
    });
    await this.checkForUpdates();
  }

  async checkForUpdates(): Promise<void> {
    if (this.downloaded) {
      this.publishStatus({ ...this.status });
      return;
    }
    if (this.checkActive) {
      return;
    }
    if (usesMicrosoftStoreUpdates(process.platform, process.windowsStore)) {
      const channel = this.failChannelSwitch();
      this.updateStatus({
        state: "unsupported",
        channel,
        currentVersion: app.getVersion(),
        message: "Updates are managed automatically by Microsoft Store.",
      });
      return;
    }
    if (process.platform === DesktopPlatform.Linux) {
      const channel = this.failChannelSwitch();
      this.updateStatus({
        state: "unsupported",
        channel,
        currentVersion: app.getVersion(),
        message:
          "Linux desktop updates are provided by the system package manager.",
      });
      return;
    }
    const updateURL = resolveUpdateURL(this.channel);
    const manifestURL = resolveManifestURL(this.channel);
    if (!app.isPackaged || !updateURL || !manifestURL) {
      const channel = this.failChannelSwitch();
      this.updateStatus({
        state: "unsupported",
        channel,
        currentVersion: app.getVersion(),
        message: app.isPackaged
          ? "Desktop update feed or downloads manifest is not configured."
          : "Updates are disabled in development.",
      });
      return;
    }
    this.checkActive = true;
    this.updateStatus({
      state: "checking",
      channel: this.channel,
      currentVersion: app.getVersion(),
    });
    try {
      const manifest = await fetchDownloadsManifest(manifestURL, this.channel);
      this.expectedVersion = manifest.latest;
      if (
        !shouldInstallDesktopVersion(
          app.getVersion(),
          this.expectedVersion,
          this.channelSwitchActive,
        )
      ) {
        this.checkActive = false;
        this.installWhenDownloaded = false;
        if (this.channelSwitchActive) {
          saveUpdateChannel(this.channel);
          this.channelBeforeSwitch = null;
        }
        this.channelSwitchActive = false;
        this.updateStatus({
          state: "not-available",
          channel: this.channel,
          currentVersion: app.getVersion(),
          availableVersion: this.expectedVersion,
        });
        return;
      }
      this.updateStatus({
        state: this.installWhenDownloaded ? "downloading" : "available",
        channel: this.channel,
        currentVersion: app.getVersion(),
        availableVersion: this.expectedVersion,
      });
      if (this.channelSwitchActive) {
        if (process.platform === DesktopPlatform.MacOS) {
          await this.checkMacChannelSwitch(updateURL);
          return;
        }
        if (process.platform === DesktopPlatform.Windows) {
          await this.downloadWindowsChannelSwitch(manifest);
          return;
        }
      }
      autoUpdater.setFeedURL(
        desktopUpdateFeedOptions(updateURL, process.platform),
      );
      autoUpdater.checkForUpdates();
    } catch (error) {
      this.checkActive = false;
      this.installWhenDownloaded = false;
      const channel = this.failChannelSwitch();
      await this.closeMacChannelFeed();
      this.updateStatus({
        state: "error",
        channel,
        currentVersion: app.getVersion(),
        message: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async installDownloadedUpdate(): Promise<void> {
    if (this.downloaded) {
      await this.installUpdateNow();
      return;
    }
    if (
      !this.expectedVersion ||
      !shouldInstallDesktopVersion(
        app.getVersion(),
        this.expectedVersion,
        this.channelSwitchActive,
      )
    ) {
      throw new Error("No desktop update is available.");
    }
    this.installWhenDownloaded = true;
    this.updateStatus({
      state: "downloading",
      channel: this.channel,
      currentVersion: app.getVersion(),
      availableVersion: this.expectedVersion,
    });
  }

  private bindEvents(): void {
    autoUpdater.on("checking-for-update", () => {
      this.updateStatus({
        state: this.installWhenDownloaded
          ? "downloading"
          : this.expectedVersion
            ? "available"
            : "checking",
        channel: this.channel,
        currentVersion: app.getVersion(),
        availableVersion: this.expectedVersion || undefined,
      });
    });
    autoUpdater.on("update-available", () => {
      this.updateStatus({
        state: this.installWhenDownloaded ? "downloading" : "available",
        channel: this.channel,
        currentVersion: app.getVersion(),
        availableVersion: this.expectedVersion || undefined,
      });
    });
    autoUpdater.on("update-not-available", () => {
      this.checkActive = false;
      this.installWhenDownloaded = false;
      const wasChannelSwitch = this.channelSwitchActive;
      const channel = this.failChannelSwitch();
      void this.closeMacChannelFeed();
      if (wasChannelSwitch) {
        this.updateStatus({
          state: "error",
          channel,
          currentVersion: app.getVersion(),
          availableVersion: this.expectedVersion || undefined,
          message: `The target channel did not offer version ${this.expectedVersion}.`,
        });
        return;
      }
      if (
        this.expectedVersion &&
        compareDesktopReleaseVersions(app.getVersion(), this.expectedVersion) <
          0
      ) {
        this.updateStatus({
          state: "error",
          channel: this.channel,
          currentVersion: app.getVersion(),
          availableVersion: this.expectedVersion,
          message: `The native desktop update feed did not offer manifest latest version ${this.expectedVersion}.`,
        });
        return;
      }
      this.updateStatus({
        state: "not-available",
        channel: this.channel,
        currentVersion: app.getVersion(),
        availableVersion: this.expectedVersion || undefined,
      });
    });
    autoUpdater.on(
      "update-downloaded",
      (_event, _releaseNotes, releaseName) => {
        this.checkActive = false;
        this.downloaded = true;
        void this.closeMacChannelFeed();
        this.updateStatus({
          state: "downloaded",
          channel: this.channel,
          currentVersion: app.getVersion(),
          availableVersion:
            this.expectedVersion ||
            (typeof releaseName === "string" ? releaseName : undefined),
        });
        if (this.installWhenDownloaded) {
          setTimeout(() => {
            void this.installUpdateNow().catch(() => undefined);
          }, 250);
        }
      },
    );
    autoUpdater.on("error", (error) => {
      this.checkActive = false;
      this.installWhenDownloaded = false;
      const channel = this.failChannelSwitch();
      void this.closeMacChannelFeed();
      this.updateStatus({
        state: "error",
        channel,
        currentVersion: app.getVersion(),
        message: error.message,
      });
    });
  }

  private updateStatus(status: DesktopUpdateStatus): void {
    this.status = status;
    logDesktopInfo("desktop-update-status", {
      state: status.state,
      channel: status.channel,
      currentVersion: status.currentVersion,
      availableVersion: status.availableVersion,
      message: status.message,
    });
    this.publishStatus({ ...status });
  }

  private async installUpdateNow(): Promise<void> {
    if (this.installing) {
      return;
    }
    if (!this.downloaded) {
      throw new Error("No desktop update has been downloaded.");
    }
    this.installing = true;
    const channelBeforeSwitch = this.channelBeforeSwitch;
    try {
      await this.beforeInstall();
      if (channelBeforeSwitch) {
        saveUpdateChannel(this.channel);
        this.channelBeforeSwitch = null;
        this.channelSwitchActive = false;
      }
      if (this.windowsChannelInstallerPath) {
        const updateExePath = path.resolve(
          path.dirname(process.execPath),
          "..",
          "Update.exe",
        );
        await launchWindowsChannelInstaller(
          this.windowsChannelInstallerPath,
          updateExePath,
          path.basename(process.execPath),
          process.pid,
        );
        app.quit();
        return;
      }
      autoUpdater.quitAndInstall();
    } catch (error) {
      this.installing = false;
      this.installWhenDownloaded = false;
      if (channelBeforeSwitch) {
        this.channel = channelBeforeSwitch;
        this.channelBeforeSwitch = null;
        this.channelSwitchActive = false;
        saveUpdateChannel(channelBeforeSwitch);
      }
      this.updateStatus({
        state: "error",
        channel: this.channel,
        currentVersion: app.getVersion(),
        availableVersion: this.expectedVersion || undefined,
        message: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private async checkMacChannelSwitch(updateURL: string): Promise<void> {
    let update;
    try {
      const requestURL = new URL(updateURL);
      requestURL.searchParams.set("switch", Date.now().toString());
      const response = await net.fetch(requestURL.toString(), {
        cache: "no-store",
      });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      update = parseMacChannelUpdate(
        await response.json(),
        this.expectedVersion,
      );
    } catch (error) {
      logDesktopInfo("desktop-channel-switch-mac-feed-missing", {
        channel: this.channel,
        expectedVersion: this.expectedVersion,
        message: error instanceof Error ? error.message : String(error),
      });
      throw missingSignedMacUpdatePackageError(
        this.channel,
        this.expectedVersion,
        error,
      );
    }
    await this.closeMacChannelFeed();
    this.macChannelFeed = await startMacChannelUpdateFeed(update);
    autoUpdater.setFeedURL({ url: this.macChannelFeed.url });
    autoUpdater.checkForUpdates();
  }

  private async downloadWindowsChannelSwitch(
    manifest: DesktopDownloadsManifest,
  ): Promise<void> {
    const artifact = desktopInstallerArtifact(
      manifest,
      process.platform,
      process.arch,
    );
    if (!artifact) {
      throw new Error(
        `The ${this.channel} channel does not provide a Windows installer for ${process.arch}.`,
      );
    }
    const installerDirectory = path.join(
      app.getPath("userData"),
      "desktop-updates",
    );
    const installerPath = path.join(
      installerDirectory,
      `CSGClaw-${this.channel}-${this.expectedVersion}-${process.arch}.exe`,
    );
    if (!(await isVerifiedArtifact(installerPath, artifact))) {
      await downloadVerifiedArtifact(net.fetch, artifact, installerPath);
    }
    this.checkActive = false;
    this.downloaded = true;
    this.windowsChannelInstallerPath = installerPath;
    this.updateStatus({
      state: "downloaded",
      channel: this.channel,
      currentVersion: app.getVersion(),
      availableVersion: this.expectedVersion,
    });
    if (this.installWhenDownloaded) {
      await this.installUpdateNow();
    }
  }

  private async discardPendingUpdate(): Promise<void> {
    this.downloaded = false;
    this.windowsChannelInstallerPath = "";
    this.installWhenDownloaded = false;
    this.expectedVersion = "";
    await this.closeMacChannelFeed();
  }

  private async closeMacChannelFeed(): Promise<void> {
    const feed = this.macChannelFeed;
    this.macChannelFeed = null;
    await feed?.close();
  }

  private failChannelSwitch(): DesktopUpdateChannel {
    if (this.channelBeforeSwitch) {
      this.channel = this.channelBeforeSwitch;
      this.channelBeforeSwitch = null;
    }
    this.channelSwitchActive = false;
    return this.channel;
  }
}

async function fetchDownloadsManifest(
  manifestURL: string,
  channel: DesktopUpdateChannel,
): Promise<DesktopDownloadsManifest> {
  const requestURL = new URL(manifestURL);
  requestURL.searchParams.set("check", Date.now().toString());
  const response = await net.fetch(requestURL.toString(), {
    cache: "no-store",
  });
  if (!response.ok) {
    throw new Error(
      `Desktop downloads manifest request failed: HTTP ${response.status}.`,
    );
  }
  const manifest = parseDesktopDownloadsManifest(
    await response.json(),
    channel,
  );
  compareDesktopReleaseVersions(manifest.latest, manifest.latest);
  return manifest;
}

function resolveUpdateURL(channel: DesktopUpdateChannel): string {
  return desktopUpdateFeedURL({
    channel,
    platform: process.platform,
    arch: process.arch,
    directURL: process.env.CSGCLAW_DESKTOP_UPDATE_URL || "",
    channelsBaseURL: readChannelsBaseURL(),
  });
}

function resolveManifestURL(channel: DesktopUpdateChannel): string {
  return desktopDownloadsManifestURL({
    channel,
    channelsBaseURL: readChannelsBaseURL(),
  });
}

function readChannelsBaseURL(): string {
  const configured = process.env.CSGCLAW_DESKTOP_UPDATE_CHANNELS_URL?.trim();
  if (configured) {
    return configured;
  }
  try {
    const source = JSON.parse(
      fs.readFileSync(
        path.join(process.resourcesPath, "desktop-update.json"),
        "utf8",
      ),
    ) as unknown;
    if (source && typeof source === "object" && !Array.isArray(source)) {
      const value = String(
        (source as Record<string, unknown>).channels_base_url || "",
      ).trim();
      if (value) {
        return value;
      }
    }
  } catch {
    // Fall back to the official public channel root.
  }
  return DEFAULT_UPDATE_CHANNELS_BASE_URL;
}

function updatePreferencesPath(): string {
  return path.join(app.getPath("userData"), UPDATE_PREFERENCES_FILE);
}

function loadUpdateChannel(): DesktopUpdateChannel {
  try {
    const source = JSON.parse(
      fs.readFileSync(updatePreferencesPath(), "utf8"),
    ) as unknown;
    if (source && typeof source === "object" && !Array.isArray(source)) {
      const channel = (source as Record<string, unknown>).channel;
      if (channel === "release" || channel === "beta") {
        return channel;
      }
    }
  } catch {
    // Missing or invalid preferences use the stable channel.
  }
  return "release";
}

function saveUpdateChannel(channel: DesktopUpdateChannel): void {
  const filePath = updatePreferencesPath();
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, `${JSON.stringify({ channel }, null, 2)}\n`, {
    mode: 0o600,
  });
}
