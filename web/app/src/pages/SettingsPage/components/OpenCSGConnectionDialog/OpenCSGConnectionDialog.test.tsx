import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@/components/ui";
import { defaultAuthEnvironmentDraft } from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import type { TranslateFn } from "@/models/conversations";
import { OpenCSGConnectionDialog } from "./OpenCSGConnectionDialog";

const t: TranslateFn = (key) => key;

describe("OpenCSGConnectionDialog", () => {
  it("keeps custom login disabled until the site URL is valid", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();

    function Harness() {
      const [draft, setDraft] = useState<AuthEnvironmentDraft>(defaultAuthEnvironmentDraft);
      return (
        <TooltipProvider delayDuration={0}>
          <OpenCSGConnectionDialog
            busy={false}
            draft={draft}
            open
            t={t}
            onConnect={onConnect}
            onDraftChange={setDraft}
            onOpenChange={() => undefined}
          />
        </TooltipProvider>
      );
    }

    render(<Harness />);

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
    expect(onConnect).toHaveBeenCalledOnce();
  });
});
