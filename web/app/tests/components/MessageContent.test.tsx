import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MessageContent } from "@/components/business/MessageContent";
import {
  ACTION_REBUILD_MANAGER,
  CSGCLAW_ACTION_CARD_TYPE,
  CSGCLAW_NOTIFY_CARD_TYPE,
} from "@/shared/constants/messages";

describe("MessageContent", () => {
  it("renders sanitized markdown links with safe external-link attributes", () => {
    render(<MessageContent content={'Open [docs](https://example.com/docs)<script>alert("xss")</script>'} />);

    const link = screen.getByRole("link", { name: "docs" });
    expect(link).toHaveAttribute("href", "https://example.com/docs");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(document.querySelector("script")).toBeNull();
  });

  it("renders action cards and invokes the selected action with message context", async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    const message = { id: "message-1" };

    render(
      <MessageContent
        content={JSON.stringify({
          actions: [
            { id: ACTION_REBUILD_MANAGER, label: "Rebuild manager", style: "danger" },
            { id: "ignored-action", label: "Ignored" },
          ],
          badge: "Required",
          title: "Manager needs rebuild",
          type: CSGCLAW_ACTION_CARD_TYPE,
        })}
        message={message}
        onAction={onAction}
      />,
    );

    expect(screen.getByText("Manager needs rebuild")).toBeInTheDocument();
    expect(screen.getByText("Required")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ignored" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Rebuild manager" }));

    expect(onAction).toHaveBeenCalledTimes(1);
    expect(onAction).toHaveBeenCalledWith(
      expect.objectContaining({
        id: ACTION_REBUILD_MANAGER,
        label: "Rebuild manager",
        style: "danger",
      }),
      message,
    );
  });

  it("disables the busy action and shows action errors for the matching message", () => {
    render(
      <MessageContent
        actionBusy="message-1:rebuild-manager"
        actionError={{ key: "message-1:rebuild-manager", message: "Rebuild failed" }}
        content={JSON.stringify({
          actions: [{ id: ACTION_REBUILD_MANAGER, label: "Rebuild manager" }],
          title: "Manager needs rebuild",
          type: CSGCLAW_ACTION_CARD_TYPE,
        })}
        message={{ id: "message-1" }}
        onAction={vi.fn()}
      />,
    );

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
    expect(button).toHaveTextContent("...");
    expect(screen.getByText("Rebuild failed")).toBeInTheDocument();
  });

  it("renders notifier cards as structured content", () => {
    render(
      <MessageContent
        content={JSON.stringify({
          type: CSGCLAW_NOTIFY_CARD_TYPE,
          title: "GitHub · Pull request",
          badge: "opened",
          link: "https://github.com/acme/app/pull/1",
          meta: [{ label: "Branch", value: "feature -> main" }],
          raw: '{"action":"opened"}',
        })}
      />,
    );

    expect(screen.getByText("GitHub · Pull request")).toBeInTheDocument();
    expect(screen.getByText("opened")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open link" })).toHaveAttribute(
      "href",
      "https://github.com/acme/app/pull/1",
    );
    expect(screen.getByText("Branch")).toBeInTheDocument();
    expect(screen.getByText("feature -> main")).toBeInTheDocument();
  });
});
