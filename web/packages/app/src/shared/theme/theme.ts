import { THEME_STORAGE_KEY } from "@/shared/storage/keys";

export type ResolvedThemeMode = "light" | "dark";
export type ThemeMode = "system" | ResolvedThemeMode;

export function parseThemeMode(value: unknown): ThemeMode {
  return value === "system" || value === "light" || value === "dark" ? value : "dark";
}

export function detectInitialTheme(): ThemeMode {
  try {
    return parseThemeMode(typeof window === "undefined" ? null : window.localStorage.getItem(THEME_STORAGE_KEY));
  } catch {
    return "dark";
  }
}

export function resolveThemeMode(theme: ThemeMode): ResolvedThemeMode {
  if (theme === "light" || theme === "dark") {
    return theme;
  }
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return "dark";
  }
  try {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  } catch {
    return "dark";
  }
}
