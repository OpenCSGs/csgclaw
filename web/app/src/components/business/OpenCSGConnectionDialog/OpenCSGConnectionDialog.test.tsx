import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@/components/ui";
import { defaultAuthEnvironmentDraft } from "@/models/authEnvironment";
import type { TranslateFn } from "@/models/conversations";
import { OpenCSGConnectionDialog } from "./OpenCSGConnectionDialog";

const t: TranslateFn = (key) => key;

describe("OpenCSGConnectionDialog", () => {
  it("keeps custom login disabled until the site URL is valid", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    render(
      <TooltipProvider delayDuration={0}>
        <OpenCSGConnectionDialog
          busy={false}
          environment={defaultAuthEnvironmentDraft()}
          open
          t={t}
          onConnect={onConnect}
          onOpenChange={() => undefined}
        />
      </TooltipProvider>,
    );

    await user.click(screen.getByRole("radio", { name: /csghubEnvCustom/ }));
    const continueButton = screen.getByRole("button", { name: "csghubConnectContinue" });
    const customURLInput = screen.getByRole("textbox", { name: /csghubOpenCSGBaseURL/ });
    expect(continueButton).toBeDisabled();
    expect(screen.queryByText("csghubInvalidSiteURL")).not.toBeInTheDocument();
    expect(customURLInput).not.toHaveAttribute("aria-invalid");

    await user.click(screen.getByRole("radio", { name: /csghubEnvProduction/ }));
    expect(screen.queryByText("csghubInvalidSiteURL")).not.toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: /csghubEnvCustom/ }));
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
          open
          t={t}
          variant="authentication-required"
          onConnect={onConnect}
          onOpenChange={() => undefined}
        />
      </TooltipProvider>,
    );

    expect(screen.getByText("openCSGLoginRequiredTitle")).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /csghubEnvProduction/ })).toBeChecked();
    await user.click(screen.getByRole("radio", { name: /csghubEnvStage/ }));
    await user.click(screen.getByRole("button", { name: "csghubSignIn" }));

    expect(onConnect).toHaveBeenCalledWith({
      preset: "stage",
      opencsgBaseURL: "https://opencsg-stg.com",
      csgHubBaseURL: "https://opencsg-stg.com",
      aiGatewayBaseURL: "https://aigateway.opencsg-stg.com/v1",
    });
  });
});
