import { createRef } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import type { IMMessage } from "@/models/conversations";
import { CollaborationStatus } from "./CollaborationStatus";
import { ConversationMessageList } from "../ConversationMessageList";

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMConnection: () => () => undefined,
  imEventsConnected: () => true,
}));
const t = createTranslator("en");
const usersById = new Map([["dev", { id: "dev", name: "Developer" }]]);
const participant = {
  id: "dev",
  name: "Developer",
  roomID: "room",
  requestID: "request",
  leaseID: "lease",
  canStop: true,
};
function message(status = "running"): IMMessage {
  return {
    id: "turn",
    sender_id: "dev",
    content: status === "succeeded" ? "Delivered result" : "\u200b",
    metadata: {
      csgclaw: {
        request_id: "request",
        turn_progress: {
          id: "turn",
          revision: 1,
          status,
          started_at: "2026-11-11T12:04:00Z",
          error: status === "failed" ? "Provider unavailable" : undefined,
          items: [
            {
              id: "search",
              kind: "tool",
              tool: {
                name: "web_search",
                actions: ["search"],
                status: "running",
                input: '{"query":"AI news"}',
                output: "Private tool output",
              },
            },
          ],
        },
      },
    },
  };
}

describe("collaboration status dock", () => {
  it("opens the working member's activity, stops the exact lease, and hides the dock after completion", async () => {
    const user = userEvent.setup();
    const onOpenActivity = vi.fn();
    const onStop = vi.fn();
    const props = {
      roomID: "room",
      messages: [message()],
      participants: [participant],
      usersById,
      t,
      onOpenActivity,
      onStop,
    };
    const { rerender } = render(<CollaborationStatus {...props} />);
    expect(screen.getByText("Developer")).toBeVisible();
    expect(screen.getByRole("status")).toBeVisible();
    expect(screen.queryByRole("button", { name: "View activity" })).not.toBeInTheDocument();
    expect(screen.getByText("AI news")).toBeVisible();
    expect(screen.queryByText("Private tool output")).not.toBeInTheDocument();
    const activity = screen.getByRole("button", { name: /View Developer/ });
    expect(activity).toHaveAccessibleDescription("AI news");
    await user.click(activity);
    expect(onOpenActivity).toHaveBeenCalledWith("dev", "turn", "request");
    await user.click(screen.getByRole("button", { name: /Stop Developer/ }));
    expect(onStop).toHaveBeenCalledWith(participant);
    rerender(<CollaborationStatus {...props} messages={[message("succeeded")]} />);
    expect(screen.queryByRole("region", { name: "Collaboration activity" })).not.toBeInTheDocument();
  });

  it("exposes pending members and failures without expanding anything", () => {
    const { rerender } = render(
      <CollaborationStatus roomID="room" messages={[]} participants={[participant]} usersById={usersById} t={t} />,
    );
    expect(screen.getByText("Developer")).toBeVisible();
    rerender(<CollaborationStatus roomID="room" messages={[message("failed")]} usersById={usersById} t={t} />);
    expect(screen.getByText("Provider unavailable")).toBeVisible();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("keeps execution out of the collaborative transcript and preserves the original direct chat", () => {
    const messages = [message()];
    const props = {
      conversation: { id: "room", members: ["dev"], messages, is_direct: false },
      visibleMessages: messages,
      usersById,
      t,
      locale: "en" as const,
      theme: "light" as const,
      messageActionBusy: "",
      messageActionFeedback: {},
      messageListRef: createRef<HTMLElement>(),
      onMessageAction: vi.fn(),
      onOpenThread: vi.fn(),
      onPreviewUser: vi.fn(),
    };
    const { rerender, container } = render(<ConversationMessageList {...props} />);
    expect(screen.queryByRole("region", { name: "Collaboration activity" })).not.toBeInTheDocument();
    expect(container.querySelector("[data-turn-id]")).toBeNull();
    rerender(<ConversationMessageList {...props} conversation={{ ...props.conversation, is_direct: true }} />);
    expect(container.querySelector("[data-turn-id]")).not.toBeNull();
    expect(container.querySelector("[data-turn-id] ol")).toBeVisible();
  });
});
