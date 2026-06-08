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
};

function t(key: string): string {
  return labels[key] ?? key;
}

const updateAvailableStatus: UpgradeStatus = {
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
    expect(screen.getByText("v0.3.5+local")).toBeInTheDocument();
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
});
