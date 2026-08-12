import { act, fireEvent, render, screen } from "@testing-library/react";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import type { WorkspaceController } from "@/hooks/workspace";
import { emptyAuthStatus } from "@/models/auth";
import type { TranslateFn } from "@/models/conversations";
import { SettingsPage } from "@/pages/SettingsPage/SettingsPage";

const labels: Record<string, string> = {
  settings: "Settings",
  settingsAccountLogin: "Log in",
  settingsAppearanceDescription: "Appearance settings.",
  settingsCommunityAccount: "Community account",
  settingsCommunityAccountDescription: "Manage your community account.",
  settingsCurrentVersion: "Current version",
  settingsEnvironmentDescription: "Choose a site.",
  settingsFeedbackDescription: "Send feedback.",
  settingsPageSubtitle: "Manage product settings.",
  settingsParametersDescription: "Configure parameters.",
  settingsVersionDescription: "View the current version and update status.",
  upgradeAction: "Update & Restart",
  upgradeDownloadAction: "Update",
  upgradeChannel: "Update channel",
  upgradeChannelBeta: "Preview",
  upgradeChannelRelease: "Stable",
  upgradeProgressLabel: "Checking and preparing the update",
};

const t: TranslateFn = (key) => labels[key] ?? key;

describe("SettingsPage", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("opens the upgrade flow when an update is available", () => {
    const onOpenUpgrade = vi.fn();
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
        showUpgradeControls: true,
        t,
        theme: "light",
        upgradeBusy: false,
        upgradeChannelBusy: false,
        upgradeChannelLocked: false,
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

    render(
      <WorkspaceControllerProvider controller={controller}>
        <SettingsPage />
      </WorkspaceControllerProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Update" }));

    expect(onOpenUpgrade).toHaveBeenCalledTimes(1);
  });

  it("shows channel selection for a development desktop build and persists preview selection", () => {
    const onUpgradeChannelChange = vi.fn().mockResolvedValue(undefined);
    const controller = {
      ready: true,
      sidebarProps: {
        appVersion: "v0.4.6-dev2g6c80d157dirty",
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
        upgradeChannelLocked: false,
        upgradePhase: "idle",
        upgradeStatus: {
          auto_upgrade_supported: false,
          auto_upgrade_unsupported_reason: "desktop_update_unsupported",
          channel: "release",
          checking: false,
          current_version: "v0.4.6-dev2g6c80d157dirty",
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

    render(
      <WorkspaceControllerProvider controller={controller}>
        <SettingsPage />
      </WorkspaceControllerProvider>,
    );

    expect(screen.queryByRole("button", { name: "Update & Restart" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    expect(onUpgradeChannelChange).toHaveBeenCalledWith("beta");
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
        upgradeChannelLocked: false,
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

    render(
      <WorkspaceControllerProvider controller={controller}>
        <SettingsPage />
      </WorkspaceControllerProvider>,
    );

    const progress = screen.getByRole("progressbar", { name: "Checking and preparing the update" });
    expect(progress).toHaveAttribute("aria-valuenow", "3");
    act(() => vi.advanceTimersByTime(1000));
    expect(progress).toHaveAttribute("aria-valuenow", "10");
    expect(screen.getByRole("button", { name: "Stable" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled();
  });
});
