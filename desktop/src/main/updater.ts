import fs from "node:fs";
import path from "node:path";
import { app, autoUpdater, net } from "electron";
import type {
  DesktopUpdateChannel,
  DesktopUpdateStatus,
} from "../shared/desktopBridge.types";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import {
  compareDesktopReleaseVersions,
  inferDesktopUpdateChannel,
} from "../shared/releaseVersion";
import { WINDOWS_UPDATE_HELPER_RESOURCE_NAME } from "../shared/windowsUpdateCoordinator";
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
  isSquirrelUpdateLockError,
  shouldInstallDesktopVersion,
  squirrelFirstRunUpdateDelay,
  usesMicrosoftStoreUpdates,
} from "./updatePolicy";

const STARTUP_CHECK_DELAY_MS = 5_000;
const PERIODIC_CHECK_INTERVAL_MS = 60 * 60 * 1000;
const SQUIRREL_LOCK_RETRY_INTERVAL_MS = 30_000;
const SQUIRREL_LOCK_RETRY_WINDOW_MS = 15 * 60 * 1000;

export class DesktopUpdater {
  private readonly channel: DesktopUpdateChannel;
  private targetChannel: DesktopUpdateChannel | null = null;
  private checkActive = false;
  private downloaded = false;
  private expectedVersion = "";
  private installWhenDownloaded = false;
  private installing = false;
  private channelSwitchActive = false;
  private macChannelFeed: LocalUpdateFeed | null = null;
  private windowsChannelInstallerPath = "";
  private readonly updateChecksReadyAt: number;
  private updateChecksReady: Promise<void> | null = null;
  private readonly squirrelLockRetryUntil: number;
  private squirrelLockRetryTimer: ReturnType<typeof setTimeout> | null = null;
  private periodicTimer: ReturnType<typeof setInterval> | null = null;
  private startupTimer: ReturnType<typeof setTimeout> | null = null;
  private status: DesktopUpdateStatus;

  constructor(
    private readonly publishStatus: (status: DesktopUpdateStatus) => void,
    private readonly beforeInstall: () => Promise<void>,
  ) {
    this.channel = inferDesktopUpdateChannel(app.getVersion());
    const startedAt = Date.now();
    const firstRunDelay = squirrelFirstRunUpdateDelay(
      process.platform,
      process.argv,
    );
    this.updateChecksReadyAt = startedAt + firstRunDelay;
    this.squirrelLockRetryUntil = firstRunDelay
      ? startedAt + SQUIRREL_LOCK_RETRY_WINDOW_MS
      : 0;
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
    if (this.squirrelLockRetryTimer !== null) {
      clearTimeout(this.squirrelLockRetryTimer);
      this.squirrelLockRetryTimer = null;
    }
  }

  async setChannel(channel: DesktopUpdateChannel): Promise<void> {
    if (channel === this.channel) {
      this.targetChannel = null;
      this.channelSwitchActive = false;
      this.installWhenDownloaded = false;
      this.updateStatus({
        state: "idle",
        channel: this.channel,
        currentVersion: app.getVersion(),
      });
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
    this.targetChannel = channel;
    this.expectedVersion = "";
    this.channelSwitchActive = true;
    this.installWhenDownloaded = true;
    this.updateStatus({
      state: "idle",
      channel: this.channel,
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
    if (this.squirrelLockRetryTimer !== null) {
      this.publishStatus({ ...this.status });
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
    if (!app.isPackaged) {
      const channel = this.failChannelSwitch();
      this.updateStatus({
        state: "unsupported",
        channel,
        currentVersion: app.getVersion(),
        message: "Updates are disabled in development.",
      });
      return;
    }
    await this.waitForUpdateChecksReady();
    if (this.downloaded) {
      this.publishStatus({ ...this.status });
      return;
    }
    if (this.checkActive) {
      return;
    }
    const updateChannel = this.effectiveUpdateChannel();
    const updateURL = resolveUpdateURL(updateChannel);
    const manifestURL = resolveManifestURL(updateChannel);
    if (!updateURL || !manifestURL) {
      const channel = this.failChannelSwitch();
      this.updateStatus({
        state: "unsupported",
        channel,
        currentVersion: app.getVersion(),
        message: "Desktop update feed or downloads manifest is not configured.",
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
      const manifest = await fetchDownloadsManifest(manifestURL, updateChannel);
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
        this.targetChannel = null;
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
      const message = error instanceof Error ? error.message : String(error);
      if (this.scheduleSquirrelLockRetry(message)) {
        this.updateStatus({
          state: "checking",
          channel: this.channel,
          currentVersion: app.getVersion(),
          availableVersion: this.expectedVersion || undefined,
        });
        return;
      }
      this.installWhenDownloaded = false;
      const channel = this.failChannelSwitch();
      await this.closeMacChannelFeed();
      this.updateStatus({
        state: "error",
        channel,
        currentVersion: app.getVersion(),
        message,
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
      if (this.scheduleSquirrelLockRetry(error.message)) {
        this.updateStatus({
          state: "checking",
          channel: this.channel,
          currentVersion: app.getVersion(),
          availableVersion: this.expectedVersion || undefined,
        });
        return;
      }
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
    const wasChannelSwitch = this.channelSwitchActive;
    try {
      await this.beforeInstall();
      if (
        process.platform === DesktopPlatform.Windows &&
        this.windowsChannelInstallerPath
      ) {
        const rootExecutablePath = path.resolve(
          path.dirname(process.execPath),
          "..",
          path.basename(process.execPath),
        );
        await launchWindowsChannelInstaller({
          helperResourcePath: path.join(
            process.resourcesPath,
            WINDOWS_UPDATE_HELPER_RESOURCE_NAME,
          ),
          installerPath: this.windowsChannelInstallerPath,
          logPath: path.join(app.getPath("logs"), "channel-installer.log"),
          parentProcessId: process.pid,
          rootExecutablePath,
        });
        logDesktopInfo("desktop-channel-switch-windows-coordinator-ready", {
          availableVersion: this.expectedVersion,
          channel: this.effectiveUpdateChannel(),
        });
        app.quit();
        return;
      }
      autoUpdater.quitAndInstall();
    } catch (error) {
      this.installing = false;
      this.installWhenDownloaded = false;
      if (wasChannelSwitch) {
        this.targetChannel = null;
        this.channelSwitchActive = false;
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
    const targetChannel = this.effectiveUpdateChannel();
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
        channel: targetChannel,
        expectedVersion: this.expectedVersion,
        message: error instanceof Error ? error.message : String(error),
      });
      throw missingSignedMacUpdatePackageError(
        targetChannel,
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
    const targetChannel = this.effectiveUpdateChannel();
    const artifact = desktopInstallerArtifact(
      manifest,
      process.platform,
      process.arch,
    );
    if (!artifact) {
      throw new Error(
        `The ${targetChannel} channel does not provide a Windows installer for ${process.arch}.`,
      );
    }
    const installerDirectory = path.join(
      app.getPath("userData"),
      "desktop-updates",
    );
    const installerPath = path.join(
      installerDirectory,
      `CSGClaw-${targetChannel}-${this.expectedVersion}-${process.arch}.exe`,
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
    this.targetChannel = null;
    this.channelSwitchActive = false;
    return this.channel;
  }

  private effectiveUpdateChannel(): DesktopUpdateChannel {
    return this.targetChannel ?? this.channel;
  }

  private async waitForUpdateChecksReady(): Promise<void> {
    const delayMs = this.updateChecksReadyAt - Date.now();
    if (delayMs <= 0) {
      return;
    }
    if (this.updateChecksReady === null) {
      logDesktopInfo("desktop-update-check-delayed", {
        delayMs,
        reason: "squirrel-firstrun",
      });
      this.updateChecksReady = new Promise<void>((resolve) => {
        const timer = setTimeout(resolve, delayMs);
        timer.unref();
      });
    }
    await this.updateChecksReady;
  }

  private scheduleSquirrelLockRetry(message: string): boolean {
    if (
      !isSquirrelUpdateLockError(message) ||
      this.squirrelLockRetryUntil <= Date.now()
    ) {
      return false;
    }
    if (this.squirrelLockRetryTimer !== null) {
      return true;
    }
    const delayMs = Math.min(
      SQUIRREL_LOCK_RETRY_INTERVAL_MS,
      this.squirrelLockRetryUntil - Date.now(),
    );
    logDesktopInfo("desktop-update-squirrel-lock-retry-scheduled", {
      delayMs,
      retryUntil: new Date(this.squirrelLockRetryUntil).toISOString(),
    });
    this.squirrelLockRetryTimer = setTimeout(() => {
      this.squirrelLockRetryTimer = null;
      void this.checkForUpdates().catch(() => undefined);
    }, delayMs);
    this.squirrelLockRetryTimer.unref();
    return true;
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
