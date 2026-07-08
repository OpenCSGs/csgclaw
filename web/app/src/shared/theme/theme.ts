import { THEME_STORAGE_KEY } from "@/shared/storage/keys";

export type ResolvedThemeMode = "light" | "dark";
export type ThemeMode = "system" | ResolvedThemeMode;

export function detectInitialTheme(): ThemeMode {
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "system" || stored === "light" || stored === "dark") {
    return stored;
  }
  return "dark";
}

export function resolveThemeMode(theme: ThemeMode): ResolvedThemeMode {
  if (theme === "light" || theme === "dark") {
    return theme;
  }
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return "dark";
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
