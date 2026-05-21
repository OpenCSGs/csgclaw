import { Button } from "@/components/ui";
import { GlobeIcon, MoonIcon, SidebarToggleIcon, SunIcon } from "@/components/ui/Icons";

export function SidebarHeader({
  theme,
  onThemeChange,
  locale,
  onLocaleChange,
  t,
  currentWorkspaceLabel,
  runningAgentCount,
  agentCount,
  onCollapseSidebar,
}) {
  return (
    <div className="sidebar-header workspace-header">
      <div className="sidebar-brand-row">
        <div className="sidebar-brand-lockup" aria-label="CSGClaw">
          <img
            className="sidebar-brand-logo sidebar-brand-logo-light"
            src="/brand/csgclaw-logo-light.svg"
            alt=""
            aria-hidden="true"
          />
          <img
            className="sidebar-brand-logo sidebar-brand-logo-dark"
            src="/brand/csgclaw-logo-dark.svg"
            alt=""
            aria-hidden="true"
          />
        </div>
        <div className="sidebar-controls">
          <div className="theme-switch" role="group" aria-label={t("themeSwitcher")}>
            <div className={`theme-switch-track ${theme === "dark" ? "is-dark" : "is-light"}`}>
              <span className="theme-switch-thumb" aria-hidden="true"></span>
              <Button
                variant="ghost"
                className="theme-toggle"
                active={theme === "light"}
                aria-label={t("themeLight")}
                aria-pressed={theme === "light"}
                title={t("themeLight")}
                onClick={() => onThemeChange("light")}
              >
                <span aria-hidden="true">
                  <SunIcon />
                </span>
              </Button>
              <Button
                variant="ghost"
                className="theme-toggle"
                active={theme === "dark"}
                aria-label={t("themeDark")}
                aria-pressed={theme === "dark"}
                title={t("themeDark")}
                onClick={() => onThemeChange("dark")}
              >
                <span aria-hidden="true">
                  <MoonIcon />
                </span>
              </Button>
            </div>
          </div>
          <div className="language-switch sidebar-language-switch" role="group" aria-label={t("languageSwitcher")}>
            <span className="language-switch-icon" aria-hidden="true">
              <GlobeIcon />
            </span>
            <div className={`language-switch-track ${locale === "en" ? "is-en" : "is-zh"}`}>
              <span className="language-switch-thumb" aria-hidden="true"></span>
              <Button
                variant="ghost"
                className="language-toggle"
                active={locale === "zh"}
                aria-pressed={locale === "zh"}
                title={t("languageOptionZh")}
                onClick={() => onLocaleChange("zh")}
              >
                中
              </Button>
              <Button
                variant="ghost"
                className="language-toggle"
                active={locale === "en"}
                aria-pressed={locale === "en"}
                title={t("languageOptionEn")}
                onClick={() => onLocaleChange("en")}
              >
                EN
              </Button>
            </div>
          </div>
          <Button
            variant="ghost"
            className="sidebar-toggle-button"
            aria-label={t("collapseSidebar")}
            title={t("collapseSidebar")}
            onClick={onCollapseSidebar}
          >
            <span className="sidebar-toggle-mark">
              <SidebarToggleIcon />
            </span>
          </Button>
        </div>
      </div>
      <div className="workspace-signal-panel" aria-label={currentWorkspaceLabel}>
        <div className="workspace-signal-copy">
          <span>{currentWorkspaceLabel}</span>
          <strong>
            {runningAgentCount}/{agentCount} {t("activeNow")}
          </strong>
        </div>
        <div className="workspace-signal-meter" aria-hidden="true">
          <span></span>
          <span></span>
          <span></span>
        </div>
      </div>
    </div>
  );
}
