import { useEffect, useRef, useState } from "react";
import { LogIn, LogOut, Settings } from "lucide-react";
import { Button } from "@/components/ui";
import { MoonIcon, SunIcon } from "@/components/ui/Icons";
import { isAuthenticated } from "@/models/auth";
import type { AuthStatus } from "@/models/auth";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import { formatSidebarVersionLabel, hasUpgradeAttention, isLocalBuildUpgradeStatus } from "@/models/upgradeStatus";
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
  onOpenConfigSettings?: () => void;
  authStatus?: AuthStatus | null;
  authBusy?: boolean;
  authPending?: boolean;
  authError?: string;
  onLogin?: () => void | Promise<void>;
  onLogout?: () => void | Promise<void>;
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
  onOpenConfigSettings,
  authStatus = null,
  authBusy = false,
  authPending = false,
  authError = "",
  onLogin,
  onLogout,
  t,
}: SidebarUserButtonProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const currentVersion = upgradeStatus?.current_version || appVersion;
  const localBuild = isLocalBuildUpgradeStatus(upgradeStatus, currentVersion);
  const upgradeControlsAvailable =
    showUpgradeControls && !localBuild && upgradeStatus?.auto_upgrade_supported !== false;
  const upgradeAttention = upgradeControlsAvailable && hasUpgradeAttention(upgradeStatus, upgradePhase, upgradeBusy);
  const upgradeRunning = upgradeControlsAvailable ? upgradeBusy || Boolean(upgradeStatus?.upgrading) : false;
  const upgradeIssue = upgradeControlsAvailable ? upgradeError || upgradeStatus?.last_error || "" : "";
  const upgradeView = upgradeControlsAvailable
    ? {
        actionLabel: upgradeMenuActionText({
          phase: upgradePhase,
          running: upgradeRunning,
          issue: upgradeIssue,
          manualRestartRequired: Boolean(upgradeStatus?.manual_restart_required),
          t,
        }),
        issue: upgradeIssue,
        running: upgradeRunning,
        versionLabel:
          upgradeStatus?.update_available && upgradeStatus.latest_version
            ? `${formatSidebarVersionLabel(currentVersion)} -> ${formatSidebarVersionLabel(upgradeStatus.latest_version)}`
            : formatSidebarVersionLabel(currentVersion),
      }
    : null;
  const accountAuthenticated = isAuthenticated(authStatus);
  const accountDisplayName = authStatus?.user_id || authStatus?.user_uuid || t("csghubSignedIn");
  const loginLabel = authPending ? t("csghubLoginPending") : authBusy ? t("csghubSigningIn") : t("csghubSignIn");

  function handleOpenUpgrade() {
    setOpen(false);
    onOpenUpgrade?.();
  }

  function handleOpenConfigSettings() {
    setOpen(false);
    onOpenConfigSettings?.();
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
          <div className="sidebar-csghub-panel">
            <div className="sidebar-csghub-heading">
              <span className="sidebar-csghub-kicker">{t("csghubAccount")}</span>
              <span className={classNames("sidebar-csghub-state", accountAuthenticated && "is-authenticated")}>
                {accountAuthenticated ? t("csghubSignedIn") : t("csghubNotSignedIn")}
              </span>
            </div>
            {accountAuthenticated ? (
              <div className="sidebar-csghub-account">
                <span className="sidebar-csghub-identity">
                  <span className="sidebar-csghub-avatar" aria-hidden="true">
                    {authStatus?.avatar ? (
                      <img src={authStatus.avatar} alt="" />
                    ) : (
                      <span>{initialsForAccount(accountDisplayName)}</span>
                    )}
                  </span>
                  <strong>{accountDisplayName}</strong>
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="sidebar-csghub-action"
                  disabled={authBusy}
                  role="menuitem"
                  onClick={() => onLogout?.()}
                >
                  <LogOut size={14} strokeWidth={2} aria-hidden="true" />
                  <span>{t("csghubSignOut")}</span>
                </Button>
              </div>
            ) : (
              <div className="sidebar-csghub-account is-disconnected">
                <span className="sidebar-csghub-status">
                  {authPending ? t("csghubLoginPendingDetail") : t("csghubNotSignedIn")}
                </span>
                <Button
                  variant="secondaryColor"
                  size="sm"
                  className="sidebar-csghub-action"
                  disabled={authBusy || authPending}
                  role="menuitem"
                  onClick={() => onLogin?.()}
                >
                  <LogIn size={14} strokeWidth={2} aria-hidden="true" />
                  <span>{loginLabel}</span>
                </Button>
              </div>
            )}
            {authError ? <div className="sidebar-csghub-error">{authError}</div> : null}
          </div>
          <div className="sidebar-menu-divider"></div>
          <button type="button" className="sidebar-menu-row" role="menuitem" onClick={handleOpenConfigSettings}>
            {t("configSettingsMenu")}
          </button>
          <div className="sidebar-menu-divider"></div>
          <div className="sidebar-version-panel">
            <div className="sidebar-version-heading">
              <span className="sidebar-menu-label">{t("versionInfo")}</span>
              {localBuild ? (
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
            {localBuild ? null : (
              <strong className="sidebar-version-value">
                {upgradeView ? upgradeView.versionLabel : formatSidebarVersionLabel(currentVersion)}
              </strong>
            )}
            {upgradeView?.issue ? <div className="sidebar-version-error">{upgradeView.issue}</div> : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function initialsForAccount(name: string): string {
  const cleaned = name.trim();
  if (!cleaned) {
    return "CS";
  }
  return cleaned.slice(0, 2).toUpperCase();
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
