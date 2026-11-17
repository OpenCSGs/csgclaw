import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fetchAuthLoginSites } from "@/api/auth";
import { TooltipProvider } from "@/components/ui";
import { authEnvironmentDraftFromPreset, defaultAuthEnvironmentDraft } from "@/models/authEnvironment";
import { AccountSection } from "./AccountSection";

const { onLogin, onLogout } = vi.hoisted(() => ({ onLogin: vi.fn(), onLogout: vi.fn() }));

vi.mock("@/api/auth", () => ({ checkAuthSite: vi.fn(), fetchAuthLoginSites: vi.fn() }));
vi.mock("@/hooks/workspace", () => ({
  useWorkspaceControllerContext: () => ({
    sidebarProps: {
      authStatus: { authenticated: true, user_id: "alice", opencsg_base_url: "https://opencsg.com" },
      authEnvironment: defaultAuthEnvironmentDraft(),
      locale: "en",
      t: (key: string) => key,
      onLogin,
      onLogout,
    },
  }),
}));

it("keeps the current session until a target is confirmed and signs in only after logout succeeds", async () => {
  onLogout.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
  vi.mocked(fetchAuthLoginSites).mockResolvedValue(
    [
      { id: "main", url: "https://opencsg.com", title: "Main site" },
      { id: "east", url: "https://openeast.opencsg.com", title: "East site" },
    ].map(({ id, url, title }) => ({
      id,
      url,
      icon: "data:image/png;base64,AAAA",
      locales: { en: { title, description: "" }, zh: { title, description: "" } },
    })),
  );
  const user = userEvent.setup();
  render(
    <TooltipProvider delayDuration={0}>
      <AccountSection />
    </TooltipProvider>,
  );

  const switchButton = screen.getByRole("button", { name: "csghubSwitchEnvironment" });
  await user.click(switchButton);
  await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "cancel" }));
  expect(onLogout).not.toHaveBeenCalled();

  await user.click(switchButton);
  await screen.findByText("Main site");
  screen.getByRole("combobox").focus();
  await user.keyboard("{Enter}");
  (await screen.findByRole("option", { name: /East site/ })).focus();
  await user.keyboard("{Enter}");
  await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "csghubSwitchEnvironment" }));
  expect(onLogout).not.toHaveBeenCalled();

  const confirm = await screen.findByRole("button", { name: "csghubSwitchConfirmAction" });
  await user.click(confirm);
  expect(onLogout).toHaveBeenCalledTimes(1);
  expect(onLogin).not.toHaveBeenCalled();
  await user.click(confirm);
  await waitFor(() => expect(onLogin).toHaveBeenCalledWith(authEnvironmentDraftFromPreset("east")));
});
