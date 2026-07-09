import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight, ExternalLink, LogIn, LogOut, Monitor, SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui";
import { MoonIcon, SidebarGearIcon, SunIcon } from "@/components/ui/Icons";
import { isAuthenticated } from "@/models/auth";
import type { AuthStatus } from "@/models/auth";
import {
  AUTH_ENVIRONMENT_PRESETS,
  authEnvironmentDisplayLabel,
  authEnvironmentDraftFromPreset,
  authEnvironmentDraftFromStatus,
  authEnvironmentLoginReady,
  defaultAuthEnvironmentDraft,
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
import { readStoredAuthEnvironmentDraft, writeStoredAuthEnvironmentDraft } from "@/shared/storage/authEnvironment";
import type { ThemeMode } from "@/shared/theme/theme";
import styles from "./SidebarUserButton.module.css";

type SidebarUserButtonProps = {
  active?: boolean;
  presentation?: "icon" | "row";
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
  onOpenSettings?: () => void;
  authStatus?: AuthStatus | null;
  authBusy?: boolean;
  authPending?: boolean;
  authError?: string;
  onLogin?: (environment?: AuthEnvironmentDraft) => void | Promise<void>;
  onLogout?: () => void | Promise<void>;
  t: TranslateFn;
};

export function SidebarUserButton({
  active = false,
  presentation = "icon",
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
  onOpenSettings,
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

  function handlePrimaryClick() {
    if (onOpenSettings) {
      setOpen(false);
      onOpenSettings();
      return;
    }
    setOpen((value) => !value);
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
    <div ref={rootRef} className={classNames(styles.root, presentation === "row" ? styles.rootRow : styles.rootIcon)}>
      <button
        type="button"
        className={classNames(
          styles.button,
          presentation === "row" ? styles.buttonRow : styles.buttonIcon,
          active && styles.active,
        )}
        aria-label={t("settings")}
        aria-current={active ? "page" : undefined}
        aria-expanded={onOpenSettings ? undefined : open}
        title={t("settings")}
        onClick={handlePrimaryClick}
      >
        <span className={styles.settingsMark} aria-hidden="true">
          <SidebarGearIcon size={24} />
        </span>
        {presentation === "row" ? <span className={styles.buttonLabel}>{t("settings")}</span> : null}
        {upgradeAttention ? <span className={styles.alertDot} aria-hidden="true"></span> : null}
      </button>
      {open ? (
        <div className={styles.menu} role="menu" aria-label={t("settings")}>
          <div className={styles.group}>
            <span className={styles.menuLabel}>{t("appearanceSettings")}</span>
            <div className={styles.segmented} role="group" aria-label={t("themeSwitcher")}>
              <Button
                variant="ghost"
                active={theme === "system"}
                aria-label={t("themeSystem")}
                aria-pressed={theme === "system"}
                onClick={() => onThemeChange?.("system")}
              >
                <Monitor size={16} strokeWidth={2} aria-hidden="true" />
              </Button>
              <Button
                variant="ghost"
                active={theme === "light"}
                aria-label={t("themeLight")}
                aria-pressed={theme === "light"}
                onClick={() => onThemeChange?.("light")}
              >
                <span className={styles.menuIcon} aria-hidden="true">
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
                <span className={styles.menuIcon} aria-hidden="true">
                  <MoonIcon />
                </span>
              </Button>
            </div>
            <div
              className={classNames(styles.segmented, styles.textSegmented)}
              role="group"
              aria-label={t("languageSwitcher")}
            >
              <Button
                variant="ghost"
                active={locale === "zh"}
                aria-pressed={locale === "zh"}
                onClick={() => onLocaleChange?.("zh")}
              >
                {t("languageOptionZh")}
              </Button>
              <Button
                variant="ghost"
                active={locale === "en"}
                aria-pressed={locale === "en"}
                onClick={() => onLocaleChange?.("en")}
              >
                {t("languageOptionEn")}
              </Button>
            </div>
          </div>
          <div className={styles.divider}></div>
          <div className={styles.csghubPanel}>
            <div className={styles.csghubSummary}>
              {!accountAuthenticated ? (
                <button
                  type="button"
                  className={styles.csghubExpand}
                  aria-expanded={accountPanelOpen}
                  aria-label={t("csghubToggleEnvironmentPanel")}
                  title={t("csghubToggleEnvironmentPanel")}
                  onClick={() => setAccountPanelOpen((value) => !value)}
                >
                  <ChevronRight
                    className={classNames(styles.summaryChevron, accountPanelOpen && styles.open)}
                    size={15}
                    strokeWidth={2.3}
                    aria-hidden="true"
                  />
                </button>
              ) : null}
              <span className={styles.summaryMain}>
                <span className={styles.kicker}>{t("csghubAccount")}</span>
                {showAuthEnvironmentLabel ? <span className={styles.envChip}>{authEnvironmentLabel}</span> : null}
              </span>
              <span
                className={classNames(styles.state, accountAuthenticated && styles.authenticated)}
                title={accountSummaryLabel}
              >
                <span className={styles.stateText}>{accountSummaryLabel}</span>
              </span>
            </div>
            {!accountAuthenticated && accountPanelOpen ? (
              <div className={styles.env}>
                <label className={styles.envRow}>
                  <span className={styles.envLabel}>{t("csghubLoginEnvironment")}</span>
                  <span className={styles.envCombo}>
                    <select
                      className={styles.envControl}
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
                      className={classNames(
                        styles.advancedToggle,
                        showAuthEnvironmentAdvanced && styles.advancedToggleOpen,
                      )}
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
                  <div className={styles.envAdvanced}>
                    <label className={styles.envRow}>
                      <span className={styles.envLabel}>{t("csghubOpenCSGBaseURL")}</span>
                      <input
                        className={styles.envControl}
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
              <div className={styles.account}>
                <span className={styles.identity}>
                  <span className={styles.avatar} aria-hidden="true">
                    {authStatus?.avatar ? (
                      <img src={authStatus.avatar} alt="" />
                    ) : (
                      <span>{initialsForAccount(accountDisplayName)}</span>
                    )}
                  </span>
                  <strong>{accountDisplayName}</strong>
                </span>
                <span className={styles.accountActions}>
                  <Button
                    variant="ghost"
                    size="sm"
                    className={styles.action}
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
              <div className={classNames(styles.account, styles.disconnected, styles.hasStatus)}>
                <span className={styles.status}>
                  {authPending ? t("csghubLoginPendingDetail") : t("csghubNotSignedIn")}
                </span>
                <Button
                  variant="secondaryColor"
                  size="sm"
                  className={styles.action}
                  disabled={authActionDisabled}
                  role="menuitem"
                  onClick={handleLogin}
                >
                  <LogIn size={14} strokeWidth={2} aria-hidden="true" />
                  <span>{loginLabel}</span>
                </Button>
              </div>
            )}
            {authError ? <div className={styles.error}>{authError}</div> : null}
          </div>
          <div className={styles.divider}></div>
          <button type="button" className={styles.menuRow} role="menuitem" onClick={handleOpenConfigSettings}>
            {t("configSettingsMenu")}
          </button>
          <div className={styles.divider}></div>
          <div className={styles.versionPanel}>
            <div className={styles.versionHeading}>
              <span className={styles.menuLabel}>{t("versionInfo")}</span>
              {localBuild ? (
                <span className={styles.versionStatus}>{t("upgradeLocalBuild")}</span>
              ) : upgradeAttention && upgradeView ? (
                <Button
                  variant={upgradePhase === "done" ? "secondaryColor" : "secondaryGray"}
                  size="sm"
                  className={classNames(
                    styles.versionAction,
                    upgradeView.running && styles.running,
                    upgradePhase === "done" && styles.done,
                  )}
                  onClick={handleOpenUpgrade}
                >
                  <span className={styles.upgradeDot} aria-hidden="true"></span>
                  <span>{upgradeView.actionLabel}</span>
                </Button>
              ) : null}
            </div>
            {localBuild ? null : (
              <strong className={styles.versionValue}>
                {upgradeView ? upgradeView.versionLabel : formatSidebarVersionLabel(currentVersion)}
              </strong>
            )}
            {upgradeView?.issue ? <div className={styles.versionError}>{upgradeView.issue}</div> : null}
            <a
              className={styles.feedbackLink}
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
