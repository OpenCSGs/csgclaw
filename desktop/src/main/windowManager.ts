import path from "node:path";
import { BrowserWindow, session, type Session } from "electron";
import { installNavigationPolicy } from "./navigationPolicy";
import { isWindowsDesktop, windowsAppIconPath } from "./platform";
import { installPermissionPolicy } from "./permissionPolicy";

export type WindowManagerOptions = {
  onLoadFailure: (error: Error) => void;
  shouldQuit: () => boolean;
};

export class WindowManager {
  private allowedOrigin = "";
  private authToken = "";
  private mainWindow: BrowserWindow | null = null;
  private readonly desktopSession: Session;

  constructor(private readonly options: WindowManagerOptions) {
    this.desktopSession = session.fromPartition("persist:csgclaw-desktop", { cache: true });
    this.installRequestAuthentication();
  }

  setConnection(baseURL: string, sessionToken: string): void {
    this.allowedOrigin = new URL(baseURL).origin;
    this.authToken = sessionToken;
    installPermissionPolicy(this.desktopSession, this.allowedOrigin);
  }

  async open(): Promise<BrowserWindow> {
    if (this.mainWindow && !this.mainWindow.isDestroyed()) {
      this.showAndFocus();
      return this.mainWindow;
    }
    if (!this.allowedOrigin || !this.authToken) {
      throw new Error("Desktop backend connection is not configured.");
    }

    const window = new BrowserWindow({
      width: 1440,
      height: 920,
      minWidth: 980,
      minHeight: 680,
      show: false,
      title: "CSGClaw",
      backgroundColor: "#0d1017",
      ...(isWindowsDesktop
        ? {
            icon: windowsAppIconPath(),
          }
        : {}),
      webPreferences: {
        preload: path.join(__dirname, "..", "preload", "index.js"),
        partition: "persist:csgclaw-desktop",
        contextIsolation: true,
        sandbox: true,
        nodeIntegration: false,
        nodeIntegrationInWorker: false,
        nodeIntegrationInSubFrames: false,
        webSecurity: true,
        allowRunningInsecureContent: false,
        experimentalFeatures: false,
        webviewTag: false,
        devTools: process.env.CSGCLAW_DESKTOP_DEVTOOLS === "1",
      },
    });
    this.mainWindow = window;
    installNavigationPolicy(window, this.allowedOrigin);

    window.once("ready-to-show", () => {
      if (!window.isDestroyed()) {
        window.show();
      }
    });
    window.on("close", (event) => {
      if (!this.options.shouldQuit()) {
        event.preventDefault();
        window.hide();
      }
    });
    window.on("closed", () => {
      if (this.mainWindow === window) {
        this.mainWindow = null;
      }
    });
    window.webContents.on("render-process-gone", (_event, details) => {
      if (!this.options.shouldQuit()) {
        this.options.onLoadFailure(new Error(`Renderer process exited: ${details.reason}`));
      }
    });
    window.webContents.on("did-fail-load", (_event, errorCode, errorDescription, _url, isMainFrame) => {
      if (isMainFrame && errorCode !== -3 && !this.options.shouldQuit()) {
        this.options.onLoadFailure(new Error(`Renderer failed to load (${errorCode}): ${errorDescription}`));
      }
    });

    await window.loadURL(`${this.allowedOrigin}/`);
    return window;
  }

  showAndFocus(): void {
    const window = this.mainWindow;
    if (!window || window.isDestroyed()) {
      void this.open().catch(this.options.onLoadFailure);
      return;
    }
    if (window.isMinimized()) {
      window.restore();
    }
    window.show();
    window.focus();
  }

  destroy(): void {
    if (this.mainWindow && !this.mainWindow.isDestroyed()) {
      this.mainWindow.destroy();
    }
    this.mainWindow = null;
  }

  get window(): BrowserWindow | null {
    return this.mainWindow && !this.mainWindow.isDestroyed() ? this.mainWindow : null;
  }

  private installRequestAuthentication(): void {
    this.desktopSession.webRequest.onBeforeSendHeaders((details, callback) => {
      const requestHeaders = { ...details.requestHeaders };
      try {
        if (this.allowedOrigin && this.authToken && new URL(details.url).origin === this.allowedOrigin) {
          for (const name of Object.keys(requestHeaders)) {
            if (name.toLowerCase() === "authorization") {
              delete requestHeaders[name];
            }
          }
          requestHeaders.Authorization = `Bearer ${this.authToken}`;
        }
      } catch {
        // Invalid URLs are handled by Chromium and the navigation policy.
      }
      callback({ requestHeaders });
    });
  }
}
