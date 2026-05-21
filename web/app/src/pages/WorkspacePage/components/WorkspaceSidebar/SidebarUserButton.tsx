import { useEffect, useRef, useState } from "react";
import { Settings } from "lucide-react";
import { Button } from "@/components/ui";
import { MoonIcon, SidebarToggleIcon, SunIcon } from "@/components/ui/Icons";

type SidebarUserButtonProps = {
  theme: string;
  onThemeChange?: (theme: "light" | "dark") => void;
  locale: string;
  onLocaleChange?: (locale: "zh" | "en") => void;
  onCollapseSidebar?: () => void;
  sidebarActionLabel?: string;
  t: (key: string) => string;
};

export function SidebarUserButton({
  theme,
  onThemeChange,
  locale,
  onLocaleChange,
  onCollapseSidebar,
  sidebarActionLabel = "",
  t,
}: SidebarUserButtonProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) {
      return undefined;
    }

    function handlePointerDown(event: PointerEvent) {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  return (
    <div ref={rootRef} className="sidebar-user-menu-root">
      <button
        type="button"
        className="sidebar-user-button"
        aria-label={t("settings")}
        aria-expanded={open}
        title={t("settings")}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="sidebar-settings-mark" aria-hidden="true">
          <Settings size={22} strokeWidth={2} />
        </span>
      </button>
      {open ? (
        <div className="sidebar-user-menu" role="menu" aria-label={t("settings")}>
          <div className="sidebar-menu-group">
            <span className="sidebar-menu-label">{t("appearanceSettings")}</span>
            <div className="sidebar-menu-segmented" role="group" aria-label={t("themeSwitcher")}>
              <Button
                variant="ghost"
                active={theme === "light"}
                aria-label={t("themeLight")}
                aria-pressed={theme === "light"}
                onClick={() => onThemeChange?.("light")}
              >
                <span className="sidebar-menu-icon" aria-hidden="true">
                  <SunIcon />
                </span>
              </Button>
              <Button
                variant="ghost"
                active={theme === "dark"}
                aria-label={t("themeDark")}
                aria-pressed={theme === "dark"}
                onClick={() => onThemeChange?.("dark")}
              >
                <span className="sidebar-menu-icon" aria-hidden="true">
                  <MoonIcon />
                </span>
              </Button>
            </div>
            <div className="sidebar-menu-segmented text-segmented" role="group" aria-label={t("languageSwitcher")}>
              <Button
                variant="ghost"
                active={locale === "zh"}
                aria-pressed={locale === "zh"}
                onClick={() => onLocaleChange?.("zh")}
              >
                中
              </Button>
              <Button
                variant="ghost"
                active={locale === "en"}
                aria-pressed={locale === "en"}
                onClick={() => onLocaleChange?.("en")}
              >
                EN
              </Button>
            </div>
          </div>
          <div className="sidebar-menu-divider"></div>
          <button
            type="button"
            className="sidebar-menu-row"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onCollapseSidebar?.();
            }}
          >
            <span>{sidebarActionLabel || t("collapseSidebar")}</span>
            <span className="sidebar-menu-icon" aria-hidden="true">
              <SidebarToggleIcon />
            </span>
          </button>
        </div>
      ) : null}
    </div>
  );
}
