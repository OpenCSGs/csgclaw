import { createRef } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { ConversationMessageList } from "@/components/business/ConversationPane";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, IMUser, TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key) => key;

const agentUser: IMUser = {
  id: "agent-1",
  name: "Builder",
  role: "worker",
};

const conversation: IMConversation = {
  id: "room-1",
  members: [agentUser.id],
  messages: [
    {
      content: "Done",
      id: "message-1",
      sender_id: agentUser.id,
    },
  ],
  title: "Build room",
};

function renderMessageList({
  agents = [{ id: agentUser.id, name: agentUser.name, role: "worker" }],
}: {
  agents?: AgentLike[];
} = {}) {
  const onOpenAgentDetail = vi.fn<(agent: AgentLike, anchor: HTMLElement) => void>();
  const onPreviewUser = vi.fn<(user: IMUser, anchor: HTMLElement) => void>();
  render(
    <ConversationMessageList
      agents={agents}
      conversation={conversation}
      locale="en"
      messageActionBusy=""
      messageActionFeedback={{}}
      messageListRef={createRef<HTMLElement>()}
      t={t}
      theme="light"
      usersById={new Map([[agentUser.id, agentUser]])}
      visibleMessages={conversation.messages}
      onMessageAction={vi.fn()}
      onOpenAgentDetail={onOpenAgentDetail}
      onOpenThread={vi.fn()}
      onPreviewUser={onPreviewUser}
    />,
  );
  return { onOpenAgentDetail, onPreviewUser };
}

describe("ConversationMessageList profile preview", () => {
  it("opens the same profile preview for agent avatars", () => {
    const { onOpenAgentDetail, onPreviewUser } = renderMessageList();
    const avatar = screen.getByRole("button", { name: "profilePreview Builder" });

    avatar.focus();
    expect(onPreviewUser).not.toHaveBeenCalled();

    fireEvent.click(avatar);
    expect(onOpenAgentDetail).not.toHaveBeenCalled();
    expect(onPreviewUser).toHaveBeenCalledWith(agentUser, avatar);
  });

  it("still opens the compact preview when a human avatar is activated", () => {
    const { onOpenAgentDetail, onPreviewUser } = renderMessageList({ agents: [] });
    const avatar = screen.getByRole("button", { name: "profilePreview Builder" });

    fireEvent.click(avatar);
    expect(onOpenAgentDetail).not.toHaveBeenCalled();
    expect(onPreviewUser).toHaveBeenCalledWith(agentUser, avatar);
  });

  it("grays an agent message avatar when runtime availability is degraded", () => {
    renderMessageList({
      agents: [
        {
          id: agentUser.id,
          name: agentUser.name,
          role: "worker",
          status: "running",
          runtime: {
            state: "running",
            availability: { state: "degraded", reason: "control_plane_unavailable" },
          },
        },
      ],
    });

    const avatar = screen.getByRole("button", { name: "profilePreview Builder" });
    expect(avatar.querySelector(".message-avatar-status")).not.toHaveClass("online");
  });
});
