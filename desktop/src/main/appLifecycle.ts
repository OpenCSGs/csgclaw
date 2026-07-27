import { app, dialog, Menu, nativeImage, shell, Tray } from "electron";
import { DesktopIPC, type DesktopUpdateStatus } from "../shared/desktopBridge.types";
import { registerIPCHandlers } from "./ipcHandlers";
import { SidecarSupervisor } from "./sidecar/SidecarSupervisor";
import { DesktopUpdater } from "./updater";
import { WindowManager } from "./windowManager";

export class AppLifecycle {
  private cleanupIPC: (() => void) | null = null;
  private quitting = false;
  private recoveryActive = false;
  private shutdownComplete = false;
  private supervisor: SidecarSupervisor | null = null;
  private tray: Tray | null = null;
  private updater: DesktopUpdater | null = null;
  private windowManager: WindowManager | null = null;

  async start(): Promise<void> {
    this.supervisor = new SidecarSupervisor();
    this.supervisor.on("crashed", (error: Error) => {
      if (!this.quitting) {
        void this.recoverSidecar(error);
      }
    });

    this.windowManager = new WindowManager({
      shouldQuit: () => this.quitting,
      onLoadFailure: (error) => {
        if (!this.quitting) {
          void this.recoverRenderer(error);
        }
      },
    });
    this.updater = new DesktopUpdater(
      (status) => this.publishUpdateStatus(status),
      async () => {
        this.quitting = true;
        this.cleanup();
        await this.supervisor?.stop("install-update");
        this.shutdownComplete = true;
      },
    );
    this.cleanupIPC = registerIPCHandlers(
      () => this.windowManager?.window ?? null,
      () => this.supervisor?.connection.ready.base_url ?? "",
      this.supervisor,
      this.updater,
    );

    this.createApplicationMenu();
    this.createTray();
    try {
      const connection = await this.supervisor.startWithRetry();
      this.windowManager.setConnection(connection.ready.base_url, connection.sessionToken);
      await this.windowManager.open();
    } catch (error) {
      await this.recoverSidecar(asError(error));
    }
  }

  handleSecondInstance(): void {
    this.show();
  }

  show(): void {
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
    if (confirm && !(await this.confirmQuit())) {
      return;
    }
    this.quitting = true;
    this.cleanup();
    this.windowManager?.destroy();
    this.tray?.destroy();
    this.tray = null;
    await this.supervisor?.stop("app-quit");
    this.shutdownComplete = true;
    app.quit();
  }

  private async recoverSidecar(error: Error): Promise<void> {
    if (this.recoveryActive || this.quitting || !this.supervisor || !this.windowManager) {
      return;
    }
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
          await shell.openPath(app.getPath("logs"));
          continue;
        }
        if (result.response === 2) {
          await this.requestQuit(false);
          return;
        }
        try {
          const connection = await this.supervisor.startWithRetry();
          this.windowManager.setConnection(connection.ready.base_url, connection.sessionToken);
          await this.windowManager.open();
          return;
        } catch (retryError) {
          error = asError(retryError);
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
        await shell.openPath(app.getPath("logs"));
        await this.windowManager.open();
        return;
      }
      if (result.response === 2) {
        await this.requestQuit(false);
        return;
      }
      await this.windowManager.open();
    } catch (reloadError) {
      this.recoveryActive = false;
      await this.recoverSidecar(asError(reloadError));
      return;
    } finally {
      this.recoveryActive = false;
    }
  }

  private createTray(): void {
    const svg = [
      '<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 22 22">',
      '<path fill="none" stroke="black" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"',
      ' d="M7 5 4 8v7l3 3h8l3-3V8l-3-3-2 3H9L7 5Zm1 8h.01M14 13h.01"/>',
      "</svg>",
    ].join("");
    const icon = nativeImage.createFromDataURL(`data:image/svg+xml;base64,${Buffer.from(svg).toString("base64")}`);
    if (process.platform === "darwin") {
      icon.setTemplateImage(true);
    }
    this.tray = new Tray(icon);
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

  private createApplicationMenu(): void {
    const template: Electron.MenuItemConstructorOptions[] = [
      ...(process.platform === "darwin"
        ? [
            {
              label: app.name,
              submenu: [
                { role: "about" as const },
                { type: "separator" as const },
                { label: "Quit CSGClaw", accelerator: "Command+Q", click: () => void this.requestQuit(true) },
              ],
            },
          ]
        : []),
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
    this.cleanupIPC?.();
    this.cleanupIPC = null;
  }
}

function asError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}
