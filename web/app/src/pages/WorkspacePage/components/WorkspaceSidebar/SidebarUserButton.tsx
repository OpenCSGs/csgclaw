import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight, ExternalLink, LogIn, LogOut, Settings, SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui";
import { MoonIcon, SunIcon } from "@/components/ui/Icons";
import { isAuthenticated } from "@/models/auth";
import type { AuthStatus } from "@/models/auth";
import {
  AUTH_ENVIRONMENT_PRESETS,
  authEnvironmentDisplayLabel,
  authEnvironmentDraftFromPreset,
  authEnvironmentDraftFromStatus,
  authEnvironmentLoginReady,
  defaultAuthEnvironmentDraft,
  normalizeAuthEnvironmentDraft,
  resolveAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft, AuthEnvironmentPresetID } from "@/models/authEnvironment";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import { githubFeedbackIssueURL } from "@/models/feedback";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import {
  formatSidebarVersionLabel,
  hasUpgradeAttention,
  isLocalBuildUpgradeStatus,
  upgradeErrorMessage,
} from "@/models/upgradeStatus";
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
  suppressUpgradeIssue?: boolean;
  showUpgradeControls?: boolean;
  onOpenUpgrade?: () => void;
  onOpenConfigSettings?: () => void;
  authStatus?: AuthStatus | null;
  authBusy?: boolean;
  authPending?: boolean;
  authError?: string;
  onLogin?: (environment?: AuthEnvironmentDraft) => void | Promise<void>;
  onLogout?: () => void | Promise<void>;
  t: TranslateFn;
};

const AUTH_ENVIRONMENT_STORAGE_KEY = "csgclaw.auth.environment";

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
  suppressUpgradeIssue = false,
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
  const [authEnvironmentDraft, setAuthEnvironmentDraft] =
    useState<AuthEnvironmentDraft>(readStoredAuthEnvironmentDraft);
  const [accountPanelOpen, setAccountPanelOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const currentVersion = upgradeStatus?.current_version || appVersion;
  const feedbackURL = githubFeedbackIssueURL(appVersion, upgradeStatus);
  const localBuild = isLocalBuildUpgradeStatus(upgradeStatus, currentVersion);
  const upgradeControlsAvailable =
    showUpgradeControls && !localBuild && upgradeStatus?.auto_upgrade_supported !== false;
  const upgradeAttention = upgradeControlsAvailable && hasUpgradeAttention(upgradeStatus, upgradePhase, upgradeBusy);
  const upgradeRunning = upgradeControlsAvailable ? upgradeBusy || Boolean(upgradeStatus?.upgrading) : false;
  const statusIssue = upgradeErrorMessage(upgradeStatus, t);
  const upgradeIssue = upgradeControlsAvailable && !suppressUpgradeIssue ? upgradeError || statusIssue : "";
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
  const accountUserID = authStatus?.user_id || "";
  const accountUserName = authStatus?.name || "";
  const accountDisplayName = accountUserName || accountUserID || authStatus?.user_uuid || t("csghubSignedIn");
  const loginLabel = authPending ? t("csghubLoginPending") : authBusy ? t("csghubSigningIn") : t("csghubSignIn");
  const activeAuthEnvironmentDraft = accountAuthenticated
    ? authEnvironmentDraftFromStatus(authStatus, authEnvironmentDraft)
    : authEnvironmentDraft;
  const authEnvironmentReady = authEnvironmentLoginReady(authEnvironmentDraft);
  const authActionDisabled = authBusy || authPending || !authEnvironmentReady;
  const showAuthEnvironmentAdvanced = advancedOpen || authEnvironmentDraft.preset === "custom";
  const authEnvironmentLabel = authEnvironmentDisplayLabel(activeAuthEnvironmentDraft, t("csghubEnvCustom"));
  const showAuthEnvironmentLabel =
    authEnvironmentLabel !== authEnvironmentDisplayLabel(defaultAuthEnvironmentDraft(), t("csghubEnvCustom"));
  const accountSummaryLabel = accountAuthenticated
    ? `${accountDisplayName} · ${t("csghubSignedIn")}`
    : t("csghubNotSignedIn");

  function handleOpenUpgrade() {
    setOpen(false);
    onOpenUpgrade?.();
  }

  function handleOpenConfigSettings() {
    setOpen(false);
    onOpenConfigSettings?.();
  }

  function handleAuthEnvironmentPresetChange(preset: AuthEnvironmentPresetID) {
    if (preset === "custom") {
      setAdvancedOpen(true);
      setAuthEnvironmentDraft((current) =>
        current.preset === "custom"
          ? current
          : {
              preset: "custom",
              opencsgBaseURL: "",
              csgHubBaseURL: "",
              aiGatewayBaseURL: "",
            },
      );
      return;
    }
    setAdvancedOpen(false);
    setAuthEnvironmentDraft(authEnvironmentDraftFromPreset(preset));
  }

  function handleAuthEnvironmentInputChange(value: string) {
    setAuthEnvironmentDraft((current) => {
      return {
        ...current,
        preset: "custom",
        opencsgBaseURL: value,
        csgHubBaseURL: "",
        aiGatewayBaseURL: "",
      };
    });
  }

  function handleLogin() {
    const next = resolveAuthEnvironmentDraft(authEnvironmentDraft);
    setAuthEnvironmentDraft(next);
    writeStoredAuthEnvironmentDraft(next);
    onLogin?.(next);
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

  useEffect(() => {
    writeStoredAuthEnvironmentDraft(authEnvironmentDraft);
  }, [authEnvironmentDraft]);

  useEffect(() => {
    if (!accountAuthenticated) {
      return;
    }
    setAuthEnvironmentDraft((current) => authEnvironmentDraftFromStatus(authStatus, current));
  }, [accountAuthenticated, authStatus]);

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
            <div className="sidebar-csghub-summary">
              {!accountAuthenticated ? (
                <button
                  type="button"
                  className="sidebar-csghub-expand"
                  aria-expanded={accountPanelOpen}
                  aria-label={t("csghubToggleEnvironmentPanel")}
                  title={t("csghubToggleEnvironmentPanel")}
                  onClick={() => setAccountPanelOpen((value) => !value)}
                >
                  <ChevronRight
                    className={classNames("sidebar-csghub-summary-chevron", accountPanelOpen && "is-open")}
                    size={15}
                    strokeWidth={2.3}
                    aria-hidden="true"
                  />
                </button>
              ) : null}
              <span className="sidebar-csghub-summary-main">
                <span className="sidebar-csghub-kicker">{t("csghubAccount")}</span>
                {showAuthEnvironmentLabel ? (
                  <span className="sidebar-csghub-env-chip">{authEnvironmentLabel}</span>
                ) : null}
              </span>
              <span
                className={classNames("sidebar-csghub-state", accountAuthenticated && "is-authenticated")}
                title={accountSummaryLabel}
              >
                <span className="sidebar-csghub-state-text">{accountSummaryLabel}</span>
              </span>
            </div>
            {!accountAuthenticated && accountPanelOpen ? (
              <div className="sidebar-csghub-env">
                <label className="sidebar-csghub-env-row">
                  <span className="sidebar-csghub-env-label">{t("csghubLoginEnvironment")}</span>
                  <span className="sidebar-csghub-env-combo">
                    <select
                      className="sidebar-csghub-env-control"
                      value={authEnvironmentDraft.preset}
                      onChange={(event) =>
                        handleAuthEnvironmentPresetChange(event.currentTarget.value as AuthEnvironmentPresetID)
                      }
                    >
                      {AUTH_ENVIRONMENT_PRESETS.map((preset) => (
                        <option key={preset.id} value={preset.id}>
                          {preset.label}
                        </option>
                      ))}
                      <option value="custom">{t("csghubEnvCustom")}</option>
                    </select>
                    <button
                      type="button"
                      className={classNames("sidebar-csghub-advanced-toggle", showAuthEnvironmentAdvanced && "is-open")}
                      aria-label={t("csghubAdvancedSettings")}
                      title={t("csghubAdvancedSettings")}
                      aria-expanded={showAuthEnvironmentAdvanced}
                      onClick={() => setAdvancedOpen((value) => !value)}
                    >
                      <SlidersHorizontal size={14} strokeWidth={2.2} aria-hidden="true" />
                      <ChevronDown size={13} strokeWidth={2.3} aria-hidden="true" />
                    </button>
                  </span>
                </label>
                {showAuthEnvironmentAdvanced ? (
                  <div className="sidebar-csghub-env-advanced">
                    <label className="sidebar-csghub-env-row">
                      <span className="sidebar-csghub-env-label">{t("csghubOpenCSGBaseURL")}</span>
                      <input
                        className="sidebar-csghub-env-control"
                        value={authEnvironmentDraft.opencsgBaseURL}
                        placeholder="https://openeast.opencsg.com"
                        onChange={(event) => handleAuthEnvironmentInputChange(event.currentTarget.value)}
                      />
                    </label>
                  </div>
                ) : null}
              </div>
            ) : null}
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
                <span className="sidebar-csghub-account-actions">
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
                </span>
              </div>
            ) : (
              <div className="sidebar-csghub-account is-disconnected has-status">
                <span className="sidebar-csghub-status">
                  {authPending ? t("csghubLoginPendingDetail") : t("csghubNotSignedIn")}
                </span>
                <Button
                  variant="secondaryColor"
                  size="sm"
                  className="sidebar-csghub-action"
                  disabled={authActionDisabled}
                  role="menuitem"
                  onClick={handleLogin}
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
            <a
              className="sidebar-feedback-link"
              href={feedbackURL}
              target="_blank"
              rel="noreferrer"
              role="menuitem"
              title={t("configSettingsGithubIssueAction")}
            >
              <span>{t("configSettingsFeedbackSection")}</span>
              <ExternalLink size={14} strokeWidth={2.1} aria-hidden="true" />
            </a>
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

function readStoredAuthEnvironmentDraft(): AuthEnvironmentDraft {
  try {
    return normalizeAuthEnvironmentDraft(
      JSON.parse(window.localStorage.getItem(AUTH_ENVIRONMENT_STORAGE_KEY) || "null"),
    );
  } catch (_) {
    return defaultAuthEnvironmentDraft();
  }
}

function writeStoredAuthEnvironmentDraft(draft: AuthEnvironmentDraft) {
  try {
    window.localStorage.setItem(AUTH_ENVIRONMENT_STORAGE_KEY, JSON.stringify(resolveAuthEnvironmentDraft(draft)));
  } catch (_) {
    // Local storage can be unavailable in restricted browser contexts.
  }
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
