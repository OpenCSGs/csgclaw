import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SidebarUserButton } from "@/pages/WorkspacePage/components/WorkspaceSidebar/SidebarUserButton";
import type { UpgradeStatus } from "@/models/upgradeStatus";

const labels: Record<string, string> = {
  appearanceSettings: "Appearance",
  languageSwitcher: "Language",
  settings: "Settings",
  themeDark: "Dark",
  themeLight: "Light",
  themeSwitcher: "Theme",
  upgradeAction: "Update & Restart",
  upgradeLocalBuild: "Local build",
  versionInfo: "Version",
  versionSettings: "Version and updates",
  configSettingsMenu: "Settings",
  csghubAccount: "CSGHub account",
  csghubLoginPending: "Waiting for auth",
  csghubLoginPendingDetail: "Finish authorization",
  csghubLoginRequired: "Sign in with CSGHub",
  csghubNotSignedIn: "Not signed in",
  csghubSignIn: "Sign in",
  csghubSigningIn: "Signing in...",
  csghubSignedIn: "Signed in",
  csghubSignOut: "Sign out",
};

function t(key: string): string {
  return labels[key] ?? key;
}

const updateAvailableStatus: UpgradeStatus = {
  auto_upgrade_supported: true,
  auto_upgrade_unsupported_reason: "",
  checking: false,
  current_version: "v0.3.0",
  last_checked_at: "",
  last_error: "",
  latest_version: "v0.3.1",
  manual_restart_required: false,
  update_available: true,
  upgrading: false,
};

describe("SidebarUserButton", () => {
  it("keeps the current version visible when upgrade controls are hidden", async () => {
    const user = userEvent.setup();
    const onOpenUpgrade = vi.fn();

    render(
      <SidebarUserButton
        appVersion="v0.3.0"
        showUpgradeControls={false}
        locale="en"
        onOpenUpgrade={onOpenUpgrade}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        t={t}
        theme="light"
        upgradeStatus={updateAvailableStatus}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("v0.3.0")).toBeInTheDocument();
    expect(screen.getByText("Version")).toBeInTheDocument();
    expect(screen.queryByText("Version and updates")).not.toBeInTheDocument();
    expect(screen.queryByText("Update & Restart")).not.toBeInTheDocument();
    expect(onOpenUpgrade).not.toHaveBeenCalled();
  });

  it("shows a compact version summary when updates are available", async () => {
    const user = userEvent.setup();
    const onOpenUpgrade = vi.fn();

    render(
      <SidebarUserButton
        appVersion="v0.3.0"
        showUpgradeControls={true}
        locale="en"
        onOpenUpgrade={onOpenUpgrade}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        t={t}
        theme="light"
        upgradeStatus={updateAvailableStatus}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("v0.3.0 -> v0.3.1")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Update & Restart" }));
    expect(onOpenUpgrade).toHaveBeenCalledTimes(1);
  });

  it("hides upgrade actions for non-official installs", async () => {
    const user = userEvent.setup();
    const onOpenUpgrade = vi.fn();

    render(
      <SidebarUserButton
        appVersion="v0.3.0"
        showUpgradeControls={true}
        locale="en"
        onOpenUpgrade={onOpenUpgrade}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        t={t}
        theme="light"
        upgradeStatus={{
          ...updateAvailableStatus,
          auto_upgrade_supported: false,
          auto_upgrade_unsupported_reason: "not_official_bundle",
        }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.queryByText("Local build")).not.toBeInTheDocument();
    expect(screen.getByText("v0.3.0")).toBeInTheDocument();
    expect(screen.queryByText("v0.3.0 -> v0.3.1")).not.toBeInTheDocument();
    expect(screen.queryByText("Update & Restart")).not.toBeInTheDocument();
    expect(screen.queryByText("Manual upgrade")).not.toBeInTheDocument();
    expect(onOpenUpgrade).not.toHaveBeenCalled();
  });

  it("shows local build state without upgrade actions", async () => {
    const user = userEvent.setup();

    render(
      <SidebarUserButton
        appVersion="v0.3.5+local"
        showUpgradeControls={true}
        locale="en"
        onOpenUpgrade={() => {}}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        t={t}
        theme="light"
        upgradeStatus={{
          ...updateAvailableStatus,
          current_version: "v0.3.5+local",
          latest_version: "",
          update_available: false,
        }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("Local build")).toBeInTheDocument();
    expect(screen.queryByText("v0.3.5+local")).not.toBeInTheDocument();
    expect(screen.queryByText("Update & Restart")).not.toBeInTheDocument();
  });

  it("treats dirty git describe versions as local builds", async () => {
    const user = userEvent.setup();

    render(
      <SidebarUserButton
        appVersion="v0.3.10-21-g4dd4395-dirty"
        showUpgradeControls={true}
        locale="en"
        onOpenUpgrade={() => {}}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        t={t}
        theme="light"
        upgradeStatus={{
          ...updateAvailableStatus,
          current_version: "v0.3.10-21-g4dd4395-dirty",
          latest_version: "v0.3.11",
          update_available: true,
        }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("Local build")).toBeInTheDocument();
    expect(screen.queryByText("v0.3.10-21-g4dd4395-dirty")).not.toBeInTheDocument();
    expect(screen.queryByText("v0.3.10-21-g4dd4395-dirty -> v0.3.11")).not.toBeInTheDocument();
    expect(screen.queryByText("Update & Restart")).not.toBeInTheDocument();
  });

  it("opens config settings from the settings menu", async () => {
    const user = userEvent.setup();
    const onOpenConfigSettings = vi.fn();

    render(
      <SidebarUserButton
        appVersion="v0.3.0"
        showUpgradeControls={false}
        locale="en"
        onOpenUpgrade={() => {}}
        onOpenConfigSettings={onOpenConfigSettings}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        t={t}
        theme="light"
        upgradeStatus={updateAvailableStatus}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByRole("menuitem", { name: "Settings" }));
    expect(onOpenConfigSettings).toHaveBeenCalledTimes(1);
  });

  it("shows CSGHub sign-in when no account is connected", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();

    render(
      <SidebarUserButton
        appVersion="v0.3.0"
        showUpgradeControls={false}
        locale="en"
        onOpenUpgrade={() => {}}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        onLogin={onLogin}
        t={t}
        theme="light"
        upgradeStatus={updateAvailableStatus}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("CSGHub")).toBeInTheDocument();
    expect(screen.getAllByText("Not signed in")).toHaveLength(2);
    expect(screen.queryByText("Sign in with CSGHub")).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Sign in" }));
    expect(onLogin).toHaveBeenCalledTimes(1);
  });

  it("shows CSGHub account metadata and signs out", async () => {
    const user = userEvent.setup();
    const onLogout = vi.fn();

    render(
      <SidebarUserButton
        appVersion="v0.3.0"
        showUpgradeControls={false}
        locale="en"
        onOpenUpgrade={() => {}}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        onLogout={onLogout}
        authStatus={{
          authenticated: true,
          user_id: "alice",
          user_uuid: "user-1",
          avatar: "https://example.test/avatar.png",
          base_url: "https://hub.example.test",
          portal_url: "https://hub.example.test/portal",
          logged_in_at: "2026-06-22T09:00:00Z",
        }}
        t={t}
        theme="light"
        upgradeStatus={updateAvailableStatus}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("Signed in")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.queryByText("https://hub.example.test")).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Sign out" }));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });
});
