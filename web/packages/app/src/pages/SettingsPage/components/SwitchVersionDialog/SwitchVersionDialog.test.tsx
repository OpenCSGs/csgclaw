import type { ComponentProps } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { SwitchVersionDialog } from "./SwitchVersionDialog";

const t: TranslateFn = (key) => key;

function renderDialog(
  props: Partial<ComponentProps<typeof SwitchVersionDialog>> &
    Pick<ComponentProps<typeof SwitchVersionDialog>, "currentChannel" | "onConfirm" | "onOpenChange">,
) {
  return render(
    <TooltipProvider delayDuration={0}>
      <SwitchVersionDialog open t={t} {...props} />
    </TooltipProvider>,
  );
}

describe("SwitchVersionDialog", () => {
  it("selects the inferred current channel and keeps confirm disabled until the other channel is chosen", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn().mockResolvedValue(true);
    const onOpenChange = vi.fn();

    renderDialog({
      currentChannel: "release",
      onConfirm,
      onOpenChange,
    });

    expect(screen.getByRole("radio", { name: "upgradeChannelRelease" })).toBeChecked();
    expect(screen.getByRole("button", { name: "upgradeChannelCurrentActive" })).toBeDisabled();

    await user.click(screen.getByRole("radio", { name: "upgradeChannelBeta" }));
    expect(screen.getByRole("button", { name: "upgradeChannelInstallBeta" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "upgradeChannelInstallBeta" }));

    expect(onConfirm).toHaveBeenCalledWith("beta");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("resets to the inferred channel after close without confirming a change", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    const onOpenChange = vi.fn();

    const { rerender } = renderDialog({
      currentChannel: "beta",
      onConfirm,
      onOpenChange,
    });

    expect(screen.getByRole("radio", { name: "upgradeChannelBeta" })).toBeChecked();
    await user.click(screen.getByRole("radio", { name: "upgradeChannelRelease" }));
    expect(screen.getByRole("button", { name: "upgradeChannelInstallRelease" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "cancel" }));
    expect(onConfirm).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);

    rerender(
      <TooltipProvider delayDuration={0}>
        <SwitchVersionDialog
          currentChannel="beta"
          open={false}
          t={t}
          onConfirm={onConfirm}
          onOpenChange={onOpenChange}
        />
      </TooltipProvider>,
    );
    rerender(
      <TooltipProvider delayDuration={0}>
        <SwitchVersionDialog currentChannel="beta" open t={t} onConfirm={onConfirm} onOpenChange={onOpenChange} />
      </TooltipProvider>,
    );

    expect(screen.getByRole("radio", { name: "upgradeChannelBeta" })).toBeChecked();
  });

  it("keeps the dialog open and shows the failure reason when confirm fails", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn().mockResolvedValue(false);
    const onOpenChange = vi.fn();

    renderDialog({
      currentChannel: "release",
      error: "upgradeChannelSwitchFailed network down",
      onConfirm,
      onOpenChange,
    });

    await user.click(screen.getByRole("radio", { name: "upgradeChannelBeta" }));
    await user.click(screen.getByRole("button", { name: "upgradeChannelInstallBeta" }));

    expect(onConfirm).toHaveBeenCalledWith("beta");
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByText("upgradeChannelSwitchFailed network down")).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "upgradeChannelBeta" })).toBeChecked();
  });

  it("allows the current channel to be checked again after a failed switch", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn().mockResolvedValue(true);
    const onOpenChange = vi.fn();

    renderDialog({
      currentChannel: "beta",
      error: "upgradeChannelSwitchFailed",
      onConfirm,
      onOpenChange,
    });

    expect(screen.getByRole("radio", { name: "upgradeChannelBeta" })).toBeChecked();
    expect(screen.getByRole("button", { name: "upgradeChannelRetryCurrent" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "upgradeChannelRetryCurrent" }));

    expect(onConfirm).toHaveBeenCalledWith("beta");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
