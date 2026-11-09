import type { BrowserWindow } from "electron";

export function restoreWindowFocusAfterDialog(
  window: BrowserWindow,
  platform: NodeJS.Platform = process.platform,
): boolean {
  if (platform !== "win32" || window.isDestroyed()) {
    return false;
  }
  window.blur();
  window.focus();
  return true;
}
