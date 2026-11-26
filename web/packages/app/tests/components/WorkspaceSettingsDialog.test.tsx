import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { TooltipProvider } from "@/components/ui";
import {
  WorkspaceControllerProvider,
  useWorkspaceControllerContext,
  type WorkspaceController,
} from "@/hooks/workspace";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";
import { SettingsPage } from "@/pages/SettingsPage";
import { WorkspaceFeedback } from "@/components/business/Feedback";
import { WorkspaceAccountFooter } from "@/pages/WorkspacePage/components/WorkspaceUserMenu";
import { normalizeUpgradeStatus } from "@/models/upgradeStatus";
import { emptyAuthStatus } from "@/models/auth";
import { defaultAuthEnvironmentDraft } from "@/models/authEnvironment";

function Harness() {
  const dialogs = useWorkspaceDialogs();
  const location = useLocation();
  return (
    <>
      <output data-testid="location">
        {location.pathname}
        {location.search}
      </output>
      <input aria-label="Unsent draft" defaultValue="keep this draft" />
      <button onClick={() => dialogs.openSettings()}>Open settings</button>
      <SettingsPage />
    </>
  );
}

it("opens Settings over the current page and closes it without changing its tab or draft", async () => {
  const load = vi.fn().mockResolvedValue(undefined);
  const t = (key: string) => key;
  const controller = {
    ready: true,
    t,
    configSettings: { load },
    sidebarProps: {
      t,
      authStatus: emptyAuthStatus(),
      authEnvironment: defaultAuthEnvironmentDraft(),
      hub: { setSelectedHubResourceType: vi.fn() },
      theme: "light",
      locale: "zh",
      onThemeChange: vi.fn(),
      onLocaleChange: vi.fn(),
      onLogin: vi.fn(),
      onLogout: vi.fn(),
    },
  } as unknown as WorkspaceController;
  render(
    <MemoryRouter initialEntries={["/agents/agent-1?tab=connectors"]}>
      <TooltipProvider>
        <WorkspaceControllerProvider controller={controller}>
          <Harness />
        </WorkspaceControllerProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Open settings" }));
  expect(screen.getByRole("dialog")).toHaveAccessibleName("settings");
  expect(screen.getByTestId("location")).toHaveTextContent("/agents/agent-1?tab=connectors&dialog=settings");
  await user.click(screen.getByRole("button", { name: /iaSettingsCardAppearance/ }));
  expect(screen.getByTestId("location")).toHaveTextContent("section=appearance");
  await user.click(screen.getByRole("button", { name: "close" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByTestId("location")).toHaveTextContent("/agents/agent-1?tab=connectors");
  expect(screen.getByRole("textbox", { name: "Unsent draft" })).toHaveValue("keep this draft");
  expect(load).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(screen.getByRole("button", { name: "Open settings" })).toHaveFocus());
  await user.click(screen.getByRole("button", { name: "Open settings" }));
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(screen.getByRole("button", { name: "Open settings" })).toHaveFocus());
});

function EntryHarness() {
  const { sidebarProps } = useWorkspaceControllerContext();
  const { openSettings } = useWorkspaceDialogs();
  if (!sidebarProps) return null;
  return (
    <>
      <WorkspaceAccountFooter {...sidebarProps} onOpenSettings={() => openSettings()} />
      <SettingsPage />
      <WorkspaceFeedback />
    </>
  );
}

function renderEntryHarness(updateAvailable: boolean, authenticated = true) {
  const t = (key: string) => key;
  const apply = vi.fn();
  const controller = {
    ready: true,
    t,
    sidebarProps: {
      t,
      authStatus: { ...emptyAuthStatus(), authenticated },
      authEnvironment: defaultAuthEnvironmentDraft(),
      theme: "light",
      locale: "zh",
      currentUserID: "local",
      usersById: new Map(),
      showUpgradeControls: true,
      upgradeStatus: normalizeUpgradeStatus({
        current_version: "v0.12.0",
        latest_version: "v0.13.0",
        update_available: updateAvailable,
      }),
      upgradePhase: "idle",
      onOpenUpgrade: apply,
      onRefreshUpgradeStatus: vi.fn().mockResolvedValue(null),
    },
  } as unknown as WorkspaceController;
  render(
    <MemoryRouter initialEntries={["/agents/agent-1?tab=connectors"]}>
      <TooltipProvider>
        <WorkspaceControllerProvider controller={controller}>
          <EntryHarness />
        </WorkspaceControllerProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
  return apply;
}

it("opens version settings with a highlight and clears it on leaving the section", async () => {
  const apply = renderEntryHarness(true);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "sidebarNewVersion" }));
  expect(screen.getByRole("heading", { name: /versionInfo/ }).closest("section")?.className).toContain("highlighted");
  expect(apply).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: /iaSettingsCardAppearance/ }));
  await user.click(screen.getByRole("button", { name: /iaSettingsCardVersion/ }));
  expect(screen.getByRole("heading", { name: /versionInfo/ }).closest("section")?.className).not.toContain(
    "highlighted",
  );
});

it("groups menu items and opens the same feedback draft from Settings", async () => {
  renderEntryHarness(false);
  const user = userEvent.setup();
  expect(screen.queryByRole("button", { name: "sidebarNewVersion" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "iaUserMenuLabel" }));
  expect(
    screen
      .getAllByRole("menuitem")
      .slice(-4)
      .map((item) => item.textContent),
  ).toEqual(["iaUserMenuProfileLocal", "settings", "sidebarAppearanceLanguage", "feedbackMenuAction"]);
  await user.click(screen.getByRole("menuitem", { name: "feedbackMenuAction" }));
  await user.type(screen.getByRole("textbox", { name: "settingsFeedbackLabel" }), "Shared draft");
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "iaUserMenuLabel" }));
  await user.click(screen.getByRole("menuitem", { name: "settings" }));
  await user.click(screen.getByRole("button", { name: /iaSettingsCardFeedback/ }));
  const trigger = screen.getByRole("button", { name: "feedbackWrite" });
  await user.click(trigger);
  expect(screen.getByRole("textbox", { name: "settingsFeedbackLabel" })).toHaveValue("Shared draft");
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.getByRole("dialog", { name: "settings" })).toBeInTheDocument());
  await waitFor(() => expect(trigger).toHaveFocus());
});

it.each([true, false])(
  "returns keyboard focus to the account menu after feedback (signed in: %s)",
  async (signedIn) => {
    renderEntryHarness(false, signedIn);
    const user = userEvent.setup();
    const account = screen.getByRole("button", { name: "iaUserMenuLabel" });
    account.focus();
    await user.keyboard("{Enter}{End}{Enter}");
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAccessibleName(signedIn ? "feedbackTitle" : "feedbackLoginTitle");
    if (!signedIn) {
      expect(screen.getByText("feedbackLoginReturnHint", { exact: false })).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "feedbackContinueOnGitHub" })).toHaveAttribute(
        "href",
        expect.stringContaining("https://github.com/OpenCSGs/csgclaw/issues/new"),
      );
    }

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(account).toHaveFocus());
  },
);
