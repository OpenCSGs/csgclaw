import { createRef, useRef, useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationPane } from "@/pages/ConversationPage/components/ConversationPane/ConversationPane";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import type { IMConversation, IMUser, ThreadView, TranslateFn } from "@/models/conversations";
import {
  getCollapsedSelectionTextOffset,
  parseComposerSegments,
  segmentsToPlainText,
  type ComposerSegment,
} from "@/models/composer";

const users: IMUser[] = [
  {
    accent_hex: "#8b1d2c",
    avatar: "AD",
    handle: "ylong",
    id: "u-admin",
    name: "本地用户",
    role: "admin",
  },
  {
    accent_hex: "#0f5b66",
    avatar: "MG",
    handle: "manager",
    id: "u-manager",
    name: "manager",
    role: "worker",
  },
];

const roomUsers: IMUser[] = [
  users[0],
  {
    accent_hex: "#4f2ec7",
    avatar: "D",
    handle: "dev",
    id: "u-dev",
    name: "dev",
    role: "worker",
  },
  users[1],
  {
    accent_hex: "#1f57c8",
    avatar: "Q",
    handle: "qa",
    id: "u-qa",
    name: "qa",
    role: "worker",
  },
  {
    accent_hex: "#047857",
    avatar: "U",
    handle: "ux",
    id: "u-ux",
    name: "ux",
    role: "worker",
  },
  {
    accent_hex: "#0f5b66",
    avatar: "S",
    handle: "sales",
    id: "u-sales",
    name: "sales",
    role: "worker",
  },
];

const usersById = new Map(users.map((user) => [user.id, user]));

function toolActivityContent(summary: string) {
  return JSON.stringify({
    type: CSGCLAW_AGENT_ACTIVITY_TYPE,
    content: {
      msgtype: AgentActivityMsgTypes.tool,
      body: "Tool running",
      tool: {
        id: "tool-1",
        input_summary: summary,
        kind: "execute",
        status: "running",
        title: "Run shell command",
      },
    },
  });
}

const t: TranslateFn = (key, params = {}) => {
  const labels: Record<string, string> = {
    close: "Close",
    composerTip: "Enter to send",
    directMessagesSection: "Direct Messages",
    inputPlaceholder: "Message",
    latestThreadReply: "Latest reply",
    replyInThread: "Reply in thread",
    send: "Send",
    timestampToday: "Today",
    timestampYesterday: "Yesterday",
    threadComposerPlaceholder: "Reply in thread",
    threadNoReplies: "No replies",
    threadPanelTitle: "Thread",
  };
  if (key === "threadReplies") {
    return `${params.count ?? 0} replies`;
  }
  return labels[key] ?? key;
};

function renderThreadPane({
  conversationMembers = users,
  isDirect = true,
  messages,
  onClearRoomMessages = vi.fn(),
  onDeleteRoom = vi.fn(),
  onPreviewUser = vi.fn(),
  replies = [],
  showToolCalls = false,
  mentionCandidates = [],
  mentionIndex = 0,
  onApplyMention = vi.fn(),
}: {
  conversationMembers?: IMUser[];
  isDirect?: boolean;
  messages?: IMConversation["messages"];
  mentionCandidates?: IMUser[];
  mentionIndex?: number;
  onClearRoomMessages?: (id: string) => void;
  onDeleteRoom?: (id: string) => void;
  onApplyMention?: (user: IMUser) => void;
  onPreviewUser?: (user: IMUser) => void;
  replies?: ThreadView["replies"];
  showToolCalls?: boolean;
} = {}) {
  const root = {
    content: "Hi! How can I help you today?",
    created_at: "2026-05-25T08:13:00Z",
    id: "msg-root",
    sender_id: "u-manager",
  };
  const timelineMessages = messages || [root];
  const conversation: IMConversation = {
    id: "room-1",
    is_direct: isDirect,
    members: conversationMembers.map((user) => user.id),
    messages: timelineMessages,
    title: "manager",
  };
  const thread: ThreadView = {
    replies,
    room_id: "room-1",
    root,
    summary: {
      context_summary: { root_excerpt: root.content },
      reply_count: 0,
      root_id: root.id,
    },
  };

  function Harness() {
    const [showChannelTools, setShowChannelTools] = useState(false);
    const [showMemberList, setShowMemberList] = useState(false);
    const [threadDraftSegments, setThreadDraftSegments] = useState<ComposerSegment[]>([]);
    return (
      <ConversationPane
        activeThreadRootID="msg-root"
        activeThreadView={thread}
        authBusyProvider=""
        authStatuses={{}}
        channelToolsRef={createRef<HTMLDivElement>()}
        composerError=""
        conversation={conversation}
        conversationMembers={conversationMembers}
        currentUserID="u-admin"
        draftSegments={[]}
        draftText=""
        editorRef={createRef<HTMLDivElement>()}
        inviteActionLabel="Invite"
        locale="zh"
        logAgent={null}
        managerProfile={null}
        managerProfileIncomplete={false}
        memberMenuRef={createRef<HTMLDivElement>()}
        mentionCandidates={mentionCandidates}
        mentionIndex={mentionIndex}
        mentionableUsersByHandle={new Map([["manager", users[1]]])}
        messageActionBusy=""
        messageActionError={{}}
        messageListRef={createRef<HTMLElement>()}
        onApplyMention={onApplyMention}
        onClearRoomMessages={onClearRoomMessages}
        onCloseThread={() => {}}
        onComposerCompositionEnd={() => {}}
        onComposerCompositionStart={() => {}}
        onComposerKeyDown={() => {}}
        onDeleteRoom={onDeleteRoom}
        onInviteAction={() => {}}
        onMessageAction={() => {}}
        onOpenThread={() => {}}
        onProviderLogin={() => {}}
        onPreviewUser={onPreviewUser}
        onSendMessage={() => {}}
        onSendThreadReply={() => {}}
        onSyncComposer={() => {}}
        onToggleChannelTools={setShowChannelTools}
        onToggleMemberList={setShowMemberList}
        onToggleToolCalls={() => {}}
        selectedMessageCount={timelineMessages.length}
        showChannelTools={showChannelTools}
        showMemberList={showMemberList}
        showToolCalls={showToolCalls}
        t={t}
        theme="light"
        threadDraftSegments={threadDraftSegments}
        threadError=""
        threadLoading={false}
        onThreadDraftChange={setThreadDraftSegments}
        usersById={usersById}
        visibleMessages={timelineMessages}
      />
    );
  }

  return render(<Harness />);
}

describe("ConversationPane", () => {
  it("keeps the caret at the end after slash text is tokenized in the main composer", async () => {
    const user = userEvent.setup();
    const conversation: IMConversation = {
      id: "dm-1",
      is_direct: true,
      members: users.map((item) => item.id),
      messages: [],
      title: "manager",
    };

    function Harness() {
      const [showChannelTools, setShowChannelTools] = useState(false);
      const [showMemberList, setShowMemberList] = useState(false);
      const [draftSegments, setDraftSegments] = useState<ComposerSegment[]>([]);
      const editorRef = useRef<HTMLDivElement | null>(null);

      return (
        <ConversationPane
          activeThreadRootID=""
          activeThreadView={null}
          authBusyProvider=""
          authStatuses={{}}
          channelToolsRef={createRef<HTMLDivElement>()}
          composerError=""
          conversation={conversation}
          conversationMembers={users}
          currentUserID="u-admin"
          draftSegments={draftSegments}
          draftText={segmentsToPlainText(draftSegments)}
          editorRef={editorRef}
          inviteActionLabel="Invite"
          locale="zh"
          logAgent={{ id: "u-manager", name: "manager" }}
          managerProfile={null}
          managerProfileIncomplete={false}
          memberMenuRef={createRef<HTMLDivElement>()}
          mentionCandidates={[]}
          mentionIndex={0}
          mentionableUsersByHandle={new Map()}
          messageActionBusy=""
          messageActionError={{}}
          messageListRef={createRef<HTMLElement>()}
          onApplyMention={() => {}}
          onApplySlashCandidate={() => {}}
          onClearRoomMessages={() => {}}
          onCloseThread={() => {}}
          onComposerCompositionEnd={() => {}}
          onComposerCompositionStart={() => {}}
          onComposerKeyDown={() => {}}
          onDeleteRoom={() => {}}
          onInviteAction={() => {}}
          onMessageAction={() => {}}
          onOpenThread={() => {}}
          onProviderLogin={() => {}}
          onPreviewUser={() => {}}
          onSendMessage={() => {}}
          onSendThreadReply={() => {}}
          onSyncComposer={() => {
            const editor = editorRef.current;
            if (!editor) {
              return;
            }
            setDraftSegments(parseComposerSegments(editor) as ComposerSegment[]);
          }}
          onToggleChannelTools={setShowChannelTools}
          onToggleMemberList={setShowMemberList}
          onToggleToolCalls={() => {}}
          selectedMessageCount={0}
          showChannelTools={showChannelTools}
          showMemberList={showMemberList}
          showToolCalls={false}
          slashCandidates={[]}
          slashIndex={0}
          slashPickerLoading={false}
          slashPickerOpen={false}
          t={t}
          theme="light"
          threadDraftSegments={[]}
          threadError=""
          threadLoading={false}
          onThreadDraftChange={() => {}}
          usersById={usersById}
          visibleMessages={[]}
        />
      );
    }

    render(<Harness />);
    const editor = screen.getByLabelText("Message");

    await user.click(editor);
    await user.type(editor, "/abc");

    expect(editor).toHaveTextContent("/abc");
    expect(getCollapsedSelectionTextOffset(editor)).toBe(4);
  });

  it("shows one date divider per day without times", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-03T12:00:00+08:00"));

    try {
      const { container } = renderThreadPane({
        messages: [
          {
            content: "Morning",
            created_at: "2026-05-11T10:25:00+08:00",
            id: "msg-morning",
            sender_id: "u-manager",
          },
          {
            content: "Afternoon",
            created_at: "2026-05-11T16:45:00+08:00",
            id: "msg-afternoon",
            sender_id: "u-manager",
          },
          {
            content: "Next day",
            created_at: "2026-05-12T09:15:00+08:00",
            id: "msg-next-day",
            sender_id: "u-manager",
          },
        ],
      });

      expect(
        [...container.querySelectorAll(".message-time-divider-label")].map((item) => ({
          tooltip: item.getAttribute("data-tooltip"),
          text: item.textContent,
          title: item.getAttribute("title"),
        })),
      ).toEqual([
        { text: "5月11日", title: "2026-05-11 10:25:00", tooltip: "2026-05-11 10:25:00" },
        { text: "5月12日", title: "2026-05-12 09:15:00", tooltip: "2026-05-12 09:15:00" },
      ]);
      expect(container.querySelector(".message-row .message-timestamp")).toHaveAttribute(
        "data-tooltip",
        "2026-05-11 10:25:00",
      );
      expect(
        [...container.querySelectorAll(".message-row .message-timestamp")].map((item) => item.textContent),
      ).toEqual(["10:25", "16:45", "09:15"]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("offers mention choices inside the thread composer", async () => {
    const user = userEvent.setup();
    renderThreadPane();

    const threadComposer = within(screen.getByRole("complementary", { name: "Thread" })).getByRole("textbox", {
      name: "Reply in thread",
    });
    await user.type(threadComposer, "@");

    expect(screen.getByText("@manager")).toBeInTheDocument();
  });

  it("renders main composer mention choices as clickable options", async () => {
    const user = userEvent.setup();
    const onApplyMention = vi.fn();
    renderThreadPane({
      mentionCandidates: roomUsers,
      mentionIndex: 1,
      onApplyMention,
    });

    const options = screen.getAllByRole("option");
    expect(options).toHaveLength(roomUsers.length);
    expect(options[1]).toHaveClass("mention-option", "active");
    expect(options[1]).toHaveAttribute("aria-selected", "true");

    await user.click(options[2]);
    expect(onApplyMention).toHaveBeenCalledWith(roomUsers[2]);
  });

  it("keeps keyboard selection while navigating thread mention choices", async () => {
    const user = userEvent.setup();
    renderThreadPane();

    const threadComposer = within(screen.getByRole("complementary", { name: "Thread" })).getByRole("textbox", {
      name: "Reply in thread",
    });
    await user.type(threadComposer, "@");
    await user.keyboard("{ArrowDown}");

    expect(screen.getByText("@manager").closest("button")).toHaveClass("active");
  });

  it("scrolls the active thread mention option into view while navigating", async () => {
    const user = userEvent.setup();
    const scrollIntoView = vi.fn();
    const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    try {
      renderThreadPane({ conversationMembers: roomUsers });

      const threadComposer = within(screen.getByRole("complementary", { name: "Thread" })).getByRole("textbox", {
        name: "Reply in thread",
      });
      await user.type(threadComposer, "@");
      await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}");

      expect(screen.getByText("@sales").closest("button")).toHaveClass("active");
      expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest" });
    } finally {
      HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
    }
  });

  it("opens profile preview from thread message avatars", async () => {
    const user = userEvent.setup();
    const onPreviewUser = vi.fn();
    renderThreadPane({ conversationMembers: users, onPreviewUser });

    const threadPanel = screen.getByRole("complementary", { name: "Thread" });
    await user.click(within(threadPanel).getByRole("button", { name: "profilePreview manager" }));

    expect(onPreviewUser).toHaveBeenCalledWith(users[1], expect.any(HTMLElement));
  });

  it("hides tool-call replies in the thread panel when tool calls are off", () => {
    renderThreadPane({
      replies: [
        {
          content: toolActivityContent("hidden shell output"),
          created_at: "2026-05-25T08:14:00Z",
          id: "msg-tool",
          sender_id: "u-manager",
        },
        {
          content: "Visible answer",
          created_at: "2026-05-25T08:15:00Z",
          id: "msg-answer",
          sender_id: "u-manager",
        },
      ],
      showToolCalls: false,
    });

    const threadPanel = screen.getByRole("complementary", { name: "Thread" });
    expect(within(threadPanel).queryByText("hidden shell output")).not.toBeInTheDocument();
    expect(within(threadPanel).getByText("Visible answer")).toBeInTheDocument();
    expect(within(threadPanel).getByText("1 replies")).toBeInTheDocument();
  });

  it("confirms before clearing room messages from the tools menu", async () => {
    const user = userEvent.setup();
    const onClearRoomMessages = vi.fn();
    const onDeleteRoom = vi.fn();
    renderThreadPane({ isDirect: false, onClearRoomMessages, onDeleteRoom });

    await user.click(screen.getByRole("button", { name: "channelTools" }));
    await user.click(screen.getByRole("button", { name: "clearRoomMessages" }));

    expect(onClearRoomMessages).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "clearRoomMessagesConfirm" }));

    expect(onClearRoomMessages).toHaveBeenCalledWith("room-1");
    expect(onDeleteRoom).not.toHaveBeenCalled();
  });

  it("confirms before deleting a room from the tools menu", async () => {
    const user = userEvent.setup();
    const onClearRoomMessages = vi.fn();
    const onDeleteRoom = vi.fn();
    renderThreadPane({ isDirect: false, onClearRoomMessages, onDeleteRoom });

    await user.click(screen.getByRole("button", { name: "channelTools" }));
    await user.click(screen.getByRole("button", { name: "deleteRoom" }));

    expect(onDeleteRoom).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "deleteRoomConfirm" }));

    expect(onDeleteRoom).toHaveBeenCalledWith("room-1");
    expect(onClearRoomMessages).not.toHaveBeenCalled();
  });
});
