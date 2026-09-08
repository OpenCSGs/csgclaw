import type { DesktopThemeSource } from "../shared/desktopBridge.types";
import { shouldUseDarkThemeIcon } from "../shared/desktopTheme";

// A fixed deadline, not trailing debounce: repeated clicks cannot starve refresh.
export class WindowsTaskbarRefreshScheduler {
  private timer: ReturnType<typeof setTimeout> | null = null;

  request(refresh: () => void): void {
    if (this.timer !== null) return;
    this.timer = setTimeout(() => {
      this.timer = null;
      refresh();
    }, 100);
  }

  cancel(): void {
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
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
