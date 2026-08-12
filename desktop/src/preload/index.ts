import { contextBridge, ipcRenderer } from "electron";
import type {
  DesktopBridge,
  DesktopOAuthInput,
  DesktopThemeSource,
  DesktopUpdateChannel,
  DesktopUpdateStatus,
} from "../shared/desktopBridge.types";

// Sandboxed preload scripts cannot require local CommonJS modules at runtime.
// Keep these channels inlined so the compiled preload remains a single file.
const DesktopIPC = {
  checkForUpdates: "csgclaw:desktop:check-for-updates",
  getRuntimeInfo: "csgclaw:desktop:get-runtime-info",
  installDownloadedUpdate: "csgclaw:desktop:install-downloaded-update",
  openOAuth: "csgclaw:desktop:open-oauth",
  restartSidecar: "csgclaw:desktop:restart-sidecar",
  setThemeSource: "csgclaw:desktop:set-theme-source",
  setUpdateChannel: "csgclaw:desktop:set-update-channel",
  updateStatus: "csgclaw:desktop:update-status",
} as const;

const bridge: DesktopBridge = Object.freeze({
  getRuntimeInfo: () => ipcRenderer.invoke(DesktopIPC.getRuntimeInfo),
  openOAuth: (input: DesktopOAuthInput) =>
    ipcRenderer.invoke(DesktopIPC.openOAuth, input),
  checkForUpdates: () => ipcRenderer.invoke(DesktopIPC.checkForUpdates),
  installDownloadedUpdate: () =>
    ipcRenderer.invoke(DesktopIPC.installDownloadedUpdate),
  restartSidecar: () => ipcRenderer.invoke(DesktopIPC.restartSidecar),
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
