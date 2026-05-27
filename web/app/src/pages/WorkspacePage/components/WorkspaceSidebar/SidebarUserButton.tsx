import { useEffect, useRef, useState } from "react";
import { Settings } from "lucide-react";
import { Button } from "@/components/ui";
import { MoonIcon, SunIcon } from "@/components/ui/Icons";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import { formatSidebarVersionLabel, hasUpgradeAttention, isLocalBuildVersion, upgradeStatusLabel } from "@/models/upgradeStatus";
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
  onOpenUpgrade,
  t,
}: SidebarUserButtonProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const upgradeAttention = hasUpgradeAttention(upgradeStatus, upgradePhase, upgradeBusy);
  const upgradeRunning = upgradeBusy || Boolean(upgradeStatus?.upgrading);
  const upgradeIssue = upgradeError || upgradeStatus?.last_error || "";
  const latestVersion = upgradeStatus?.latest_version || t("upgradeNoLatest");
  const upgradeMenuStatus = upgradeStatusText({
    phase: upgradePhase,
    running: upgradeRunning,
    issue: upgradeIssue,
    known: Boolean(upgradeStatus),
    currentVersion: upgradeStatus?.current_version || appVersion,
    manualRestartRequired: Boolean(upgradeStatus?.manual_restart_required),
    updateAvailable: Boolean(upgradeStatus?.update_available),
    t,
  });
  const upgradeActionLabel = upgradeMenuActionText({
    phase: upgradePhase,
    running: upgradeRunning,
    issue: upgradeIssue,
    manualRestartRequired: Boolean(upgradeStatus?.manual_restart_required),
    t,
  });

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
              <span className="sidebar-menu-label">{t("versionSettings")}</span>
              {upgradeAttention ? <span className="sidebar-version-alert-dot" aria-hidden="true"></span> : null}
            </div>
            <div className="sidebar-version-row">
              <span>{t("upgradeCurrentVersion")}</span>
              <strong>{formatSidebarVersionLabel(appVersion)}</strong>
            </div>
            <div className="sidebar-version-row">
              <span>{t("upgradeLatestVersion")}</span>
              <strong>{latestVersion}</strong>
            </div>
            <div className="sidebar-version-row">
              <span>{t("upgradeStatus")}</span>
              <strong>{upgradeMenuStatus}</strong>
            </div>
            {upgradeIssue ? <div className="sidebar-version-error">{upgradeIssue}</div> : null}
            {upgradeAttention ? (
              <Button
                variant={upgradePhase === "done" ? "secondaryColor" : "secondaryGray"}
                className={classNames(
                  "sidebar-upgrade-menu-button",
                  upgradeRunning && "is-running",
                  upgradePhase === "done" && "is-done",
                )}
                onClick={handleOpenUpgrade}
              >
                <span className="sidebar-upgrade-menu-dot" aria-hidden="true"></span>
                <span>{upgradeActionLabel}</span>
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function upgradeStatusText({
  phase,
  running,
  issue,
  known,
  currentVersion,
  manualRestartRequired,
  updateAvailable,
  t,
}: {
  phase: UpgradePhase;
  running: boolean;
  issue: string;
  known: boolean;
  currentVersion: string;
  manualRestartRequired: boolean;
  updateAvailable: boolean;
  t: TranslateFn;
}): string {
  if (issue || phase === "error") {
    return t("upgradeStatusError");
  }
  if (phase === "manual_restart" || manualRestartRequired) {
    return t("upgradeStatusManualRestart");
  }
  if (phase === "done") {
    return t("upgradeStatusDone");
  }
  if (running || phase === "starting" || phase === "restarting") {
    return upgradeStatusLabel(phase === "idle" ? "restarting" : phase, t);
  }
  if (!known) {
    return t("upgradeNoLatest");
  }
  if (isLocalBuildVersion(currentVersion)) {
    return t("upgradeStatusLocal");
  }
  return updateAvailable ? t("upgradeAction") : t("upgradeUpToDate");
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
