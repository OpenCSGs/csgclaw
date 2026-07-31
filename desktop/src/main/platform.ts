import path from "node:path";
import { app } from "electron";
import { DesktopPlatform } from "../shared/desktopEnvironment";

export const isMacOSDesktop = process.platform === DesktopPlatform.MacOS;
export const isWindowsDesktop = process.platform === DesktopPlatform.Windows;

export function desktopIconResourcePath(fileName: string): string {
  return app.isPackaged
    ? path.join(process.resourcesPath, fileName)
    : path.resolve(__dirname, "..", "..", "resources", "icons", fileName);
}

export function windowsAppIconPath(): string {
  return desktopIconResourcePath("csgclaw.ico");
}
