import { checkAuthSite, fetchAuthLoginSites } from "@/api/auth";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@/components/ui";
import {
  authEnvironmentDraftFromPreset,
  authEnvironmentLoginPayload,
  defaultAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import type { TranslateFn } from "@/models/conversations";
import { AUTH_CUSTOM_SITES_STORAGE_KEY } from "@/shared/storage/keys";
import { OpenCSGConnectionDialog } from "./OpenCSGConnectionDialog";

const t: TranslateFn = (key) => key;

vi.mock("@/api/auth", () => ({ checkAuthSite: vi.fn(), fetchAuthLoginSites: vi.fn() }));

const sites = [
  { id: "main", url: "https://opencsg.com", title: "OpenCSG Main Site" },
  { id: "df", url: "https://openeast.opencsg.com", title: "Open East Community" },
  { id: "sx", url: "https://sanxia.opencsg.com", title: "Three Gorges Community" },
].map(({ id, url, title }) => ({
  id,
  url,
  icon: "data:image/png;base64,AAAA",
  locales: { en: { title, description: "" }, zh: { title, description: "" } },
}));

beforeEach(() => {
  window.localStorage.clear();
  vi.mocked(fetchAuthLoginSites).mockResolvedValue(sites);
});

async function chooseSite(user: ReturnType<typeof userEvent.setup>, label: RegExp) {
  const picker = screen.getByRole("combobox", { name: "csghubLoginEnvironment" });
  picker.focus();
  await user.keyboard("{Enter}");
  const option = await screen.findByRole("option", { name: label });
  option.focus();
  await user.keyboard("{Enter}");
}

describe("OpenCSGConnectionDialog", () => {
  it("keeps custom login disabled until the site URL is valid", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <OpenCSGConnectionDialog
          busy={false}
          environment={defaultAuthEnvironmentDraft()}
          locale="en"
          open
          t={t}
          onConnect={onConnect}
          onOpenChange={() => undefined}
        />
      </TooltipProvider>,
    );

    await screen.findByText("OpenCSG Main Site");
    await chooseSite(user, /csghubEnvCustom/);
    const continueButton = screen.getByRole("button", { name: "csghubConnectContinue" });
    const customURLInput = screen.getByRole("textbox", { name: /csghubOpenCSGBaseURL/ });
    expect(continueButton).toBeDisabled();
    expect(screen.queryByText("csghubInvalidSiteURL")).not.toBeInTheDocument();
    expect(customURLInput).not.toHaveAttribute("aria-invalid");

    await chooseSite(user, /OpenCSG Main Site/);
    expect(screen.queryByText("csghubInvalidSiteURL")).not.toBeInTheDocument();

    await chooseSite(user, /csghubEnvCustom/);
    const reopenedCustomURLInput = screen.getByRole("textbox", { name: /csghubOpenCSGBaseURL/ });
    await user.type(reopenedCustomURLInput, "not-a-url");
    expect(screen.getByText("csghubInvalidSiteURL")).toBeInTheDocument();
    expect(reopenedCustomURLInput).toHaveAttribute("aria-invalid", "true");

    await user.clear(reopenedCustomURLInput);
    await user.type(reopenedCustomURLInput, "https://east.example.com");
    expect(continueButton).toBeEnabled();
    expect(screen.queryByText("csghubInvalidSiteURL")).not.toBeInTheDocument();
    expect(reopenedCustomURLInput).not.toHaveAttribute("aria-invalid");

    await user.click(continueButton);
    expect(onConnect).toHaveBeenCalledWith({
      preset: "custom",
      opencsgBaseURL: "https://east.example.com",
      csgHubBaseURL: "",
      aiGatewayBaseURL: "",
    });
  });

  it("keeps multiple saved sites available after remounting, even without the catalog", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    const renderDialog = () =>
      render(
        <TooltipProvider delayDuration={0}>
          <OpenCSGConnectionDialog
            busy={false}
            environment={defaultAuthEnvironmentDraft()}
            locale="en"
            open
            t={t}
            onConnect={onConnect}
            onOpenChange={vi.fn()}
          />
        </TooltipProvider>,
      );

    const first = renderDialog();
    await screen.findByText("OpenCSG Main Site");
    for (const url of ["https://FIRST.example.com/team/?token=ignored#section", "https://second.example.com"]) {
      await chooseSite(user, /csghubEnvCustom/);
      await user.type(screen.getByRole("textbox", { name: /csghubOpenCSGBaseURL/ }), url);
      await user.click(screen.getByRole("button", { name: "csghubSaveCustomSite" }));
    }
    expect(onConnect).not.toHaveBeenCalled();
    expect(screen.getByRole("combobox")).toHaveTextContent("https://second.example.com");
    first.unmount();

    vi.mocked(fetchAuthLoginSites).mockRejectedValueOnce(new Error("unavailable"));
    renderDialog();
    await screen.findByText("csghubLoginSitesUnavailable");
    await chooseSite(user, /https:\/\/second\.example\.com/);
    await chooseSite(user, /https:\/\/first\.example\.com\/team$/);
    await user.click(screen.getByRole("button", { name: "csghubConnectContinue" }));

    expect(onConnect).toHaveBeenCalledWith({
      preset: "custom",
      opencsgBaseURL: "https://first.example.com/team",
      csgHubBaseURL: "",
      aiGatewayBaseURL: "",
    });
  });

  it("preserves preset endpoints offline and merges an equivalent catalog URL on retry", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    const renderDialog = (environment = defaultAuthEnvironmentDraft()) =>
      render(
        <TooltipProvider delayDuration={0}>
          <OpenCSGConnectionDialog
            busy={false}
            environment={environment}
            locale="en"
            open
            t={t}
            onConnect={onConnect}
            onOpenChange={vi.fn()}
          />
        </TooltipProvider>,
      );
    const expectProductionLogin = async () => {
      onConnect.mockClear();
      await user.click(screen.getByRole("button", { name: "csghubConnectContinue" }));
      expect(onConnect).toHaveBeenCalledTimes(1);
      expect(authEnvironmentLoginPayload(onConnect.mock.calls[0][0])).toEqual({
        opencsg_base_url: "https://opencsg.com",
        csghub_base_url: "https://hub.opencsg.com",
        ai_gateway_base_url: "https://ai.space.opencsg.com/v1",
      });
    };

    vi.mocked(fetchAuthLoginSites).mockRejectedValueOnce(new Error("unavailable"));
    const first = renderDialog();
    await screen.findByText("csghubLoginSitesUnavailable");
    await user.click(screen.getByRole("button", { name: "csghubSaveCustomSite" }));
    await expectProductionLogin();
    first.unmount();

    vi.mocked(fetchAuthLoginSites)
      .mockRejectedValueOnce(new Error("unavailable"))
      .mockResolvedValueOnce([{ ...sites[0], url: "https://OPENCSG.com/" }]);
    renderDialog(authEnvironmentDraftFromPreset("east"));
    await screen.findByText("csghubLoginSitesUnavailable");
    await chooseSite(user, /^https:\/\/opencsg\.com$/);
    await expectProductionLogin();

    await user.click(screen.getByRole("button", { name: "retry" }));
    await screen.findByText("OpenCSG Main Site");
    const picker = screen.getByRole("combobox", { name: "csghubLoginEnvironment" });
    picker.focus();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("option", { name: /OpenCSG Main Site/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /^https:\/\/opencsg\.com$/ })).not.toBeInTheDocument();
  });

  it("edits and deletes a legacy saved URL and tests the edited site's login endpoint", async () => {
    window.localStorage.setItem(AUTH_CUSTOM_SITES_STORAGE_KEY, JSON.stringify(["https://legacy.example.com"]));
    vi.mocked(checkAuthSite).mockResolvedValueOnce({ latency_ms: 31 });
    const user = userEvent.setup();
    render(
      <TooltipProvider delayDuration={0}>
        <OpenCSGConnectionDialog
          busy={false}
          environment={defaultAuthEnvironmentDraft()}
          locale="en"
          open
          t={t}
          onConnect={vi.fn()}
          onOpenChange={vi.fn()}
        />
      </TooltipProvider>,
    );
    await screen.findByText("OpenCSG Main Site");
    await chooseSite(user, /legacy\.example\.com/);
    await user.click(screen.getByRole("button", { name: "csghubSiteEdit" }));
    expect(screen.queryByRole("button", { name: "csghubConnectContinue" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "csghubSiteCancelEdit" }));
    expect(screen.getByRole("button", { name: "csghubConnectContinue" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "csghubSiteEdit" }));
    await user.type(screen.getByRole("textbox", { name: "csghubSiteName" }), "Partner");
    const address = screen.getByRole("textbox", { name: "csghubOpenCSGBaseURL" });
    await user.clear(address);
    await user.type(address, "partner.example.com");
    await user.click(screen.getByRole("button", { name: "csghubSiteSaveChanges" }));
    expect(screen.getByRole("combobox")).toHaveTextContent("Partner");
    expect(JSON.parse(window.localStorage.getItem(AUTH_CUSTOM_SITES_STORAGE_KEY) || "[]")).toEqual([
      { name: "Partner", url: "https://partner.example.com" },
    ]);

    await user.click(screen.getByRole("button", { name: "csghubSiteTest" }));
    expect(checkAuthSite).toHaveBeenCalledWith("https://partner.example.com", expect.any(AbortSignal));
    expect(await screen.findByText("csghubSiteTestPassed")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "delete" }));
    await user.click(screen.getByRole("button", { name: "csghubSiteConfirmDelete" }));
    expect(screen.getByRole("combobox")).toHaveTextContent("OpenCSG Main Site");
    expect(window.localStorage.getItem(AUTH_CUSTOM_SITES_STORAGE_KEY)).toBe("[]");
  });

  it("requires choosing a different site before continuing a switch", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <OpenCSGConnectionDialog
          busy={false}
          environment={defaultAuthEnvironmentDraft()}
          locale="en"
          open
          t={t}
          variant="switch-site"
          onConnect={onConnect}
          onOpenChange={vi.fn()}
        />
      </TooltipProvider>,
    );
    await screen.findByText("OpenCSG Main Site");
    const proceed = screen.getByRole("button", { name: "csghubSwitchEnvironment" });
    expect(proceed).toBeDisabled();
    expect(screen.queryByText("csghubLoginTargetHint")).not.toBeInTheDocument();
    await chooseSite(user, /Open East Community/);
    expect(screen.getByText(/csghubLoginTargetHint/)).toBeInTheDocument();
    await user.click(proceed);
    expect(onConnect).toHaveBeenCalledWith(authEnvironmentDraftFromPreset("east"));
  });

  it("uses the same site selection flow when authentication is required", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <OpenCSGConnectionDialog
          busy={false}
          environment={defaultAuthEnvironmentDraft()}
          locale="en"
          open
          t={t}
          variant="authentication-required"
          onConnect={onConnect}
          onOpenChange={() => undefined}
        />
      </TooltipProvider>,
    );

    expect(screen.getByText("openCSGLoginRequiredTitle")).toBeInTheDocument();
    await screen.findByText("OpenCSG Main Site");
    await chooseSite(user, /Three Gorges Community/);
    await user.click(screen.getByRole("button", { name: "csghubSignIn" }));

    expect(onConnect).toHaveBeenCalledWith({
      preset: "custom",
      opencsgBaseURL: "https://sanxia.opencsg.com",
      csgHubBaseURL: "",
      aiGatewayBaseURL: "",
    });
  });

  it("selects the OpenEast community preset", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <OpenCSGConnectionDialog
          busy={false}
          environment={defaultAuthEnvironmentDraft()}
          locale="en"
          open
          t={t}
          onConnect={onConnect}
          onOpenChange={() => undefined}
        />
      </TooltipProvider>,
    );

    await screen.findByText("OpenCSG Main Site");
    await chooseSite(user, /Open East Community/);
    await user.click(screen.getByRole("button", { name: "csghubConnectContinue" }));

    expect(onConnect).toHaveBeenCalledWith({
      preset: "east",
      opencsgBaseURL: "https://openeast.opencsg.com",
      csgHubBaseURL: "https://openeast.opencsg.com",
      aiGatewayBaseURL: "https://openeast.opencsg.com/aigateway/v1",
    });
  });

  it("allows retry and manual entry when the site catalog is unavailable", async () => {
    vi.mocked(fetchAuthLoginSites).mockRejectedValueOnce(new Error("unavailable"));
    const user = userEvent.setup();
    render(
      <TooltipProvider delayDuration={0}>
        <OpenCSGConnectionDialog
          busy={false}
          environment={defaultAuthEnvironmentDraft()}
          locale="en"
          open
          t={t}
          onConnect={vi.fn()}
          onOpenChange={vi.fn()}
        />
      </TooltipProvider>,
    );

    await screen.findByText("csghubLoginSitesUnavailable");
    expect(screen.getByRole("textbox", { name: "csghubOpenCSGBaseURL" })).toHaveValue("https://opencsg.com");
    await user.click(screen.getByRole("button", { name: "retry" }));
    expect(await screen.findByText("OpenCSG Main Site")).toBeVisible();
    expect(screen.queryByRole("textbox", { name: "csghubOpenCSGBaseURL" })).not.toBeInTheDocument();
  });
});
