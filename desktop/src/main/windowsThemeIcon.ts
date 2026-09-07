import type { DesktopThemeSource } from "../shared/desktopBridge.types";
import { shouldUseDarkThemeIcon } from "../shared/desktopTheme";

export function windowsThemeIconName(
  theme: DesktopThemeSource,
  systemUsesDarkColors: boolean,
): string {
  const useDarkColors = shouldUseDarkThemeIcon(theme, systemUsesDarkColors);
  return useDarkColors
    ? "csgclaw-taskbar-dark.ico"
    : "csgclaw-taskbar-light.ico";
}
