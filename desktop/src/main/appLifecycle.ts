import path from "node:path";
import {
  app,
  dialog,
  Menu,
  nativeImage,
  nativeTheme,
  shell,
  Tray,
} from "electron";
import {
  DesktopIPC,
  type DesktopThemeSource,
  type DesktopUpdateStatus,
} from "../shared/desktopBridge.types";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import { shouldUseDarkDockIcon } from "../shared/desktopTheme";
import { DeferredRelaunch } from "./deferredRelaunch";
import { logDesktopError, logDesktopInfo } from "./desktopLogger";
import { registerIPCHandlers } from "./ipcHandlers";
import { desktopIconResourcePath, isMacOSDesktop, windowsAppIconPath } from "./platform";
import { SidecarSupervisor } from "./sidecar/SidecarSupervisor";
import { DesktopUpdater } from "./updater";
import { WindowManager } from "./windowManager";

export class AppLifecycle {
  private cleanupIPC: (() => void) | null = null;
  private cleanupDockThemeIcon: (() => void) | null = null;
  private desktopThemeSource: DesktopThemeSource = "system";
  private readonly deferredRelaunch = new DeferredRelaunch();
  private quitting = false;
  private recoveryActive = false;
  private rendererOrigin = "";
  private restartActive: Promise<void> | null = null;
  private shutdownComplete = false;
  private supervisor: SidecarSupervisor | null = null;
  private tray: Tray | null = null;
  private updater: DesktopUpdater | null = null;
  private windowManager: WindowManager | null = null;

  async start(): Promise<void> {
    logDesktopInfo("lifecycle-start");
    this.supervisor = new SidecarSupervisor();
    this.supervisor.on("state", (state: string) => {
      logDesktopInfo("sidecar-state", { state });
    });
    this.supervisor.on("crashed", (error: Error) => {
      logDesktopError("sidecar-crashed", error);
      if (!this.quitting) {
        void this.recoverSidecar(error);
      }
    });

    this.windowManager = new WindowManager({
      shouldQuit: () => this.quitting,
      onLoadFailure: (error) => {
        logDesktopError("renderer-load-failed", error);
        if (!this.quitting) {
          void this.recoverRenderer(error);
        }
      },
    });
    this.updater = new DesktopUpdater(
      (status) => this.publishUpdateStatus(status),
      async () => {
        logDesktopInfo("update-install-quit-requested");
        this.quitting = true;
        this.cleanup();
        await this.supervisor?.stop("install-update");
        this.shutdownComplete = true;
        logDesktopInfo("update-install-shutdown-complete");
      },
    );
    this.updater.startBackgroundChecks();
    this.cleanupIPC = registerIPCHandlers(
      () => this.windowManager?.window ?? null,
      () => this.rendererOrigin,
      this.supervisor,
      this.updater,
      () => this.restartSidecar(),
      (theme) => this.setThemeSource(theme),
    );

    this.configureDockThemeIcon();
    this.createApplicationMenu();
    this.createTray();
    logDesktopInfo("desktop-shell-ready");
    try {
      const connection = await this.supervisor.startWithRetry();
      logDesktopInfo("sidecar-ready", {
        distribution: connection.ready.distribution,
        sidecarPid: connection.ready.pid,
        version: connection.ready.version,
      });
      this.setConnection(connection.ready.base_url, connection.sessionToken);
      await this.windowManager.open();
      logDesktopInfo("window-opened");
    } catch (error) {
      logDesktopError("lifecycle-start-failed", error);
      await this.recoverSidecar(asError(error));
    }
  }

  handleSecondInstance(): void {
    this.show();
  }

  show(): void {
    if (this.quitting) {
      if (this.deferredRelaunch.request()) {
        logDesktopInfo("quit-relaunch-requested");
      }
      return;
    }
    this.windowManager?.showAndFocus();
  }

  handleBeforeQuit(event: Electron.Event): void {
    if (this.shutdownComplete) {
      return;
    }
    event.preventDefault();
    void this.requestQuit(true);
  }

  async requestQuit(confirm: boolean): Promise<void> {
    if (this.quitting) {
      return;
    }
    logDesktopInfo("quit-requested", { confirm });
    if (confirm && !(await this.confirmQuit())) {
      logDesktopInfo("quit-cancelled");
      return;
    }
    this.quitting = true;
    this.cleanup();
    this.windowManager?.destroy();
    this.tray?.destroy();
    this.tray = null;
    await this.supervisor?.stop("app-quit");
    if (this.deferredRelaunch.scheduleIfRequested(() => app.relaunch())) {
      logDesktopInfo("quit-relaunch-scheduled");
    }
    this.shutdownComplete = true;
    logDesktopInfo("shutdown-complete");
    app.quit();
  }

  private async recoverSidecar(error: Error): Promise<void> {
    if (this.recoveryActive || this.quitting || !this.supervisor || !this.windowManager) {
      return;
    }
    logDesktopError("sidecar-recovery-started", error);
    this.recoveryActive = true;
    this.windowManager.destroy();
    try {
      while (!this.quitting) {
        const result = await dialog.showMessageBox({
          type: "error",
          buttons: ["Retry", "Open Logs", "Quit"],
          defaultId: 0,
          cancelId: 2,
          noLink: true,
          title: "CSGClaw could not start",
          message: error.message || "The local CSGClaw service stopped unexpectedly.",
          detail: this.supervisor.failureSummary,
        });
        if (result.response === 1) {
          logDesktopInfo("sidecar-recovery-open-logs");
          await shell.openPath(app.getPath("logs"));
          continue;
        }
        if (result.response === 2) {
          logDesktopInfo("sidecar-recovery-quit");
          await this.requestQuit(false);
          return;
        }
        logDesktopInfo("sidecar-recovery-retry");
        try {
          const connection = await this.supervisor.startWithRetry();
          this.setConnection(connection.ready.base_url, connection.sessionToken);
          await this.windowManager.open();
          logDesktopInfo("sidecar-recovery-succeeded");
          return;
        } catch (retryError) {
          error = asError(retryError);
          logDesktopError("sidecar-recovery-failed", error);
        }
      }
    } finally {
      this.recoveryActive = false;
    }
  }

  private async recoverRenderer(error: Error): Promise<void> {
    if (this.recoveryActive || this.quitting || !this.windowManager) {
      return;
    }
    logDesktopError("renderer-recovery-started", error);
    this.recoveryActive = true;
    this.windowManager.destroy();
    try {
      const result = await dialog.showMessageBox({
        type: "error",
        buttons: ["Reload Window", "Open Logs", "Quit"],
        defaultId: 0,
        cancelId: 2,
        noLink: true,
        title: "CSGClaw window stopped",
        message: error.message,
        detail: "The local service is still running. Reloading only recreates the desktop window.",
      });
      if (result.response === 1) {
        logDesktopInfo("renderer-recovery-open-logs");
        await shell.openPath(app.getPath("logs"));
        await this.windowManager.open();
        return;
      }
      if (result.response === 2) {
        logDesktopInfo("renderer-recovery-quit");
        await this.requestQuit(false);
        return;
      }
      await this.windowManager.open();
      logDesktopInfo("renderer-recovery-succeeded");
    } catch (reloadError) {
      logDesktopError("renderer-recovery-failed", reloadError);
      this.recoveryActive = false;
      await this.recoverSidecar(asError(reloadError));
      return;
    } finally {
      this.recoveryActive = false;
    }
  }

  private async restartSidecar(): Promise<void> {
    if (this.quitting) {
      throw new Error("CSGClaw is quitting.");
    }
    if (!this.supervisor || !this.windowManager) {
      throw new Error("Desktop backend is unavailable.");
    }
    if (this.restartActive) {
      return this.restartActive;
    }

    const task = (async () => {
      try {
        logDesktopInfo("sidecar-restart-started");
        const connection = await this.supervisor!.restart("settings-change");
        this.setConnection(connection.ready.base_url, connection.sessionToken);
        logDesktopInfo("sidecar-restart-succeeded");
      } catch (error) {
        logDesktopError("sidecar-restart-failed", error);
        void this.recoverSidecar(asError(error));
        throw error;
      }
    })();
    this.restartActive = task;
    try {
      await task;
    } finally {
      if (this.restartActive === task) {
        this.restartActive = null;
      }
    }
  }

  private setConnection(baseURL: string, sessionToken: string): void {
    this.rendererOrigin = new URL(baseURL).origin;
    this.windowManager?.setConnection(baseURL, sessionToken);
  }

  private createTray(): void {
    this.tray = new Tray(this.loadTrayIcon());
    this.tray.setToolTip("CSGClaw");
    this.tray.setContextMenu(
      Menu.buildFromTemplate([
        { label: "Open CSGClaw", click: () => this.show() },
        { type: "separator" },
        { label: "Quit", click: () => void this.requestQuit(true) },
      ]),
    );
    this.tray.on("double-click", () => this.show());
  }

  private loadTrayIcon(): Electron.NativeImage {
    switch (process.platform) {
      case DesktopPlatform.Windows:
        return this.loadWindowsIcon();
      case DesktopPlatform.MacOS:
        return this.loadMacOSTrayIcon();
      default:
        return this.createTemplateTrayIcon();
    }
  }

  private configureDockThemeIcon(): void {
    if (!isMacOSDesktop || !app.dock) {
      return;
    }
    nativeTheme.on("updated", this.handleNativeThemeUpdated);
    this.cleanupDockThemeIcon = () =>
      nativeTheme.removeListener("updated", this.handleNativeThemeUpdated);
    this.updateDockThemeIcon();
  }

  private readonly handleNativeThemeUpdated = (): void => {
    this.updateDockThemeIcon();
  };

  private updateDockThemeIcon(): void {
    if (!isMacOSDesktop || !app.dock) {
      return;
    }
    const useDarkColors = shouldUseDarkDockIcon(
      this.desktopThemeSource,
      nativeTheme.shouldUseDarkColors,
    );
    const iconDirectory = app.isPackaged
      ? process.resourcesPath
      : path.resolve(__dirname, "..", "..", "resources", "icons");
    const iconName = useDarkColors
      ? "csgclaw-dock-dark.png"
      : "csgclaw-dock-light.png";
    const icon = nativeImage.createFromPath(path.join(iconDirectory, iconName));
    if (!icon.isEmpty()) {
      app.dock.setIcon(icon);
    }
  }

  private setThemeSource(theme: DesktopThemeSource): void {
    this.desktopThemeSource = theme;
    nativeTheme.themeSource = theme;
    this.updateDockThemeIcon();
  }

  private loadMacOSTrayIcon(): Electron.NativeImage {
    return nativeImage
      .createFromPath(desktopIconResourcePath("csgclaw-dock-light.png"))
      .resize({ width: 16, height: 16 });
  }

  private createTemplateTrayIcon(): Electron.NativeImage {
    const svg = [
      '<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 22 22">',
      '<path fill="none" stroke="black" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"',
      ' d="M7 5 4 8v7l3 3h8l3-3V8l-3-3-2 3H9L7 5Zm1 8h.01M14 13h.01"/>',
      "</svg>",
    ].join("");
    return nativeImage.createFromDataURL(
      `data:image/svg+xml;base64,${Buffer.from(svg).toString("base64")}`,
    );
  }

  private loadWindowsIcon(): Electron.NativeImage {
    const markIconName = nativeTheme.shouldUseDarkColors
      ? "csgclaw-mark-dark.svg"
      : "csgclaw-mark-light.svg";
    const markIcon = nativeImage.createFromPath(desktopIconResourcePath(markIconName));
    if (!markIcon.isEmpty()) {
      return markIcon.resize({ width: 22, height: 22 });
    }
    const icon = nativeImage.createFromPath(windowsAppIconPath());
    return icon.isEmpty() ? this.createTemplateTrayIcon() : icon;
  }

  private createApplicationMenu(): void {
    if (!isMacOSDesktop) {
      Menu.setApplicationMenu(null);
      return;
    }
    const template: Electron.MenuItemConstructorOptions[] = [
      {
        label: app.name,
        submenu: [
          { role: "about" },
          { type: "separator" },
          { label: "Quit CSGClaw", accelerator: "Command+Q", click: () => void this.requestQuit(true) },
        ],
      },
      {
        label: "Edit",
        submenu: [
          { role: "undo" },
          { role: "redo" },
          { type: "separator" },
          { role: "cut" },
          { role: "copy" },
          { role: "paste" },
          { role: "selectAll" },
        ],
      },
      {
        label: "Window",
        submenu: [
          { label: "Open CSGClaw", accelerator: "CmdOrCtrl+Shift+O", click: () => this.show() },
          { role: "minimize" },
          { role: "close" },
        ],
      },
    ];
    Menu.setApplicationMenu(Menu.buildFromTemplate(template));
  }

  private async confirmQuit(): Promise<boolean> {
    const options: Electron.MessageBoxOptions = {
      type: "warning",
      buttons: ["Quit CSGClaw", "Keep Running"],
      defaultId: 1,
      cancelId: 1,
      noLink: true,
      title: "Quit CSGClaw?",
      message: "Quitting stops the local CSGClaw service and any running agents.",
    };
    const window = this.windowManager?.window;
    const result = window ? await dialog.showMessageBox(window, options) : await dialog.showMessageBox(options);
    return result.response === 0;
  }

  private publishUpdateStatus(status: DesktopUpdateStatus): void {
    const window = this.windowManager?.window;
    if (window && !window.isDestroyed()) {
      window.webContents.send(DesktopIPC.updateStatus, status);
    }
  }

  private cleanup(): void {
    this.updater?.stopBackgroundChecks();
    this.cleanupDockThemeIcon?.();
    this.cleanupDockThemeIcon = null;
    this.cleanupIPC?.();
    this.cleanupIPC = null;
  }
}

function asError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}
