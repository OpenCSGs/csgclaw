import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { TooltipProvider } from "@/components/ui";
import { WorkspaceControllerProvider, type WorkspaceController } from "@/hooks/workspace";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";
import { SettingsPage } from "@/pages/SettingsPage";
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
});
