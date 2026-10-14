import { Monitor, Moon, Sun } from "lucide-react";
import { Tooltip } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { classNames } from "@/shared/lib/classNames";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
import type { ThemeMode } from "@/shared/theme/theme";
import type { ReactNode } from "react";

export function AppearanceSection() {
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  if (!sidebar) {
    return null;
  }
  const t = sidebar.t;

  return (
    <SettingsRow title={t("appearanceSettings")} description={t("settingsAppearanceDescription")}>
      <div className={styles.formGrid}>
        <div className="field">
          <span>{t("themeSwitcher")}</span>
          <div className={styles.segmented} role="group" aria-label={t("themeSwitcher")}>
            <ThemeSegmentButton
              active={sidebar.theme === "system"}
              label={t("themeSystem")}
              theme="system"
              onThemeChange={sidebar.onThemeChange}
            >
              <Monitor size={20} strokeWidth={2} aria-hidden="true" />
            </ThemeSegmentButton>
            <ThemeSegmentButton
              active={sidebar.theme === "light"}
              label={t("themeLight")}
              theme="light"
              onThemeChange={sidebar.onThemeChange}
            >
              <Sun size={20} strokeWidth={2} aria-hidden="true" />
            </ThemeSegmentButton>
            <ThemeSegmentButton
              active={sidebar.theme === "dark"}
              label={t("themeDark")}
              theme="dark"
              onThemeChange={sidebar.onThemeChange}
            >
              <Moon size={20} strokeWidth={2} aria-hidden="true" />
            </ThemeSegmentButton>
          </div>
        </div>
        <div className="field">
          <span>{t("languageSwitcher")}</span>
          <div className={styles.segmented} role="group" aria-label={t("languageSwitcher")}>
            <button
              type="button"
              className={classNames(styles.textSegmentButton, sidebar.locale === "zh" && styles.active)}
              aria-pressed={sidebar.locale === "zh"}
              onClick={() => sidebar.onLocaleChange("zh")}
            >
              {t("languageOptionZh")}
            </button>
            <button
              type="button"
              className={classNames(styles.textSegmentButton, sidebar.locale === "en" && styles.active)}
              aria-pressed={sidebar.locale === "en"}
              onClick={() => sidebar.onLocaleChange("en")}
            >
              {t("languageOptionEn")}
            </button>
          </div>
        </div>
      </div>
    </SettingsRow>
  );
}

function ThemeSegmentButton({
  active,
  children,
  label,
  onThemeChange,
  theme,
}: {
  active: boolean;
  children: ReactNode;
  label: string;
  onThemeChange: (theme: ThemeMode) => void;
  theme: ThemeMode;
}) {
  return (
    <Tooltip content={label}>
      <button
        type="button"
        className={classNames(styles.segmentButton, active && styles.active)}
        aria-label={label}
        aria-pressed={active}
        onClick={() => onThemeChange(theme)}
      >
        {children}
      </button>
    </Tooltip>
  );
}
