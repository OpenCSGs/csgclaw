import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Monitor, Moon, Sun } from "lucide-react";
import { Button, Select, Tooltip } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import {
  authEnvironmentDisplayLabel,
  authEnvironmentDraftFromStatus,
  defaultAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import { githubFeedbackIssueURL } from "@/models/feedback";
import {
  formatSidebarVersionLabel,
  hasUpgradeAttention,
  inferUpgradeChannelFromVersion,
  isLocalBuildUpgradeStatus,
  upgradeErrorMessage,
} from "@/models/upgradeStatus";
import { nextSimulatedUpgradeProgress } from "@/models/upgradeProgress";
import { classNames } from "@/shared/lib/classNames";
import {
  DEFAULT_TURN_NOTIFICATION_MODE,
  normalizeTurnNotificationMode,
  TurnNotificationModes,
} from "@/models/turnNotifications";
import { readStoredAuthEnvironmentDraft, writeStoredAuthEnvironmentDraft } from "@/shared/storage/authEnvironment";
import type { ThemeMode } from "@/shared/theme/theme";
import { OpenCSGConnectionDialog, OpenCSGSwitchDialog } from "./components/OpenCSGConnectionDialog";
import { SwitchVersionDialog } from "./components/SwitchVersionDialog";
import styles from "./SettingsPage.module.css";

export function SettingsPage() {
  const controller = useWorkspaceControllerContext();
  const [uncontrolledAuthEnvironmentDraft, setUncontrolledAuthEnvironmentDraft] =
    useState<AuthEnvironmentDraft>(readStoredAuthEnvironmentDraft);
  const [connectionDraft, setConnectionDraft] = useState<AuthEnvironmentDraft>(defaultAuthEnvironmentDraft);
  const [connectionOpen, setConnectionOpen] = useState(false);
  const [switchOpen, setSwitchOpen] = useState(false);
  const [switchVersionOpen, setSwitchVersionOpen] = useState(false);
  const sidebar = controller.sidebarProps;
  const refreshUpgradeStatus = sidebar?.onRefreshUpgradeStatus;
  const showUpgradeControls = Boolean(sidebar?.showUpgradeControls);
  const progressActive = Boolean(
    sidebar && (sidebar.upgradeChannelBusy || sidebar.upgradeStatus?.checking || sidebar.upgradeStatus?.upgrading),
  );
  const upgradeProgress = useSimulatedUpgradeProgress(progressActive, Boolean(sidebar?.upgradeStatus?.downloaded));

  useEffect(() => {
    if (!controller.ready || !showUpgradeControls || !refreshUpgradeStatus) {
      return;
    }
    void refreshUpgradeStatus();
  }, [controller.ready, refreshUpgradeStatus, showUpgradeControls]);

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
  const currentVersion = sidebar.upgradeStatus?.current_version || sidebar.appVersion;
  const version = formatSidebarVersionLabel(currentVersion);
  const localBuild = isLocalBuildUpgradeStatus(sidebar.upgradeStatus, currentVersion);
  const mockUpgradeAvailable = import.meta.env.DEV && isMockUpgradePreviewEnabled();
  const showUpgradeAction =
    sidebar.showUpgradeControls &&
    (mockUpgradeAvailable ||
      (!localBuild &&
        sidebar.upgradeStatus?.auto_upgrade_supported !== false &&
        hasUpgradeAttention(sidebar.upgradeStatus, sidebar.upgradePhase, sidebar.upgradeBusy)));
  const showUpgradeChannel = sidebar.showUpgradeControls;
  const inferredUpgradeChannel = inferUpgradeChannelFromVersion(currentVersion);
  const upgradeChannelDisabled = Boolean(
    sidebar.upgradeChannelBusy ||
    sidebar.upgradeBusy ||
    sidebar.upgradeStatus?.checking ||
    sidebar.upgradeStatus?.upgrading,
  );
  const showUpgradeProgress = upgradeProgress.visible;
  const versionError = sidebar.upgradeError || upgradeErrorMessage(sidebar.upgradeStatus, sidebar.t);
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
  const authEnvironmentLabel = authEnvironmentDisplayLabel(activeAuthEnvironmentDraft, sidebar.t("csghubEnvCustom"));
  const onLogin = sidebar.onLogin;
  const onLogout = sidebar.onLogout;
  const onAuthEnvironmentChange = sidebar.onAuthEnvironmentChange;
  const turnNotificationMode = sidebar.turnNotificationMode ?? DEFAULT_TURN_NOTIFICATION_MODE;
  const turnNotificationPermission = sidebar.turnNotificationPermission ?? "unsupported";
  const turnNotificationOptions = [
    { label: sidebar.t("turnNotificationModeOff"), value: TurnNotificationModes.off },
    { label: sidebar.t("turnNotificationModeAlways"), value: TurnNotificationModes.always },
    { label: sidebar.t("turnNotificationModeWhenUnfocused"), value: TurnNotificationModes.whenUnfocused },
  ];
  const selectedTurnNotificationLabel =
    turnNotificationOptions.find((option) => option.value === turnNotificationMode)?.label ??
    sidebar.t("turnNotificationModeWhenUnfocused");

  function updateAuthEnvironment(next: AuthEnvironmentDraft) {
    setUncontrolledAuthEnvironmentDraft(next);
    writeStoredAuthEnvironmentDraft(next);
    onAuthEnvironmentChange?.(next);
  }

  function openConnectionDialog(draft = authEnvironmentDraft) {
    setConnectionDraft(draft);
    setConnectionOpen(true);
  }

  function handleLogin() {
    updateAuthEnvironment(connectionDraft);
    setConnectionOpen(false);
    void onLogin(connectionDraft);
  }

  async function handleSwitchEnvironment() {
    await onLogout();
    setSwitchOpen(false);
    openConnectionDialog(activeAuthEnvironmentDraft);
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
        <SettingsRow title={sidebar.t("settingsOpenCSGAccount")} description={sidebar.t("settingsOpenCSGDescription")}>
          <div className={styles.accountStack}>
            <div className={styles.accountActionLine}>
              {signedIn ? (
                <>
                  <span className={styles.accountSummary}>
                    <strong className={styles.accountIdentity}>{accountName}</strong>
                    <span>{authEnvironmentLabel}</span>
                  </span>
                  <Button
                    className={styles.designButton}
                    variant="secondaryGray"
                    size="md"
                    disabled={sidebar.authBusy}
                    onClick={() => setSwitchOpen(true)}
                  >
                    {sidebar.t("csghubSwitchEnvironment")}
                  </Button>
                  <Button
                    className={styles.designButton}
                    variant="secondaryGray"
                    size="md"
                    disabled={sidebar.authBusy}
                    loading={sidebar.authLoggingOut}
                    loadingLabel={sidebar.t("csghubSigningOut")}
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
                  disabled={sidebar.authBusy}
                  loading={sidebar.authBusy}
                  loadingLabel={sidebar.authLoggingOut ? sidebar.t("csghubSigningOut") : sidebar.t("csghubSigningIn")}
                  onClick={() => openConnectionDialog()}
                >
                  {sidebar.authLoggingOut
                    ? sidebar.t("csghubSigningOut")
                    : sidebar.authPending
                      ? sidebar.t("csghubReauthorize")
                      : sidebar.t("settingsAccountLogin")}
                </Button>
              )}
            </div>
            {sidebar.authPending ? <p className={styles.statusHint}>{sidebar.t("csghubLoginPendingDetail")}</p> : null}
            {sidebar.authError ? <div className={styles.inlineError}>{sidebar.authError}</div> : null}
          </div>
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
          title={sidebar.t("notificationSettings")}
          description={sidebar.t("settingsNotificationDescription")}
        >
          <div className={styles.stack}>
            <div className={styles.settingLine}>
              <span className={styles.controlLabel}>{sidebar.t("turnCompletionNotifications")}</span>
              <Select
                contentClassName={styles.notificationSelectContent}
                options={turnNotificationOptions}
                selectedLabel={selectedTurnNotificationLabel}
                triggerClassName={styles.notificationSelectTrigger}
                triggerProps={{ "aria-label": sidebar.t("turnCompletionNotifications") }}
                value={turnNotificationMode}
                onValueChange={(value) =>
                  void sidebar.onTurnNotificationModeChange?.(normalizeTurnNotificationMode(value))
                }
              />
            </div>
            <div className={styles.settingLine}>
              <span className={styles.controlLabel}>{sidebar.t("notificationPermission")}</span>
              <div className={styles.notificationPermissionControls}>
                <span
                  className={classNames(
                    styles.permissionStatus,
                    turnNotificationPermission === "granted" && styles.permissionGranted,
                    turnNotificationPermission === "denied" && styles.permissionDenied,
                  )}
                >
                  <span aria-hidden="true" />
                  {sidebar.t(`notificationPermission${permissionLabelSuffix(turnNotificationPermission)}`)}
                </span>
                {turnNotificationPermission === "default" ? (
                  <Button
                    className={styles.designButton}
                    variant="secondaryGray"
                    size="md"
                    onClick={() => void sidebar.onRequestTurnNotificationPermission?.()}
                  >
                    {sidebar.t("notificationPermissionEnable")}
                  </Button>
                ) : null}
              </div>
            </div>
            {turnNotificationPermission === "denied" ? (
              <p className={styles.notificationPermissionHint}>{sidebar.t("notificationPermissionDeniedHint")}</p>
            ) : null}
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
          <div className={classNames(styles.stack, styles.versionStack)}>
            <div className={styles.versionValue}>
              <span className={styles.versionLabel}>{sidebar.t("settingsCurrentVersion")}</span>
              <div className={styles.versionMeta}>
                <strong>{version}</strong>
                {showUpgradeAction ? (
                  <Button
                    className={styles.designButton}
                    variant="primary"
                    size="md"
                    disabled={sidebar.upgradeBusy || sidebar.upgradeStatus?.upgrading}
                    onClick={sidebar.onOpenUpgrade}
                  >
                    {sidebar.t(sidebar.upgradeStatus?.downloaded ? "upgradeAction" : "upgradeDownloadAction")}
                  </Button>
                ) : null}
              </div>
            </div>
            {showUpgradeChannel ? (
              <button
                type="button"
                className={styles.versionChannelLink}
                disabled={upgradeChannelDisabled}
                onClick={() => setSwitchVersionOpen(true)}
              >
                {sidebar.t("upgradeChannelSwitch")}
              </button>
            ) : null}
            {showUpgradeChannel && showUpgradeProgress ? (
              <div
                className={styles.upgradeProgressRow}
                role="progressbar"
                aria-label={sidebar.t("upgradeProgressLabel")}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={upgradeProgress.percent}
              >
                <div className={styles.upgradeProgressTrack} aria-hidden="true">
                  <span style={{ width: `${upgradeProgress.percent}%` }}></span>
                </div>
                <span className={styles.upgradeProgressValue}>{upgradeProgress.percent}%</span>
              </div>
            ) : null}
            {versionError ? (
              <p className={styles.inlineError} role="alert">
                {versionError}
              </p>
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

      <OpenCSGConnectionDialog
        busy={sidebar.authBusy}
        draft={connectionDraft}
        open={connectionOpen}
        t={sidebar.t}
        onConnect={handleLogin}
        onDraftChange={setConnectionDraft}
        onOpenChange={setConnectionOpen}
      />
      <OpenCSGSwitchDialog
        accountName={accountName}
        busy={sidebar.authBusy}
        environmentLabel={authEnvironmentLabel}
        open={switchOpen}
        t={sidebar.t}
        onConfirm={() => void handleSwitchEnvironment()}
        onOpenChange={setSwitchOpen}
      />
      <SwitchVersionDialog
        busy={upgradeChannelDisabled}
        currentChannel={inferredUpgradeChannel}
        error={sidebar.upgradeChannelError}
        open={switchVersionOpen}
        t={sidebar.t}
        onConfirm={sidebar.onUpgradeChannelChange}
        onOpenChange={setSwitchVersionOpen}
      />
    </section>
  );
}

function useSimulatedUpgradeProgress(active: boolean, completed: boolean) {
  const [percent, setPercent] = useState(0);
  const [visible, setVisible] = useState(false);
  const activeRunRef = useRef(false);

  useEffect(() => {
    if (active) {
      activeRunRef.current = true;
      setVisible(true);
      setPercent((current) => (current > 0 && current < 100 ? current : 3));
      const timer = window.setInterval(() => {
        setPercent((current) => nextSimulatedUpgradeProgress(current));
      }, 1000);
      return () => window.clearInterval(timer);
    }
    if (completed && activeRunRef.current) {
      setPercent(100);
      setVisible(true);
      const timer = window.setTimeout(() => {
        activeRunRef.current = false;
        setVisible(false);
      }, 600);
      return () => window.clearTimeout(timer);
    }
    activeRunRef.current = false;
    setPercent(0);
    setVisible(false);
    return undefined;
  }, [active, completed]);

  return { percent, visible };
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

function permissionLabelSuffix(permission: "default" | "denied" | "granted" | "unsupported"): string {
  switch (permission) {
    case "granted":
      return "Granted";
    case "denied":
      return "Denied";
    case "default":
      return "Default";
    default:
      return "Unsupported";
  }
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
