import { THEME_STORAGE_KEY } from "@/shared/storage/keys";

export type ThemeMode = "light" | "dark";

export function detectInitialTheme(): ThemeMode {
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") {
    return stored;
  }
  return "dark";
}
