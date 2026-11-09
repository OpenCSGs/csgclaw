import { useEffect } from "react";
import { useWorkspaceUiStore } from "@/hooks/workspace/workspaceUiStore";
import { initializeMermaidTheme } from "@/components/business/MessageContent/mermaid";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";
import { THEME_STORAGE_KEY } from "@/shared/storage/keys";
import { parseThemeMode, resolveThemeMode } from "@/shared/theme/theme";

export function AppThemeSynchronizer() {
  const theme = useWorkspaceUiStore((state) => state.theme);
  const setTheme = useWorkspaceUiStore((state) => state.setTheme);

  useEffect(() => {
    const applyTheme = () => {
      const resolved = resolveThemeMode(theme);
      document.documentElement.dataset.theme = resolved;
      document.documentElement.style.colorScheme = resolved;
      initializeMermaidTheme(resolved);
    };
    applyTheme();
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
      // Restricted storage must not prevent an appearance change.
    }
    void getDesktopBridge()
      ?.setThemeSource(theme)
      .catch(() => undefined);
    if (theme !== "system" || typeof window.matchMedia !== "function") return;
    try {
      const media = window.matchMedia("(prefers-color-scheme: dark)");
      media.addEventListener("change", applyTheme);
      return () => media.removeEventListener("change", applyTheme);
    } catch {
      // Some embedded browsers expose matchMedia without supporting subscriptions.
    }
  }, [theme]);

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === THEME_STORAGE_KEY || event.key === null) setTheme(parseThemeMode(event.newValue));
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, [setTheme]);

  return null;
}
