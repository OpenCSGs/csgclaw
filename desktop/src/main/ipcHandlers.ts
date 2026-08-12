import {
  app,
  dialog,
  ipcMain,
  shell,
  type BrowserWindow,
  type IpcMainInvokeEvent,
} from "electron";
import {
  DesktopIPC,
  type DesktopOAuthInput,
  type OAuthPurpose,
  type DesktopThemeSource,
  type DesktopUpdateChannel,
} from "../shared/desktopBridge.types";
import { parseDesktopThemeSource } from "../shared/desktopTheme";
import { isSafeHTTPSURL, isTrustedMainFrame } from "./navigationPolicy";
import type { SidecarSupervisor } from "./sidecar/SidecarSupervisor";
import type { DesktopUpdater } from "./updater";

const confirmedCustomOAuthHosts = new Set<string>();

export function registerIPCHandlers(
  getWindow: () => BrowserWindow | null,
  getAllowedOrigin: () => string,
  supervisor: SidecarSupervisor,
  updater: DesktopUpdater,
  restartSidecar: () => Promise<void>,
  setThemeSource: (theme: DesktopThemeSource) => void,
): () => void {
  const assertSender = (event: IpcMainInvokeEvent) => {
    const window = getWindow();
    const origin = getAllowedOrigin();
    if (
      !window ||
      event.sender !== window.webContents ||
      event.senderFrame !== event.sender.mainFrame ||
      !isTrustedMainFrame(event.sender, event.senderFrame.url, origin)
    ) {
      throw new Error("Desktop IPC sender was rejected.");
    }
  };

  ipcMain.handle(DesktopIPC.getRuntimeInfo, (event) => {
    assertSender(event);
    return {
      platform: process.platform,
      arch: process.arch,
      appVersion: app.getVersion(),
      backendVersion: supervisor.connection.ready.version,
    };
  });
  ipcMain.handle(DesktopIPC.openOAuth, async (event, input: unknown) => {
    assertSender(event);
    const parsed = parseOAuthInput(input);
    const window = getWindow();
    if (!window) {
      throw new Error("Desktop window is unavailable.");
    }
    if (!(await authorizeOAuthHost(window, parsed.purpose, parsed.url))) {
      return { opened: false };
    }
    await shell.openExternal(parsed.url, { activate: true });
    return { opened: true };
  });
  ipcMain.handle(DesktopIPC.checkForUpdates, async (event) => {
    assertSender(event);
    await updater.checkForUpdates();
    return updater.currentStatus();
  });
  ipcMain.handle(DesktopIPC.installDownloadedUpdate, async (event) => {
    assertSender(event);
    await updater.installDownloadedUpdate();
  });
  ipcMain.handle(DesktopIPC.restartSidecar, async (event) => {
    assertSender(event);
    await restartSidecar();
  });
  ipcMain.handle(DesktopIPC.setThemeSource, (event, input: unknown) => {
    assertSender(event);
    setThemeSource(parseDesktopThemeSource(input));
  });
  ipcMain.handle(DesktopIPC.setUpdateChannel, async (event, input: unknown) => {
    assertSender(event);
    await updater.setChannel(parseUpdateChannel(input));
    return updater.currentStatus();
  });

  return () => {
    ipcMain.removeHandler(DesktopIPC.getRuntimeInfo);
    ipcMain.removeHandler(DesktopIPC.openOAuth);
    ipcMain.removeHandler(DesktopIPC.checkForUpdates);
    ipcMain.removeHandler(DesktopIPC.installDownloadedUpdate);
    ipcMain.removeHandler(DesktopIPC.restartSidecar);
    ipcMain.removeHandler(DesktopIPC.setThemeSource);
    ipcMain.removeHandler(DesktopIPC.setUpdateChannel);
  };
}

function parseUpdateChannel(input: unknown): DesktopUpdateChannel {
  if (input === "release" || input === "beta") {
    return input;
  }
  throw new Error("Desktop update channel must be release or beta.");
}

function parseOAuthInput(input: unknown): DesktopOAuthInput {
  if (!input || typeof input !== "object" || Array.isArray(input)) {
    throw new Error("OAuth request is invalid.");
  }
  const source = input as Record<string, unknown>;
  if (
    source.purpose !== "opencsg-auth" &&
    source.purpose !== "github-connector"
  ) {
    throw new Error("OAuth purpose is invalid.");
  }
  if (typeof source.url !== "string" || !isSafeHTTPSURL(source.url)) {
    throw new Error(
      "OAuth URL must be an HTTPS URL without embedded credentials.",
    );
  }
  return { purpose: source.purpose, url: source.url };
}

async function authorizeOAuthHost(
  window: BrowserWindow,
  purpose: OAuthPurpose,
  rawURL: string,
): Promise<boolean> {
  const hostname = new URL(rawURL).hostname.toLowerCase();
  if (purpose === "github-connector") {
    if (hostname !== "github.com") {
      throw new Error("GitHub OAuth must use github.com.");
    }
    return true;
  }
  if (isKnownOpenCSGHost(hostname) || confirmedCustomOAuthHosts.has(hostname)) {
    return true;
  }
  const result = await dialog.showMessageBox(window, {
    type: "question",
    buttons: ["Open in Browser", "Cancel"],
    defaultId: 1,
    cancelId: 1,
    noLink: true,
    title: "Open authentication site?",
    message: `CSGClaw wants to open ${hostname} in your browser.`,
    detail: "Only continue if this is the OpenCSG environment you selected.",
  });
  if (result.response !== 0) {
    return false;
  }
  confirmedCustomOAuthHosts.add(hostname);
  return true;
}

function isKnownOpenCSGHost(hostname: string): boolean {
  return ["opencsg.com", "opencsg.cn", "csghub.com"].some(
    (domain) => hostname === domain || hostname.endsWith(`.${domain}`),
  );
}
