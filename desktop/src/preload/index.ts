import type {
  DesktopBrowserRequest,
  DesktopBrowserState,
} from "../shared/desktopBrowser.types";
import { contextBridge, ipcRenderer } from "electron";
import type {
  DesktopBridge,
  DesktopTurnNotification,
  DesktopWindowState,
  DesktopOAuthInput,
  DesktopThemeSource,
  DesktopUpdateChannel,
  DesktopUpdateStatus,
} from "../shared/desktopBridge.types";

// Sandboxed preload scripts cannot require local CommonJS modules at runtime.
// Keep these channels inlined so the compiled preload remains a single file.
const DesktopIPC = {
  getWindowState: "csgclaw:desktop:get-window-state",
  windowState: "csgclaw:desktop:window-state",
  browserCommand: "csgclaw:desktop:browser-command",
  browserState: "csgclaw:desktop:browser-state",
  checkForUpdates: "csgclaw:desktop:check-for-updates",
  getRuntimeInfo: "csgclaw:desktop:get-runtime-info",
  installDownloadedUpdate: "csgclaw:desktop:install-downloaded-update",
  openOAuth: "csgclaw:desktop:open-oauth",
  restartSidecar: "csgclaw:desktop:restart-sidecar",
  showWindow: "csgclaw:desktop:show-window",
  showTurnNotification: "csgclaw:desktop:show-turn-notification",
  turnNotificationClick: "csgclaw:desktop:turn-notification-click",
  setThemeSource: "csgclaw:desktop:set-theme-source",
  setUpdateChannel: "csgclaw:desktop:set-update-channel",
  updateStatus: "csgclaw:desktop:update-status",
} as const;

const bridge: DesktopBridge = Object.freeze({
  getWindowState: () => ipcRenderer.invoke(DesktopIPC.getWindowState),
  onWindowState: (listener: (state: DesktopWindowState) => void) => {
    const handler = (
      _event: Electron.IpcRendererEvent,
      state: DesktopWindowState,
    ) => listener(state);
    ipcRenderer.on(DesktopIPC.windowState, handler);
    return () => ipcRenderer.removeListener(DesktopIPC.windowState, handler);
  },
  browserCommand: (input: DesktopBrowserRequest) =>
    ipcRenderer.invoke(DesktopIPC.browserCommand, input),
  onBrowserState: (listener: (state: DesktopBrowserState) => void) => {
    const handler = (
      _event: Electron.IpcRendererEvent,
      state: DesktopBrowserState,
    ) => listener(state);
    ipcRenderer.on(DesktopIPC.browserState, handler);
    return () => ipcRenderer.removeListener(DesktopIPC.browserState, handler);
  },
  getRuntimeInfo: () => ipcRenderer.invoke(DesktopIPC.getRuntimeInfo),
  openOAuth: (input: DesktopOAuthInput) =>
    ipcRenderer.invoke(DesktopIPC.openOAuth, input),
  checkForUpdates: () => ipcRenderer.invoke(DesktopIPC.checkForUpdates),
  installDownloadedUpdate: () =>
    ipcRenderer.invoke(DesktopIPC.installDownloadedUpdate),
  restartSidecar: () => ipcRenderer.invoke(DesktopIPC.restartSidecar),
  showWindow: () => ipcRenderer.invoke(DesktopIPC.showWindow),
  showTurnNotification: (input: DesktopTurnNotification) =>
    ipcRenderer.invoke(DesktopIPC.showTurnNotification, input),
  onTurnNotificationClick: (listener: (roomID: string) => void) => {
    const handler = (_event: Electron.IpcRendererEvent, roomID: string) =>
      listener(roomID);
    ipcRenderer.on(DesktopIPC.turnNotificationClick, handler);
    return () =>
      ipcRenderer.removeListener(DesktopIPC.turnNotificationClick, handler);
  },
  setThemeSource: (theme: DesktopThemeSource) =>
    ipcRenderer.invoke(DesktopIPC.setThemeSource, theme),
  setUpdateChannel: (channel: DesktopUpdateChannel) =>
    ipcRenderer.invoke(DesktopIPC.setUpdateChannel, channel),
  onUpdateStatus: (listener: (status: DesktopUpdateStatus) => void) => {
    const handler = (
      _event: Electron.IpcRendererEvent,
      status: DesktopUpdateStatus,
    ) => {
      listener(status);
    };
    ipcRenderer.on(DesktopIPC.updateStatus, handler);
    return () => {
      ipcRenderer.removeListener(DesktopIPC.updateStatus, handler);
    };
  },
});

contextBridge.exposeInMainWorld("csgclawDesktop", bridge);
