import { shell, type BrowserWindow, type WebContents } from "electron";

export function installNavigationPolicy(window: BrowserWindow, allowedOrigin: string): void {
  const webContents = window.webContents;
  const handleNavigation = (event: Electron.Event, targetURL: string, _isInPlace?: boolean, isMainFrame?: boolean) => {
    if (isMainFrame === false) return;
    if (isAllowedInternalURL(targetURL, allowedOrigin)) {
      return;
    }
    event.preventDefault();
    if (isSafeWebURL(targetURL)) {
      void shell.openExternal(targetURL, { activate: true });
    }
  };

  webContents.on("will-navigate", handleNavigation);
  webContents.on("will-redirect", handleNavigation);
  webContents.on("will-attach-webview", (event) => event.preventDefault());
  webContents.setWindowOpenHandler(({ url }) => {
    if (isSafeWebURL(url)) {
      void shell.openExternal(url, { activate: true });
    }
    return { action: "deny" };
  });
}

export function isTrustedMainFrame(contents: WebContents, senderFrameURL: string, allowedOrigin: string): boolean {
  if (contents.isDestroyed()) {
    return false;
  }
  try {
    return new URL(senderFrameURL).origin === allowedOrigin;
  } catch {
    return false;
  }
}

export function isAllowedInternalURL(rawURL: string, allowedOrigin: string): boolean {
  try {
    const parsed = new URL(rawURL);
    return parsed.origin === allowedOrigin && parsed.protocol === "http:";
  } catch {
    return false;
  }
}

export function isSafeHTTPSURL(rawURL: string): boolean {
  try {
    const parsed = new URL(rawURL);
    return parsed.protocol === "https:" && !parsed.username && !parsed.password;
  } catch {
    return false;
  }
}

export function isSafeWebURL(rawURL: string): boolean {
  try {
    const parsed = new URL(rawURL);
    return ["https:", "http:"].includes(parsed.protocol) && !parsed.username && !parsed.password;
  } catch {
    return false;
  }
}
