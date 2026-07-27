import { contextBridge, ipcRenderer } from "electron";
import {
  DesktopIPC,
  type DesktopBridge,
  type DesktopOAuthInput,
  type DesktopUpdateStatus,
} from "../shared/desktopBridge.types";

const bridge: DesktopBridge = Object.freeze({
  getRuntimeInfo: () => ipcRenderer.invoke(DesktopIPC.getRuntimeInfo),
  openOAuth: (input: DesktopOAuthInput) => ipcRenderer.invoke(DesktopIPC.openOAuth, input),
  checkForUpdates: () => ipcRenderer.invoke(DesktopIPC.checkForUpdates),
  installDownloadedUpdate: () => ipcRenderer.invoke(DesktopIPC.installDownloadedUpdate),
  onUpdateStatus: (listener: (status: DesktopUpdateStatus) => void) => {
    const handler = (_event: Electron.IpcRendererEvent, status: DesktopUpdateStatus) => {
      listener(status);
    };
    ipcRenderer.on(DesktopIPC.updateStatus, handler);
    return () => {
      ipcRenderer.removeListener(DesktopIPC.updateStatus, handler);
    };
  },
});

contextBridge.exposeInMainWorld("csgclawDesktop", bridge);
