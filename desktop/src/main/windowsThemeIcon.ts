import type { DesktopThemeSource } from "../shared/desktopBridge.types";
import { shouldUseDarkThemeIcon } from "../shared/desktopTheme";

export class WindowsTaskbarRefreshScheduler {
  private timer: ReturnType<typeof setTimeout> | null = null;
  private refreshActive = false;
  private pending = false;

  request(refresh: () => void | Promise<void>): void {
    this.pending = true;
    if (this.timer !== null || this.refreshActive) return;
    this.schedule(refresh);
  }

  private schedule(refresh: () => void | Promise<void>): void {
    this.timer = setTimeout(() => {
      this.timer = null;
      if (!this.pending) return;
      this.pending = false;
      this.refreshActive = true;
      void Promise.resolve(refresh()).finally(() => {
        this.refreshActive = false;
        if (this.pending) this.schedule(refresh);
      });
    }, 100);
  }

  cancel(): void {
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
    this.pending = false;
  }
}

export function windowsThemeIconName(
  theme: DesktopThemeSource,
  systemUsesDarkColors: boolean,
): string {
  const useDarkColors = shouldUseDarkThemeIcon(theme, systemUsesDarkColors);
  return useDarkColors
    ? "csgclaw-taskbar-dark.ico"
    : "csgclaw-taskbar-light.ico";
}
