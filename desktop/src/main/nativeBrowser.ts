import {
  ipcMain,
  session,
  WebContentsView,
  type BrowserWindow,
  type Session,
} from "electron";
import { DesktopIPC } from "../shared/desktopBridge.types";
import type {
  DesktopBrowserRequest,
  DesktopBrowserState,
} from "../shared/desktopBrowser.types";
import {
  browserURL,
  clipBrowserBounds,
  parseBrowserRequest,
} from "./browserPolicy";
import { isTrustedMainFrame } from "./navigationPolicy";

type Page = {
  view: WebContentsView;
  requestedURL: string;
  visible: boolean;
  error?: string;
};

// Remote pages are top-level Chromium documents in an independent session. They
// have no preload, Node access, application authorization headers or desktop IPC.
export class NativeBrowserManager {
  private readonly pages = new Map<string, Page>();
  private readonly browserSession: Session;
  constructor(
    private readonly window: BrowserWindow,
    private readonly appOrigin: () => string,
  ) {
    this.browserSession = session.fromPartition("persist:csgclaw-browser");
    this.browserSession.setPermissionRequestHandler(
      (_contents, _permission, callback) => callback(false),
    );
    this.browserSession.setPermissionCheckHandler(() => false);
    this.browserSession.setDevicePermissionHandler(() => false);
    this.browserSession.setDisplayMediaRequestHandler((_request, callback) =>
      callback({}),
    );
    this.browserSession.on("will-download", this.handleDownload);
    this.browserSession.webRequest.onBeforeRequest((details, callback) => {
      // Prevent remote pages from loading the authenticated application, including
      // through redirects and subresources. No application headers are installed.
      let cancel = false;
      try {
        cancel = new URL(details.url).origin === appOrigin();
      } catch {
        cancel = true;
      }
      callback({ cancel });
    });
    ipcMain.handle(DesktopIPC.browserCommand, (event, input: unknown) => {
      if (
        event.sender !== window.webContents ||
        event.senderFrame !== window.webContents.mainFrame ||
        !isTrustedMainFrame(
          event.sender,
          event.senderFrame.url,
          this.appOrigin(),
        )
      )
        throw new Error("Desktop browser sender was rejected.");
      return this.command(parseBrowserRequest(input));
    });
    window.on("hide", () =>
      this.pages.forEach((page) => {
        page.view.setVisible(false);
        page.view.webContents.setAudioMuted(true);
        void page.view.webContents
          .executeJavaScript(
            'document.querySelectorAll("video,audio").forEach(media => media.pause())',
          )
          .catch(() => {});
      }),
    );
    window.on("show", () =>
      this.pages.forEach((page) => {
        page.view.setVisible(page.visible);
        page.view.webContents.setAudioMuted(!page.visible);
      }),
    );
    window.webContents.on(
      "did-start-navigation",
      (_event, _url, isInPlace, isMainFrame) => {
        if (isMainFrame && !isInPlace) this.closeAll();
      },
    );
    window.once("closed", () => this.dispose());
  }

  private readonly handleDownload = (
    event: Electron.Event,
    item: Electron.DownloadItem,
  ) => {
    if (!item.hasUserGesture()) {
      event.preventDefault();
      return;
    }
    item.setSaveDialogOptions({});
  };

  command(request: DesktopBrowserRequest): DesktopBrowserState | null {
    if (request.action === "close") {
      this.close(request.id);
      return null;
    }
    let page = this.pages.get(request.id);
    if (request.action === "navigate") {
      const url = browserURL(request.url, this.appOrigin());
      if (!page) {
        page = this.create(request.id);
        this.pages.set(request.id, page);
      }
      page.requestedURL = url;
      page.error = undefined;
      void page.view.webContents.loadURL(url).catch(() => {});
    } else if (request.action === "sync") {
      const url = browserURL(request.url, this.appOrigin());
      if (!page) {
        page = this.create(request.id);
        this.pages.set(request.id, page);
        page.requestedURL = url;
        void page.view.webContents.loadURL(url).catch(() => {});
      }
      const [width, height] = this.window.getContentSize();
      const zoom = this.window.webContents.getZoomFactor();
      const bounds = clipBrowserBounds(
        {
          x: request.bounds.x * zoom,
          y: request.bounds.y * zoom,
          width: request.bounds.width * zoom,
          height: request.bounds.height * zoom,
        },
        width ?? 0,
        height ?? 0,
      );
      const visible = request.visible && bounds.width > 0 && bounds.height > 0;
      if (page.visible && !visible) {
        void page.view.webContents
          .executeJavaScript(
            'document.querySelectorAll("video,audio").forEach(media => media.pause())',
          )
          .catch(() => {});
      }
      page.visible = visible;
      page.view.setBounds(bounds);
      page.view.setVisible(visible);
      page.view.webContents.setAudioMuted(!visible);
    } else if (page) {
      page.error = undefined;
      const contents = page.view.webContents;
      if (request.action === "reload") contents.reload();
      if (
        request.action === "back" &&
        contents.navigationHistory.getActiveIndex() > 0
      )
        contents.navigationHistory.goToIndex(
          contents.navigationHistory.getActiveIndex() - 1,
        );
      if (
        request.action === "forward" &&
        contents.navigationHistory.getActiveIndex() + 1 <
          contents.navigationHistory.length()
      )
        contents.navigationHistory.goToIndex(
          contents.navigationHistory.getActiveIndex() + 1,
        );
    }
    return page ? this.state(request.id, page) : null;
  }

  private create(id: string): Page {
    const view = new WebContentsView({
      webPreferences: {
        session: this.browserSession,
        contextIsolation: true,
        sandbox: true,
        nodeIntegration: false,
        nodeIntegrationInWorker: false,
        nodeIntegrationInSubFrames: false,
        webSecurity: true,
        allowRunningInsecureContent: false,
        webviewTag: false,
      },
    });
    view.setVisible(false);
    this.window.contentView.addChildView(view);
    const page: Page = { view, visible: false, requestedURL: "" };
    const contents = view.webContents;
    const publish = () => {
      if (this.pages.get(id) === page && !contents.isDestroyed())
        this.publish(this.state(id, page));
    };
    contents.on("will-frame-navigate", (event) => {
      if (!event.isMainFrame && !event.url.startsWith("file:")) return;
      try {
        browserURL(event.url, this.appOrigin());
      } catch {
        event.preventDefault();
        page.error = "navigation_blocked";
        publish();
      }
    });
    contents.on("will-redirect", (event, url, _inPlace, isMainFrame) => {
      if (!isMainFrame) return;
      try {
        browserURL(url, this.appOrigin());
      } catch {
        event.preventDefault();
        page.error = "navigation_blocked";
        publish();
      }
    });
    const navigated = () => {
      if (this.pages.get(id) !== page || contents.isDestroyed()) return;
      const url = contents.getURL();
      try {
        page.requestedURL = browserURL(url, this.appOrigin());
        page.error = undefined;
      } catch {
        return;
      }
      publish();
    };
    contents.on("did-navigate", navigated);
    contents.on("did-navigate-in-page", navigated);
    contents.on("did-start-loading", publish);
    contents.on("did-stop-loading", publish);
    contents.on(
      "did-fail-load",
      (_event, code, _description, _url, mainFrame) => {
        if (mainFrame && code !== -3) {
          page.error = `load_${code}`;
          publish();
        }
      },
    );
    contents.on("render-process-gone", () => {
      page.error = "renderer_gone";
      publish();
    });
    contents.setWindowOpenHandler(({ url }) => {
      if (!page.visible || this.pages.get(id) !== page)
        return { action: "deny" };
      try {
        this.publish({
          ...this.state(id, page),
          popupURL: browserURL(url, this.appOrigin()),
        });
      } catch {
        /* Privileged protocols stay blocked. */
      }
      return { action: "deny" };
    });
    contents.on("before-input-event", (event, input) => {
      if (input.type !== "keyDown") return;
      if (input.key === "Escape") {
        event.preventDefault();
        this.window.webContents.focus();
        this.publish({ ...this.state(id, page), shortcut: "escape" });
      } else if (
        (input.control || input.meta) &&
        input.key.toLowerCase() === "r"
      ) {
        event.preventDefault();
        contents.reload();
      }
    });
    return page;
  }

  private state(id: string, page: Page): DesktopBrowserState {
    const contents = page.view.webContents;
    return {
      id,
      url: page.requestedURL,
      loading: contents.isLoading(),
      error: page.error,
      canGoBack: contents.navigationHistory.getActiveIndex() > 0,
      canGoForward:
        contents.navigationHistory.getActiveIndex() + 1 <
        contents.navigationHistory.length(),
    };
  }
  private publish(state: DesktopBrowserState): void {
    if (!this.window.isDestroyed() && !this.window.webContents.isDestroyed())
      this.window.webContents.send(DesktopIPC.browserState, state);
  }
  private close(id: string): void {
    const page = this.pages.get(id);
    if (!page) return;
    this.pages.delete(id);
    if (!this.window.isDestroyed())
      this.window.contentView.removeChildView(page.view);
    if (!page.view.webContents.isDestroyed())
      page.view.webContents.close({ waitForBeforeUnload: false });
  }
  private closeAll(): void {
    for (const id of this.pages.keys()) this.close(id);
  }
  dispose(): void {
    this.browserSession.removeListener("will-download", this.handleDownload);
    ipcMain.removeHandler(DesktopIPC.browserCommand);
    this.closeAll();
  }
}
