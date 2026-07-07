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
  upgradeErrorDetails: "Details: {detail}",
  upgradeErrorLogPath: "Log: {path}",
  upgradeErrorPermission: "Permission issue.",
  upgradeLocalBuild: "Local build",
  versionInfo: "Version",
  versionSettings: "Version and updates",
  configSettingsMenu: "Settings",
  configSettingsFeedbackSection: "Feedback",
  configSettingsGithubIssueAction: "Open GitHub",
  csghubAccount: "OpenCSG",
  csghubLoginPending: "Waiting for auth",
  csghubLoginPendingDetail: "Finish authorization",
  csghubLoginRequired: "Sign in with OpenCSG",
  csghubNotSignedIn: "Not signed in",
  csghubAdvancedSettings: "Advanced",
  csghubAIGatewayBaseURL: "AI Gateway URL (defaults to site /aigateway)",
  csghubAPIBaseURL: "CSGHub API URL (defaults to site)",
  csghubEnvCustom: "Custom environment",
  csghubLoginEnvironment: "Environment",
  csghubOpenCSGBaseURL: "OpenCSG site URL",
  csghubSignIn: "Sign in",
  csghubSigningIn: "Signing in...",
  csghubSignedIn: "Signed in",
  csghubSignOut: "Sign out",
  csghubSwitchEnvironment: "Switch environment",
  csghubToggleEnvironmentPanel: "Expand or collapse OpenCSG settings",
};

function t(key: string, params: Record<string, string | number> = {}): string {
  return (labels[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
}

const updateAvailableStatus: UpgradeStatus = {
  auto_upgrade_supported: true,
  auto_upgrade_unsupported_reason: "",
  checking: false,
  current_version: "v0.3.0",
  last_checked_at: "",
  last_error: "",
  last_error_kind: "",
  last_error_log_path: "",
  latest_version: "v0.3.1",
  manual_restart_required: false,
  update_available: true,
  upgrading: false,
};

describe("SidebarUserButton", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

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

  it("shows feedback below the version summary with user feedback prefilled", async () => {
    const user = userEvent.setup();

    render(
      <SidebarUserButton
        appVersion="v0.3.0"
        showUpgradeControls={false}
        locale="en"
        onOpenUpgrade={() => {}}
        onOpenConfigSettings={() => {}}
        onLocaleChange={() => {}}
        onThemeChange={() => {}}
        t={t}
        theme="light"
        upgradeStatus={updateAvailableStatus}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    const link = screen.getByRole("menuitem", { name: "Feedback" });
    const href = link.getAttribute("href") || "";
    const url = new URL(href);
    expect(`${url.origin}${url.pathname}`).toBe("https://github.com/OpenCSGs/csgclaw/issues/new");
    expect(url.searchParams.has("title")).toBe(false);
    expect(url.searchParams.get("labels")).toBe("user-feedback");
    expect(url.searchParams.get("body")).toBe("## Version information\n- CSGClaw version: v0.3.0\n");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("shows OpenCSG sign-in when no account is connected", async () => {
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

    expect(screen.getByText("OpenCSG")).toBeInTheDocument();
    expect(screen.queryByText("opencsg.com")).not.toBeInTheDocument();
    expect(screen.getAllByText("Not signed in")).toHaveLength(2);
    expect(screen.getByRole("menuitem", { name: "Sign in" })).toBeInTheDocument();
    expect(screen.queryByText("Sign in with OpenCSG")).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Environment" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Sign in" }));
    expect(onLogin).toHaveBeenCalledTimes(1);
    expect(onLogin).toHaveBeenCalledWith(
      expect.objectContaining({
        preset: "prod",
        opencsgBaseURL: "https://opencsg.com",
        csgHubBaseURL: "https://hub.opencsg.com",
        aiGatewayBaseURL: "https://ai.space.opencsg.com/v1",
      }),
    );

    await user.click(screen.getByRole("button", { name: "Expand or collapse OpenCSG settings" }));
    expect(screen.getByRole("combobox", { name: "Environment" })).toHaveValue("prod");
    expect(screen.queryByDisplayValue("https://opencsg.com")).not.toBeInTheDocument();
  });

  it("passes the selected OpenCSG stg environment to sign-in", async () => {
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
    await user.click(screen.getByRole("button", { name: "Expand or collapse OpenCSG settings" }));
    await user.selectOptions(screen.getByLabelText("Environment"), "stage");
    await user.click(screen.getByRole("button", { name: "Expand or collapse OpenCSG settings" }));
    expect(screen.getByText("opencsg-stg.com")).toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Sign in" }));

    expect(onLogin).toHaveBeenCalledWith(
      expect.objectContaining({
        preset: "stage",
        opencsgBaseURL: "https://opencsg-stg.com",
        csgHubBaseURL: "https://opencsg-stg.com",
        aiGatewayBaseURL: "https://aigateway.opencsg-stg.com/v1",
      }),
    );
  });

  it("derives custom OpenCSG service URLs from the site URL", async () => {
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
    await user.click(screen.getByRole("button", { name: "Expand or collapse OpenCSG settings" }));
    await user.selectOptions(screen.getByLabelText("Environment"), "custom");
    await user.type(screen.getByLabelText("OpenCSG site URL"), "https://openeast.opencsg.com");

    expect(screen.queryByLabelText("CSGHub API URL (defaults to site)")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("AI Gateway URL (defaults to site /aigateway)")).not.toBeInTheDocument();

    await user.click(screen.getByRole("menuitem", { name: "Sign in" }));

    expect(onLogin).toHaveBeenCalledWith(
      expect.objectContaining({
        preset: "custom",
        opencsgBaseURL: "https://openeast.opencsg.com",
        csgHubBaseURL: "https://openeast.opencsg.com",
        aiGatewayBaseURL: "https://openeast.opencsg.com/aigateway/v1",
      }),
    );
    expect(JSON.parse(window.localStorage.getItem("csgclaw.auth.environment") || "{}")).toMatchObject({
      preset: "custom",
      opencsgBaseURL: "https://openeast.opencsg.com",
      csgHubBaseURL: "https://openeast.opencsg.com",
      aiGatewayBaseURL: "https://openeast.opencsg.com/aigateway/v1",
    });
  });

  it("shows OpenCSG account metadata and signs out", async () => {
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
          name: "Alice Zhang",
          avatar: "https://example.test/avatar.png",
          opencsg_base_url: "https://opencsg.example.test",
          base_url: "https://hub.example.test",
          ai_gateway_base_url: "https://gateway.example.test/v1",
          portal_url: "https://hub.example.test/portal",
          logged_in_at: "2026-06-22T09:00:00Z",
        }}
        t={t}
        theme="light"
        upgradeStatus={updateAvailableStatus}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("Alice Zhang · Signed in")).toBeInTheDocument();
    expect(screen.getByText("opencsg.example.test")).toBeInTheDocument();
    expect(screen.getAllByText("Alice Zhang").length).toBeGreaterThan(0);
    expect(screen.getByRole("menuitem", { name: "Sign out" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Expand or collapse OpenCSG settings" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Environment" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Switch environment" })).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("https://opencsg.example.test")).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("https://hub.example.test")).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Sign out" }));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });
});
