import path from "node:path";
import { app } from "electron";
import { DesktopPlatform } from "../shared/desktopEnvironment";

export const isMacOSDesktop = process.platform === DesktopPlatform.MacOS;
export const isWindowsDesktop = process.platform === DesktopPlatform.Windows;
export const windowsAppUserModelID = "com.squirrel.csgclaw_desktop.CSGClaw";

export function windowsTaskbarAppUserModelID(useDarkColors: boolean): string {
  return `${windowsAppUserModelID}.${useDarkColors ? "dark" : "light"}`;
}

export function desktopIconResourcePath(fileName: string): string {
  return app.isPackaged
    ? path.join(process.resourcesPath, fileName)
    : path.resolve(__dirname, "..", "..", "resources", "icons", fileName);
}

export function windowsAppIconPath(): string {
  return desktopIconResourcePath("csgclaw.ico");
}
