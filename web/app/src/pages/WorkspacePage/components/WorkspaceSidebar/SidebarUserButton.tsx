import { useEffect, useRef, useState } from "react";
import { Settings } from "lucide-react";
import { Button } from "@/components/ui";
import { MoonIcon, SunIcon } from "@/components/ui/Icons";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import { formatSidebarVersionLabel, hasUpgradeAttention, isLocalBuildVersion } from "@/models/upgradeStatus";
import { classNames } from "@/shared/lib/classNames";
import type { ThemeMode } from "@/shared/theme/theme";

type SidebarUserButtonProps = {
  theme: ThemeMode;
  onThemeChange?: (theme: ThemeMode) => void;
  locale: LocaleCode;
  onLocaleChange?: (locale: LocaleCode) => void;
  appVersion?: string;
  upgradeStatus?: UpgradeStatus | null;
  upgradeBusy?: boolean;
  upgradePhase?: UpgradePhase;
  upgradeError?: string;
  showUpgradeControls?: boolean;
  onOpenUpgrade?: () => void;
  t: TranslateFn;
};

export function SidebarUserButton({
  theme,
  onThemeChange,
  locale,
  onLocaleChange,
  appVersion = "",
  upgradeStatus = null,
  upgradeBusy = false,
  upgradePhase = "idle",
  upgradeError = "",
  showUpgradeControls = true,
  onOpenUpgrade,
  t,
}: SidebarUserButtonProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const upgradeAttention = showUpgradeControls && hasUpgradeAttention(upgradeStatus, upgradePhase, upgradeBusy);
  const upgradeRunning = showUpgradeControls ? upgradeBusy || Boolean(upgradeStatus?.upgrading) : false;
  const upgradeIssue = showUpgradeControls ? upgradeError || upgradeStatus?.last_error || "" : "";
  const currentVersion = upgradeStatus?.current_version || appVersion;
  const upgradeView = showUpgradeControls
    ? {
        actionLabel: upgradeMenuActionText({
          phase: upgradePhase,
          running: upgradeRunning,
          issue: upgradeIssue,
          manualRestartRequired: Boolean(upgradeStatus?.manual_restart_required),
          t,
        }),
        issue: upgradeIssue,
        localBuild: isLocalBuildVersion(currentVersion),
        running: upgradeRunning,
        versionLabel:
          upgradeStatus?.update_available && upgradeStatus.latest_version
            ? `${formatSidebarVersionLabel(currentVersion)} -> ${formatSidebarVersionLabel(upgradeStatus.latest_version)}`
            : formatSidebarVersionLabel(currentVersion),
      }
    : null;

  function handleOpenUpgrade() {
    setOpen(false);
    onOpenUpgrade?.();
  }

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
        {upgradeAttention ? <span className="sidebar-settings-alert-dot" aria-hidden="true"></span> : null}
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
          <div className="sidebar-version-panel">
            <div className="sidebar-version-heading">
              <span className="sidebar-menu-label">{t("versionInfo")}</span>
              {upgradeView?.localBuild ? (
                <span className="sidebar-version-status">{t("upgradeLocalBuild")}</span>
              ) : upgradeAttention && upgradeView ? (
                <Button
                  variant={upgradePhase === "done" ? "secondaryColor" : "secondaryGray"}
                  size="sm"
                  className={classNames(
                    "sidebar-version-action",
                    upgradeView.running && "is-running",
                    upgradePhase === "done" && "is-done",
                  )}
                  onClick={handleOpenUpgrade}
                >
                  <span className="sidebar-upgrade-menu-dot" aria-hidden="true"></span>
                  <span>{upgradeView.actionLabel}</span>
                </Button>
              ) : null}
            </div>
            <strong className="sidebar-version-value">
              {upgradeView ? upgradeView.versionLabel : formatSidebarVersionLabel(appVersion)}
            </strong>
            {upgradeView?.issue ? <div className="sidebar-version-error">{upgradeView.issue}</div> : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function upgradeMenuActionText({
  phase,
  running,
  issue,
  manualRestartRequired,
  t,
}: {
  phase: UpgradePhase;
  running: boolean;
  issue: string;
  manualRestartRequired: boolean;
  t: TranslateFn;
}): string {
  if (phase === "done") {
    return t("upgradeRefresh");
  }
  if (phase === "manual_restart" || manualRestartRequired) {
    return t("upgradeViewProgress");
  }
  if (running || phase === "starting" || phase === "restarting" || issue || phase === "error") {
    return t("upgradeViewProgress");
  }
  return t("upgradeAction");
}
