import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@/components/ui";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import type { WorkspaceController } from "@/hooks/workspace";
import { emptyAuthStatus } from "@/models/auth";
import type { TranslateFn } from "@/models/conversations";
import { SettingsPage } from "@/pages/SettingsPage/SettingsPage";
import { TurnNotificationModes } from "@/models/turnNotifications";

const labels: Record<string, string> = {
  cancel: "Cancel",
  close: "Close",
  confirm: "Confirm",
  settings: "Settings",
  settingsAccountLogin: "Log in",
  csghubReauthorize: "Authorize again",
  csghubSigningOut: "Signing out...",
  settingsAppearanceDescription: "Appearance settings.",
  settingsCommunityAccount: "Community account",
  settingsCommunityAccountDescription: "Manage your community account.",
  settingsCurrentVersion: "Current version",
  settingsEnvironmentDescription: "Choose a site.",
  settingsFeedbackDescription: "Send feedback.",
  settingsNotificationDescription: "Notification settings.",
  settingsPageSubtitle: "Manage product settings.",
  settingsParametersDescription: "Configure parameters.",
  settingsVersionDescription: "View the current version and update status.",
  notificationPermission: "System notification permission",
  notificationPermissionDefault: "Not yet allowed",
  notificationPermissionEnable: "Enable notifications",
  notificationSettings: "Notifications",
  turnCompletionNotifications: "Turn completion notifications",
  turnNotificationModeAlways: "Always notify",
  turnNotificationModeOff: "Off",
  turnNotificationModeWhenUnfocused: "Only when the app is unfocused",
  upgradeAction: "Update & Restart",
  upgradeDownloadAction: "Update",
  upgradeChannel: "Update channel",
  upgradeChannelBeta: "Preview",
  upgradeChannelCurrentActive: "Current Channel Is Active",
  upgradeChannelInstallBeta: "Switch to Preview and Install",
  upgradeChannelInstallRelease: "Switch to Stable and Install",
  upgradeChannelRelease: "Stable",
  upgradeChannelRetryCurrent: "Check Current Channel Again",
  upgradeChannelSwitch: "Switch channel",
  upgradeChannelSwitchDescription: "Choose a version channel.",
  upgradeChannelSwitchTitle: "Switch channel",
  upgradeLocalBuildUnsupported:
    "This is a local development build. In-app upgrades and channel switching are unavailable.",
  upgradeProgressLabel: "Checking and preparing the update",
};

const t: TranslateFn = (key) => labels[key] ?? key;

function renderSettings(controller: WorkspaceController) {
  return render(
    <TooltipProvider delayDuration={0}>
      <WorkspaceControllerProvider controller={controller}>
        <SettingsPage />
      </WorkspaceControllerProvider>
    </TooltipProvider>,
  );
}

describe("SettingsPage", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("checks the configured update channel when the settings page opens", () => {
    const onRefreshUpgradeStatus = vi.fn().mockResolvedValue(null);
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "v0.5.0-beta.7",
        authBusy: false,
        authError: "",
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onRefreshUpgradeStatus,
        onThemeChange: vi.fn(),
        onUpgradeChannelChange: vi.fn(),
        showUpgradeControls: true,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeChannelError: "",
        upgradeError: "",
        upgradePhase: "idle",
        upgradeStatus: null,
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    expect(onRefreshUpgradeStatus).toHaveBeenCalledTimes(1);
  });

  it("allows OpenCSG authorization to be started again while a login is pending", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "v0.5.0-beta.7",
        authBusy: false,
        authError: "",
        authPending: true,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin,
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onThemeChange: vi.fn(),
        onUpgradeChannelChange: vi.fn(),
        showUpgradeControls: false,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeChannelError: "",
        upgradeError: "",
        upgradePhase: "idle",
        upgradeStatus: null,
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    const retryButton = screen.getByRole("button", { name: "Authorize again" });
    expect(retryButton).toBeEnabled();
    await user.click(retryButton);
    await user.click(screen.getByRole("button", { name: "csghubConnectContinue" }));
    expect(onLogin).toHaveBeenCalledTimes(1);
  });

  it("keeps a loading state visible while logout cleanup is still running", () => {
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "v0.5.0-beta.7",
        authBusy: true,
        authError: "",
        authLoggingOut: true,
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onThemeChange: vi.fn(),
        onUpgradeChannelChange: vi.fn(),
        showUpgradeControls: false,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeChannelError: "",
        upgradeError: "",
        upgradePhase: "idle",
        upgradeStatus: null,
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    const logoutProgress = screen.getByRole("button", { name: "Signing out..." });
    expect(logoutProgress).toBeDisabled();
    expect(logoutProgress).toHaveAttribute("aria-busy", "true");
  });

  it("opens the upgrade flow when an update is available", () => {
    const onOpenUpgrade = vi.fn();
    const onRequestTurnNotificationPermission = vi.fn();
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "0.0.101",
        authBusy: false,
        authError: "",
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade,
        onUpgradeChannelChange: vi.fn(),
        onThemeChange: vi.fn(),
        onRequestTurnNotificationPermission,
        onTurnNotificationModeChange: vi.fn(),
        showUpgradeControls: true,
        t,
        theme: "light",
        turnNotificationMode: TurnNotificationModes.whenUnfocused,
        turnNotificationPermission: "default",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeChannelError: "",
        upgradeError: "switch failed",
        upgradePhase: "idle",
        upgradeStatus: {
          auto_upgrade_supported: true,
          auto_upgrade_unsupported_reason: "",
          channel: "release",
          checking: false,
          current_version: "0.0.101",
          downloaded: false,
          last_checked_at: "",
          last_error: "",
          last_error_kind: "",
          last_error_log_path: "",
          latest_version: "v0.3.18",
          manual_restart_required: false,
          update_available: true,
          upgrading: false,
        },
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    fireEvent.click(screen.getByRole("button", { name: "Enable notifications" }));

    expect(onOpenUpgrade).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("alert")).toHaveTextContent("switch failed");
    expect(screen.getByRole("combobox", { name: "Turn completion notifications" })).toHaveTextContent(
      "Only when the app is unfocused",
    );
    expect(onRequestTurnNotificationPermission).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Switch channel" })).toBeEnabled();
  });

  it("keeps version switching available when an update is already downloaded", async () => {
    const user = userEvent.setup();
    const onUpgradeChannelChange = vi.fn().mockResolvedValue(true);
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "v0.2.0-beta.1",
        authBusy: false,
        authError: "",
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onThemeChange: vi.fn(),
        onUpgradeChannelChange,
        showUpgradeControls: true,
        t,
        theme: "dark",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeError: "",
        upgradePhase: "idle",
        upgradeStatus: {
          auto_upgrade_supported: true,
          auto_upgrade_unsupported_reason: "",
          channel: "beta",
          checking: false,
          current_version: "v0.2.0-beta.1",
          downloaded: true,
          last_checked_at: "",
          last_error: "",
          last_error_kind: "",
          last_error_log_path: "",
          latest_version: "v0.2.0-beta.2",
          manual_restart_required: false,
          update_available: true,
          upgrading: false,
        },
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    const versionLabel = screen.getByText("v0.2.0-beta.1");
    const switchButton = screen.getByRole("button", { name: "Switch channel" });
    const updateButton = screen.getByRole("button", { name: "Update & Restart" });
    expect(switchButton).toBeEnabled();
    expect(updateButton).toBeInTheDocument();
    expect(versionLabel.compareDocumentPosition(updateButton) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(updateButton.compareDocumentPosition(switchButton) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByText("Update channel")).not.toBeInTheDocument();
    await user.click(switchButton);
    expect(screen.getByRole("radio", { name: "Preview" })).toBeChecked();
  });

  it("opens a version switch dialog inferred from the current version and confirms the other channel", async () => {
    const user = userEvent.setup();
    const onUpgradeChannelChange = vi.fn().mockResolvedValue(true);
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "v0.5.0-beta.6",
        authBusy: false,
        authError: "",
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onThemeChange: vi.fn(),
        onUpgradeChannelChange,
        showUpgradeControls: true,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeError: "",
        upgradePhase: "idle",
        upgradeStatus: {
          auto_upgrade_supported: true,
          auto_upgrade_unsupported_reason: "",
          channel: "beta",
          checking: false,
          current_version: "v0.5.0-beta.6",
          downloaded: false,
          last_checked_at: "",
          last_error: "",
          last_error_kind: "",
          last_error_log_path: "",
          latest_version: "",
          manual_restart_required: false,
          update_available: false,
          upgrading: false,
        },
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    expect(screen.queryByRole("button", { name: "Update & Restart" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Switch channel" }));

    expect(screen.getByRole("radio", { name: "Preview" })).toBeChecked();
    expect(screen.getByRole("button", { name: "Current Channel Is Active" })).toBeDisabled();

    await user.click(screen.getByRole("radio", { name: "Stable" }));
    await user.click(screen.getByRole("button", { name: "Switch to Stable and Install" }));

    expect(onUpgradeChannelChange).toHaveBeenCalledWith("release");
  });

  it("allows opening channel switching without showing a local-build warning in advance", async () => {
    const user = userEvent.setup();
    const onUpgradeChannelChange = vi.fn().mockResolvedValue(false);
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "v0.5.0-beta.6-5-ge6a80cb1-dirty+local",
        authBusy: false,
        authError: "",
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onThemeChange: vi.fn(),
        onUpgradeChannelChange,
        showUpgradeControls: true,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeChannelError:
          "This is a local development build. In-app upgrades and channel switching are unavailable.",
        upgradeError: "",
        upgradePhase: "idle",
        upgradeStatus: {
          auto_upgrade_supported: false,
          auto_upgrade_unsupported_reason: "local_build",
          channel: "beta",
          checking: false,
          current_version: "v0.5.0-beta.6-5-ge6a80cb1-dirty+local",
          downloaded: false,
          last_checked_at: "",
          last_error: "",
          last_error_kind: "",
          last_error_log_path: "",
          latest_version: "v0.5.0-beta.10",
          manual_restart_required: false,
          update_available: false,
          upgrading: false,
        },
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    expect(screen.getByRole("button", { name: "Switch channel" })).toBeEnabled();
    expect(
      screen.queryByText("This is a local development build. In-app upgrades and channel switching are unavailable."),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Switch channel" }));

    expect(screen.getByRole("dialog", { name: "Switch channel" })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Preview" })).toBeChecked();
    expect(
      screen.queryByText("This is a local development build. In-app upgrades and channel switching are unavailable."),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Stable" }));
    await user.click(screen.getByRole("button", { name: "Switch to Stable and Install" }));

    expect(onUpgradeChannelChange).toHaveBeenCalledWith("release");
    expect(screen.getByRole("dialog", { name: "Switch channel" })).toHaveTextContent(
      "This is a local development build. In-app upgrades and channel switching are unavailable.",
    );
  });

  it("advances simulated progress while switching channels or downloading", () => {
    vi.useFakeTimers();
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "0.5.0-beta.2",
        authBusy: false,
        authError: "",
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onThemeChange: vi.fn(),
        onUpgradeChannelChange: vi.fn(),
        showUpgradeControls: true,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeError: "",
        upgradePhase: "idle",
        upgradeStatus: {
          auto_upgrade_supported: true,
          auto_upgrade_unsupported_reason: "",
          channel: "beta",
          checking: true,
          current_version: "0.5.0-beta.2",
          downloaded: false,
          last_checked_at: "",
          last_error: "",
          last_error_kind: "",
          last_error_log_path: "",
          latest_version: "0.5.0-beta.3",
          manual_restart_required: false,
          update_available: false,
          upgrading: false,
        },
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    const progress = screen.getByRole("progressbar", { name: "Checking and preparing the update" });
    expect(progress).toHaveAttribute("aria-valuenow", "3");
    act(() => vi.advanceTimersByTime(1000));
    expect(progress).toHaveAttribute("aria-valuenow", "10");
    expect(screen.getByRole("button", { name: "Switch channel" })).toBeDisabled();
  });

  it("shows a signed-package hint when an update cannot be applied", () => {
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "0.2.1-beta.1",
        authBusy: false,
        authError: "",
        authPending: false,
        authStatus: emptyAuthStatus(),
        locale: "en",
        onLocaleChange: vi.fn(),
        onLogin: vi.fn(),
        onLogout: vi.fn(),
        onOpenConfigSettings: vi.fn(),
        onOpenUpgrade: vi.fn(),
        onThemeChange: vi.fn(),
        onUpgradeChannelChange: vi.fn(),
        showUpgradeControls: true,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeError: "",
        upgradePhase: "error",
        upgradeStatus: {
          auto_upgrade_supported: true,
          auto_upgrade_unsupported_reason: "",
          channel: "release",
          checking: false,
          current_version: "0.2.1-beta.1",
          downloaded: false,
          last_checked_at: "",
          last_error: "The release channel does not provide a signed macOS auto-update package for 0.4.6. HTTP 404",
          last_error_kind: "missing_update_package",
          last_error_log_path: "",
          latest_version: "0.4.6",
          manual_restart_required: false,
          update_available: false,
          upgrading: false,
        },
      },
    } as unknown as WorkspaceController;

    renderSettings(controller);

    expect(screen.getByRole("alert")).toHaveTextContent("upgradeErrorMissingUpdatePackage");
    expect(screen.queryByRole("button", { name: "Update" })).not.toBeInTheDocument();
  });
});
