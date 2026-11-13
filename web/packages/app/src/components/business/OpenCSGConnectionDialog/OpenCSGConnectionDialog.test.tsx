import { fetchAuthLoginSites } from "@/api/auth";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@/components/ui";
import { defaultAuthEnvironmentDraft } from "@/models/authEnvironment";
import type { TranslateFn } from "@/models/conversations";
import { OpenCSGConnectionDialog } from "./OpenCSGConnectionDialog";

const t: TranslateFn = (key) => key;

vi.mock("@/api/auth", () => ({ fetchAuthLoginSites: vi.fn() }));

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

beforeEach(() => vi.mocked(fetchAuthLoginSites).mockResolvedValue(sites));

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
