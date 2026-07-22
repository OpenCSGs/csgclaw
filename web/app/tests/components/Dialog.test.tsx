import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  DialogCloseButton,
  DialogContent,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  TooltipProvider,
} from "@/components/ui";

describe("Dialog", () => {
  it("does not show a close-button tooltip when the dialog opens", async () => {
    const user = userEvent.setup();

    render(
      <TooltipProvider delayDuration={0}>
        <DialogRoot open>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Example dialog</DialogTitle>
              <DialogCloseButton label="Close" />
            </DialogHeader>
          </DialogContent>
        </DialogRoot>
      </TooltipProvider>,
    );

    const dialog = screen.getByRole("dialog");
    await waitFor(() => expect(dialog).toHaveFocus());
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    await user.tab();
    expect(screen.getByRole("button", { name: "Close" })).toHaveFocus();
    await waitFor(() => expect(screen.getByRole("tooltip", { name: "Close" })).toBeInTheDocument());
  });
});
