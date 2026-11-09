import { Monitor, Moon, Sun } from "lucide-react";
import { SegmentedControl } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

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
          <SegmentedControl
            value={sidebar.theme}
            onChange={sidebar.onThemeChange}
            label={t("themeSwitcher")}
            options={[
              { value: "system", accessibleName: t("themeSystem"), label: <Monitor size={20} aria-hidden="true" /> },
              { value: "light", accessibleName: t("themeLight"), label: <Sun size={20} aria-hidden="true" /> },
              { value: "dark", accessibleName: t("themeDark"), label: <Moon size={20} aria-hidden="true" /> },
            ]}
          />
        </div>
        <div className="field">
          <span>{t("languageSwitcher")}</span>
          <SegmentedControl
            value={sidebar.locale}
            onChange={sidebar.onLocaleChange}
            label={t("languageSwitcher")}
            options={[
              { value: "zh", label: t("languageOptionZh") },
              { value: "en", label: t("languageOptionEn") },
            ]}
          />
        </div>
      </div>
    </SettingsRow>
  );
}
