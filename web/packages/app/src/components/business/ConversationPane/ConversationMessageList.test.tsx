import { createRef } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import * as memoryAPI from "@/api/agents";
import * as activitiesAPI from "@/api/agentActivities";
import { ConversationMessageList, type ConversationMessageListProps } from "./ConversationMessageList";

function messageListProps(): ConversationMessageListProps {
  const messages = Array.from({ length: 100 }, (_, index) => ({
    id: `message-${index}`,
    sender_id: "manager",
    content: `Reply ${index}`,
    created_at: "2026-10-12T08:00:00Z",
  }));
  return {
    conversation: { id: "room", members: ["manager"], messages, is_direct: true },
    visibleMessages: messages,
    usersById: new Map([["manager", { id: "manager", name: "Manager" }]]),
    locale: "en",
    theme: "light",
    t: (key) => key,
    messageActionBusy: "",
    messageActionFeedback: {},
    messageListRef: createRef<HTMLElement>(),
    onMessageAction: vi.fn(),
    onOpenThread: vi.fn(),
    onPreviewUser: vi.fn(),
  };
}

describe("ConversationMessageList render isolation", () => {
  it("keeps the video card after the confirmation when the agent reply moves", () => {
    const props = messageListProps();
    props.usersById.set("local", { id: "local", name: "Local user" });
    props.currentUserID = "local";
    const turn = { id: "turn", sender_id: "manager", content: "Preparing", created_at: "2026-10-12T08:00:00Z" };
    const confirmation = {
      id: "answer",
      sender_id: "local",
      content: "Confirm generation",
      created_at: "2026-10-12T08:00:01Z",
    };
    const video = {
      id: "video",
      sender_id: "manager",
      content: "Animate a kitten",
      created_at: "2026-10-12T08:00:02Z",
      metadata: { video_generation: { state: "generating" }, codex: { turn_message_id: "turn" } },
    };
    const messages = [turn, confirmation, video];
    const { container, rerender } = render(
      <ConversationMessageList
        {...props}
        conversation={{ ...props.conversation, messages }}
        visibleMessages={messages}
      />,
    );
    const messageIDs = () =>
      Array.from(container.querySelectorAll("[data-message-id]"), (node) => node.getAttribute("data-message-id"));
    expect(messageIDs()).toEqual(["turn", "answer", "video"]);
    const cardMessage = container.querySelector('[data-message-id="video"]');
    const finalMessages = [confirmation, video, { ...turn, content: "Submitted", created_at: "2026-10-12T08:00:03Z" }];
    rerender(
      <ConversationMessageList
        {...props}
        conversation={{ ...props.conversation, messages: finalMessages }}
        visibleMessages={finalMessages}
      />,
    );
    expect(messageIDs()).toEqual(["answer", "video", "turn"]);
    expect(container.querySelector('[data-message-id="video"]')).toBe(cardMessage);
  });

  it.each([
    [
      "agent_archived",
      "Worker 因智能体归档已被移出协作空间",
      "Worker was removed from the collaboration space because the agent was archived",
    ],
    ["agent_restored", "Worker 已恢复并加入协作空间", "Worker was restored and rejoined the collaboration space"],
    [
      "agent_deleted",
      "Worker 因智能体删除已被移出协作空间",
      "Worker was removed from the collaboration space because the agent was deleted",
    ],
  ])("renders %s as a centered system notice and updates it when the language changes", (key, zh, en) => {
    const props = messageListProps();
    const messages = [
      {
        id: `notice-${key}`,
        sender_id: "admin",
        kind: "event",
        content: en,
        event: { key, actor_id: "admin", target_ids: ["user-worker"] },
      },
    ];
    const usersById = new Map(props.usersById).set("user-worker", { id: "user-worker", name: "Worker" });
    const { rerender } = render(
      <ConversationMessageList
        {...props}
        locale="zh"
        usersById={usersById}
        conversation={{ ...props.conversation, is_direct: false, messages }}
        visibleMessages={messages}
      />,
    );
    expect(screen.getByText(zh).closest(".message-event-row")).not.toBeNull();
    expect(screen.queryByRole("button", { name: /profilePreview/ })).not.toBeInTheDocument();
    rerender(
      <ConversationMessageList
        {...props}
        locale="en"
        usersById={usersById}
        conversation={{ ...props.conversation, is_direct: false, messages }}
        visibleMessages={messages}
      />,
    );
    expect(screen.getByText(en).closest(".message-event-row")).not.toBeNull();
    expect(screen.queryByText(zh)).not.toBeInTheDocument();
  });

  it("keeps history unchanged during composer updates and invokes the latest event handlers", () => {
    const props = messageListProps();
    const renderMessageFooter = vi.fn(() => null);
    const firstPreview = vi.fn();
    const { rerender } = render(
      <ConversationMessageList {...props} renderMessageFooter={renderMessageFooter} onPreviewUser={firstPreview} />,
    );
    expect(renderMessageFooter).toHaveBeenCalledTimes(100);
    const latestPreview = vi.fn();
    const latestThread = vi.fn();
    for (let index = 0; index < 10; index++) {
      rerender(
        <ConversationMessageList
          {...props}
          renderMessageFooter={renderMessageFooter}
          onPreviewUser={latestPreview}
          onOpenThread={latestThread}
          onMessageAction={() => undefined}
          onCitationSelect={() => undefined}
          onPreviewAttachment={() => undefined}
        />,
      );
    }
    // Adding optional handlers changes the rendered capabilities once.
    expect(renderMessageFooter).toHaveBeenCalledTimes(200);
    fireEvent.click(screen.getAllByRole("button", { name: "profilePreview Manager" })[0]);
    expect(latestPreview).toHaveBeenCalledWith(props.usersById.get("manager"), expect.any(HTMLElement));
    expect(firstPreview).not.toHaveBeenCalled();
    fireEvent.click(screen.getAllByRole("button", { name: "replyInThread" })[0]);
    expect(latestThread).toHaveBeenCalledWith(props.visibleMessages[0]);
  });

  it("keeps failed answers retryable and hides the confirmation as soon as submission succeeds", async () => {
    const user = userEvent.setup();
    const respond = vi
      .spyOn(activitiesAPI, "respondToUserInput")
      .mockRejectedValueOnce(new Error("Try again"))
      .mockResolvedValueOnce({ id: "layout-request", status: "answered", questions: [] });
    const props = messageListProps();
    const question = {
      id: "choice-message",
      sender_id: "manager",
      content: "## Questions\n\nPick a layout",
      metadata: {
        codex: { turn_id: "layout-turn" },
        csgclaw: {
          agent_activity: {
            type: CSGCLAW_AGENT_ACTIVITY_TYPE,
            content: {
              msgtype: AgentActivityMsgTypes.question,
              question: {
                id: "layout-request",
                status: "pending",
                questions: [
                  { id: "layout", header: "Layout", question: "Pick a layout", options: [{ label: "Minimal" }] },
                ],
              },
            },
          },
        },
      },
    };
    props.usersById.set("user", { id: "user", name: "User" });
    const userPrompt = { id: "prompt", sender_id: "user", content: "Format the article" };
    const answer = {
      id: "answer",
      sender_id: "user",
      content: "Minimal selected",
      metadata: {
        csgclaw: { request_user_input: { kind: "answer", request_id: "layout-request" } },
      },
    };
    const final = {
      id: "final",
      sender_id: "manager",
      content: "Article formatted",
      created_at: "2026-11-25T02:00:03Z",
      metadata: { codex: { delivery_kind: "final" } },
    };
    const settled = structuredClone(question);
    Object.assign(settled.metadata.csgclaw.agent_activity.content.question, {
      status: "answered",
      resolved_at: "2026-11-25T02:00:02Z",
    });
    const view = (messages: ConversationMessageListProps["visibleMessages"]) => (
      <ConversationMessageList
        {...props}
        conversation={{ ...props.conversation, messages }}
        visibleMessages={messages}
      />
    );
    const { rerender } = render(view([userPrompt, question]));
    const option = await screen.findByRole("radio", { name: /Minimal/ });
    expect(option.closest("[data-message-id=choice-message]")).not.toBeNull();
    await user.click(option);
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(option).toHaveAttribute("aria-checked", "true");
    await user.click(option);
    await waitFor(() => expect(option.closest("[data-message-id=choice-message]")).not.toBeInTheDocument());
    expect(respond).toHaveBeenCalledTimes(2);

    const ordinaryMessage = { id: "ordinary", sender_id: "user", content: "## 回答\n\n我选极简风" };
    rerender(view([userPrompt, settled, answer, ordinaryMessage]));
    expect(screen.getByText("我选极简风")).toBeInTheDocument();
    expect(screen.queryByText("Minimal selected")).not.toBeInTheDocument();
    expect(screen.queryByText("Pick a layout")).not.toBeInTheDocument();
    rerender(view([userPrompt, settled, answer, ordinaryMessage, final]));
    expect(screen.getByText("Format the article")).toBeInTheDocument();
    expect(screen.getByText("Article formatted")).toBeInTheDocument();
    expect(screen.queryByText("Minimal selected")).not.toBeInTheDocument();
    expect(screen.queryByText("Pick a layout")).not.toBeInTheDocument();
    respond.mockRestore();
  });

  it("renders new history, translation and theme changes without losing existing messages", () => {
    const props = messageListProps();
    const { rerender } = render(<ConversationMessageList {...props} />);
    const newMessage = { id: "latest", sender_id: "manager", content: "New reply" };
    const messages = [...props.visibleMessages, newMessage];
    rerender(
      <ConversationMessageList
        {...props}
        conversation={{ ...props.conversation, messages }}
        visibleMessages={messages}
        theme="dark"
        t={(key) => `translated ${key}`}
      />,
    );
    expect(screen.getByText("New reply")).toBeInTheDocument();
    expect(screen.getByText("Reply 0")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "translated profilePreview Manager" })).toHaveLength(101);
  });
});

describe("private message memory marks", () => {
  it("marks the source and its final reply and forgets their shared source", async () => {
    const props = messageListProps();
    const source = { id: "source", sender_id: "user-admin", content: "Remember my report preference" };
    const reply = {
      id: "reply",
      sender_id: "user-manager",
      content: "Saved",
      metadata: {
        csgclaw: { delivery_kind: "final" },
        diagnostics: { room_id: "room", source_id: "source" },
      },
    };
    const marks = vi.spyOn(memoryAPI, "fetchAgentMemoryMarks").mockResolvedValue({
      available: true,
      enabled: true,
      revision: 1,
      sources: [{ room_id: "room", message_id: "source" }],
    });
    const forget = vi.spyOn(memoryAPI, "forgetAgentMemory").mockResolvedValue({
      available: true,
      enabled: true,
      ready: false,
      name: "MEMORY.md",
      revision: 2,
      entries: [],
      content: "",
      status: "idle",
    });
    try {
      const { container } = render(
        <ConversationMessageList
          {...props}
          agents={[{ id: "agent-manager", name: "manager", user_id: "user-manager", memory_supported: true }]}
          currentUserID="user-admin"
          usersById={
            new Map([
              ["user-admin", { id: "user-admin", name: "User" }],
              ["user-manager", { id: "user-manager", name: "Manager" }],
            ])
          }
          conversation={{
            id: "room",
            is_direct: true,
            members: ["user-admin", "user-manager"],
            messages: [source, reply],
          }}
          visibleMessages={[source, reply]}
        />,
      );
      expect(await screen.findAllByText("agentMemoryUpdated")).toHaveLength(2);
      const replyRow = container.querySelector('[data-message-id="reply"]') as HTMLElement;
      fireEvent.click(within(replyRow).getByRole("button", { name: "agentMemoryForget" }));
      expect(await within(replyRow).findByText("agentMemoryForgotten")).toBeInTheDocument();
      expect(forget).toHaveBeenCalledWith("agent-manager", "room", "source");
    } finally {
      marks.mockRestore();
      forget.mockRestore();
    }
  });
});
