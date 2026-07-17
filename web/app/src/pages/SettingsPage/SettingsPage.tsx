import { useState } from "react";
import type { ReactNode } from "react";
import { ChevronDown, Monitor, Moon, SlidersHorizontal, Sun } from "lucide-react";
import { Button, Select, Tooltip } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import {
  AUTH_ENVIRONMENT_PRESETS,
  authEnvironmentDisplayLabel,
  authEnvironmentDraftFromPreset,
  authEnvironmentDraftFromStatus,
  authEnvironmentLoginReady,
  resolveAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft, AuthEnvironmentPresetID } from "@/models/authEnvironment";
import { githubFeedbackIssueURL } from "@/models/feedback";
import { formatSidebarVersionLabel, hasUpgradeAttention, isLocalBuildUpgradeStatus } from "@/models/upgradeStatus";
import { classNames } from "@/shared/lib/classNames";
import { readStoredAuthEnvironmentDraft, writeStoredAuthEnvironmentDraft } from "@/shared/storage/authEnvironment";
import type { ThemeMode } from "@/shared/theme/theme";
import styles from "./SettingsPage.module.css";

export function SettingsPage() {
  const controller = useWorkspaceControllerContext();
  const [uncontrolledAuthEnvironmentDraft, setUncontrolledAuthEnvironmentDraft] =
    useState<AuthEnvironmentDraft>(readStoredAuthEnvironmentDraft);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const sidebar = controller.sidebarProps;

  if (!controller.ready || !sidebar) {
    return null;
  }

  const authEnvironmentDraft = sidebar.authEnvironment ?? uncontrolledAuthEnvironmentDraft;
  const signedIn = isAuthenticated(sidebar.authStatus);
  const accountName =
    sidebar.authStatus.name ||
    sidebar.authStatus.user_id ||
    sidebar.authStatus.user_uuid ||
    sidebar.t("csghubSignedIn");
  const loginLabel = sidebar.authPending
    ? sidebar.t("csghubLoginPending")
    : sidebar.authBusy
      ? sidebar.t("csghubSigningIn")
      : sidebar.t("settingsAccountLogin");
  const currentVersion = sidebar.upgradeStatus?.current_version || sidebar.appVersion;
  const version = formatSidebarVersionLabel(currentVersion);
  const mockUpgradeAvailable = import.meta.env.DEV && isMockUpgradePreviewEnabled();
  const showUpgradeAction =
    sidebar.showUpgradeControls &&
    (mockUpgradeAvailable ||
      (!isLocalBuildUpgradeStatus(sidebar.upgradeStatus, currentVersion) &&
        sidebar.upgradeStatus?.auto_upgrade_supported !== false &&
        hasUpgradeAttention(sidebar.upgradeStatus, sidebar.upgradePhase, sidebar.upgradeBusy)));
  const showNewVersionBadge = Boolean(
    sidebar.showUpgradeControls &&
      (mockUpgradeAvailable ||
        (!isLocalBuildUpgradeStatus(sidebar.upgradeStatus, currentVersion) && sidebar.upgradeStatus?.update_available)) &&
      sidebar.upgradePhase !== "done",
  );
  const feedbackURL = githubFeedbackIssueURL(sidebar.appVersion, sidebar.upgradeStatus);
  const activeAuthEnvironmentDraft = signedIn
    ? authEnvironmentDraftFromStatus(sidebar.authStatus, authEnvironmentDraft)
    : authEnvironmentDraft;
  const authEnvironmentReady = authEnvironmentLoginReady(authEnvironmentDraft);
  const authActionDisabled = sidebar.authBusy || sidebar.authPending || !authEnvironmentReady;
  const showAuthEnvironmentAdvanced = advancedOpen || authEnvironmentDraft.preset === "custom";
  const authEnvironmentLabel = authEnvironmentDisplayLabel(activeAuthEnvironmentDraft, sidebar.t("csghubEnvCustom"));
  const authEnvironmentOptions = [
    ...AUTH_ENVIRONMENT_PRESETS.map((preset) => ({
      label: preset.label,
      value: preset.id,
    })),
    {
      label: sidebar.t("csghubEnvCustom"),
      value: "custom",
    },
  ];
  const onLogin = sidebar.onLogin;
  const onAuthEnvironmentChange = sidebar.onAuthEnvironmentChange;

  function updateAuthEnvironment(next: AuthEnvironmentDraft) {
    setUncontrolledAuthEnvironmentDraft(next);
    writeStoredAuthEnvironmentDraft(next);
    onAuthEnvironmentChange?.(next);
  }

  function handleAuthEnvironmentPresetChange(preset: AuthEnvironmentPresetID) {
    if (preset === "custom") {
      setAdvancedOpen(true);
      updateAuthEnvironment(
        authEnvironmentDraft.preset === "custom"
          ? authEnvironmentDraft
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
    updateAuthEnvironment(authEnvironmentDraftFromPreset(preset));
  }

  function handleAuthEnvironmentInputChange(value: string) {
    updateAuthEnvironment({
      ...authEnvironmentDraft,
      preset: "custom",
      opencsgBaseURL: value,
      csgHubBaseURL: "",
      aiGatewayBaseURL: "",
    });
  }

  function handleLogin() {
    const next = resolveAuthEnvironmentDraft(authEnvironmentDraft);
    updateAuthEnvironment(next);
    void onLogin(next);
  }

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <div className={styles.headerText}>
          <h1>{sidebar.t("settings")}</h1>
          <p>{sidebar.t("settingsPageSubtitle")}</p>
        </div>
      </header>

      <div className={styles.content}>
        <SettingsRow
          className={styles.rowNoDivider}
          title={sidebar.t("settingsCommunityAccount")}
          description={sidebar.t("settingsCommunityAccountDescription")}
          contentClassName={styles.accountContent}
        >
          <div className={styles.accountStack}>
            <div className={styles.accountActionLine}>
              {signedIn ? (
                <>
                  <span className={styles.accountIdentity}>{accountName}</span>
                  <Button
                    className={styles.designButton}
                    variant="secondaryGray"
                    size="md"
                    disabled={sidebar.authBusy}
                    onClick={() => void sidebar.onLogout()}
                  >
                    {sidebar.t("csghubSignOut")}
                  </Button>
                </>
              ) : (
                <Button
                  className={styles.designButton}
                  variant="secondaryGray"
                  size="md"
                  disabled={authActionDisabled}
                  onClick={handleLogin}
                >
                  {loginLabel}
                </Button>
              )}
            </div>
            {sidebar.authPending ? <p className={styles.statusHint}>{sidebar.t("csghubLoginPendingDetail")}</p> : null}
            {sidebar.authError ? <div className={styles.inlineError}>{sidebar.authError}</div> : null}
          </div>
        </SettingsRow>

        <SettingsRow
          title={sidebar.t("csghubSwitchEnvironment")}
          description={sidebar.t("settingsEnvironmentDescription")}
        >
          <div className={styles.environmentLine}>
            <span className={styles.controlLabel}>{sidebar.t("csghubLoginEnvironment")}</span>
            {signedIn ? (
              <strong className={styles.environmentValue}>{authEnvironmentLabel}</strong>
            ) : (
              <span className={styles.environmentControls}>
                <Select
                  contentClassName={styles.environmentSelectContent}
                  options={authEnvironmentOptions}
                  size="md"
                  triggerClassName={styles.environmentSelectTrigger}
                  value={authEnvironmentDraft.preset}
                  onValueChange={(value) => handleAuthEnvironmentPresetChange(value as AuthEnvironmentPresetID)}
                />
                <Tooltip content={sidebar.t("csghubAdvancedSettings")}>
                  <button
                    type="button"
                    className={classNames(styles.advancedToggle, showAuthEnvironmentAdvanced && styles.open)}
                    aria-label={sidebar.t("csghubAdvancedSettings")}
                    aria-expanded={showAuthEnvironmentAdvanced}
                    onClick={() => setAdvancedOpen((value) => !value)}
                  >
                    <SlidersHorizontal size={16} strokeWidth={2} aria-hidden="true" />
                    <ChevronDown size={14} strokeWidth={2.2} aria-hidden="true" />
                  </button>
                </Tooltip>
              </span>
            )}
          </div>
          {!signedIn && showAuthEnvironmentAdvanced ? (
            <label className={styles.environmentAdvanced}>
              <span>{sidebar.t("csghubOpenCSGBaseURL")}</span>
              <input
                value={authEnvironmentDraft.opencsgBaseURL}
                placeholder="https://openeast.opencsg.com"
                onChange={(event) => handleAuthEnvironmentInputChange(event.currentTarget.value)}
              />
            </label>
          ) : null}
        </SettingsRow>

        <SettingsRow title={sidebar.t("appearanceSettings")} description={sidebar.t("settingsAppearanceDescription")}>
          <div className={styles.stack}>
            <div className={styles.settingLine}>
              <span className={styles.controlLabel}>{sidebar.t("themeSwitcher")}</span>
              <div className={styles.segmented} role="group" aria-label={sidebar.t("themeSwitcher")}>
                <ThemeSegmentButton
                  active={sidebar.theme === "system"}
                  label={sidebar.t("themeSystem")}
                  theme="system"
                  onThemeChange={sidebar.onThemeChange}
                >
                  <Monitor size={20} strokeWidth={2} aria-hidden="true" />
                </ThemeSegmentButton>
                <ThemeSegmentButton
                  active={sidebar.theme === "light"}
                  label={sidebar.t("themeLight")}
                  theme="light"
                  onThemeChange={sidebar.onThemeChange}
                >
                  <Sun size={20} strokeWidth={2} aria-hidden="true" />
                </ThemeSegmentButton>
                <ThemeSegmentButton
                  active={sidebar.theme === "dark"}
                  label={sidebar.t("themeDark")}
                  theme="dark"
                  onThemeChange={sidebar.onThemeChange}
                >
                  <Moon size={20} strokeWidth={2} aria-hidden="true" />
                </ThemeSegmentButton>
              </div>
            </div>
            <div className={styles.settingLine}>
              <span className={styles.controlLabel}>{sidebar.t("languageSwitcher")}</span>
              <div className={styles.segmented} role="group" aria-label={sidebar.t("languageSwitcher")}>
                <button
                  type="button"
                  className={classNames(styles.textSegmentButton, sidebar.locale === "zh" && styles.active)}
                  aria-pressed={sidebar.locale === "zh"}
                  onClick={() => sidebar.onLocaleChange("zh")}
                >
                  {sidebar.t("languageOptionZh")}
                </button>
                <button
                  type="button"
                  className={classNames(styles.textSegmentButton, sidebar.locale === "en" && styles.active)}
                  aria-pressed={sidebar.locale === "en"}
                  onClick={() => sidebar.onLocaleChange("en")}
                >
                  {sidebar.t("languageOptionEn")}
                </button>
              </div>
            </div>
          </div>
        </SettingsRow>

        <SettingsRow
          title={
            <span className={styles.versionTitle}>
              <span>{sidebar.t("versionInfo")}</span>
              {showNewVersionBadge ? (
                <span className={styles.versionBadge} aria-label={sidebar.t("upgradeNewVersionBadge")}>
                  <span aria-hidden="true"></span>
                  {sidebar.t("upgradeNewVersionBadge")}
                </span>
              ) : null}
            </span>
          }
          description={sidebar.t("settingsVersionDescription")}
        >
          <div className={classNames(styles.versionValue, showUpgradeAction && styles.versionValueWithAction)}>
            <span className={styles.versionLabel}>{sidebar.t("settingsCurrentVersion")}</span>
            <strong>{version}</strong>
            {showUpgradeAction ? (
              <Button className={styles.designButton} variant="primary" size="md" onClick={sidebar.onOpenUpgrade}>
                {sidebar.t("upgradeAction")}
              </Button>
            ) : null}
          </div>
        </SettingsRow>

        <SettingsRow title={sidebar.t("configSettingsMenu")} description={sidebar.t("settingsParametersDescription")}>
          <div className={styles.actionLine}>
            <Button
              className={styles.designButton}
              variant="secondaryGray"
              size="md"
              onClick={sidebar.onOpenConfigSettings}
            >
              {sidebar.t("settingsEditParameters")}
            </Button>
          </div>
        </SettingsRow>

        <SettingsRow
          title={sidebar.t("configSettingsFeedbackSection")}
          description={sidebar.t("settingsFeedbackDescription")}
        >
          <div className={styles.actionLine}>
            <a className={styles.linkButton} href={feedbackURL} target="_blank" rel="noreferrer">
              {sidebar.t("settingsFeedbackGithubAction")}
            </a>
          </div>
        </SettingsRow>
      </div>
    </section>
  );
}

function isMockUpgradePreviewEnabled(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  if (new URLSearchParams(window.location.search).get("mockUpgrade") === "1") {
    return true;
  }
  const hashQuery = window.location.hash.split("?")[1] || "";
  return new URLSearchParams(hashQuery).get("mockUpgrade") === "1";
}

function SettingsRow({
  children,
  className,
  contentClassName,
  description,
  title,
}: {
  children: ReactNode;
  className?: string;
  contentClassName?: string;
  description: string;
  title: ReactNode;
}) {
  return (
    <section className={classNames(styles.row, className)}>
      <div className={styles.rowIntro}>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
      <div className={classNames(styles.rowContent, contentClassName)}>{children}</div>
    </section>
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
