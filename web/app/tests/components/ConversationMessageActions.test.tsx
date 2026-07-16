import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ConversationMessageActions } from "@/components/business/ConversationPane/ConversationMessageActions";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key) =>
  ({
    copiedToClipboard: "Copied",
    copyToClipboard: "Copy",
    replyInThread: "Reply in thread",
  })[key] || key;

describe("ConversationMessageActions", () => {
  it("copies the complete message source with visible mention text", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    render(<ConversationMessageActions content={'First line\nSecond line for <at user_id="u-1">Alice</at>'} t={t} />);

    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith("First line\nSecond line for @Alice"));
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("copies canonical slash commands in their visible input format", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    render(
      <ConversationMessageActions
        content={
          '<slash-command name="use-skill" arg="reviewer"></slash-command> review with <at user_id="u-1">Alice</at>'
        }
        t={t}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith("/reviewer review with @Alice"));
  });

  it("opens the thread from the message action row", () => {
    const onOpenThread = vi.fn();
    render(<ConversationMessageActions content="Message" onOpenThread={onOpenThread} t={t} />);

    fireEvent.click(screen.getByRole("button", { name: "Reply in thread" }));

    expect(onOpenThread).toHaveBeenCalledTimes(1);
  });
});
