import type { DesktopThemeSource } from "./desktopBridge.types";

export function parseDesktopThemeSource(input: unknown): DesktopThemeSource {
  if (input !== "system" && input !== "light" && input !== "dark") {
    throw new Error("Desktop theme source is invalid.");
  }
  return input;
}

export function shouldUseDarkThemeIcon(
  theme: DesktopThemeSource,
  systemUsesDarkColors: boolean,
): boolean {
  return theme === "system" ? systemUsesDarkColors : theme === "dark";
}
