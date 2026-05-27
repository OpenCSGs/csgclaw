import { createRef, useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationPane } from "@/pages/ConversationPage/components/ConversationPane/ConversationPane";
import type { IMConversation, IMUser, ThreadView, TranslateFn } from "@/models/conversations";

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

const t: TranslateFn = (key, params = {}) => {
  const labels: Record<string, string> = {
    close: "Close",
    composerTip: "Enter to send",
    directMessagesSection: "Direct Messages",
    inputPlaceholder: "Message",
    latestThreadReply: "Latest reply",
    replyInThread: "Reply in thread",
    send: "Send",
    threadComposerPlaceholder: "Reply in thread",
    threadNoReplies: "No replies",
    threadPanelTitle: "Thread",
  };
  if (key === "threadReplies") {
    return `${params.count ?? 0} replies`;
  }
  return labels[key] ?? key;
};

function renderThreadPane(conversationMembers = users, onPreviewUser = vi.fn()) {
  const root = {
    content: "Hi! How can I help you today?",
    created_at: "2026-05-25T08:13:00Z",
    id: "msg-root",
    sender_id: "u-manager",
  };
  const conversation: IMConversation = {
    id: "room-1",
    is_direct: true,
    members: conversationMembers.map((user) => user.id),
    messages: [root],
    title: "manager",
  };
  const thread: ThreadView = {
    replies: [],
    room_id: "room-1",
    root,
    summary: {
      context_summary: { root_excerpt: root.content },
      reply_count: 0,
      root_id: root.id,
    },
  };

  function Harness() {
    const [threadDraft, setThreadDraft] = useState("");
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
        managerProfile={null}
        managerProfileIncomplete={false}
        memberMenuRef={createRef<HTMLDivElement>()}
        mentionCandidates={[]}
        mentionIndex={0}
        mentionableUsersByHandle={new Map([["manager", users[1]]])}
        messageActionBusy={false}
        messageActionError=""
        messageListRef={createRef<HTMLElement>()}
        onApplyMention={() => {}}
        onCloseThread={() => {}}
        onComposerCompositionEnd={() => {}}
        onComposerCompositionStart={() => {}}
        onComposerKeyDown={() => {}}
        onDeleteRoom={() => {}}
        onInviteAction={() => {}}
        onMessageAction={() => {}}
        onOpenThread={() => {}}
        onProviderLogin={() => {}}
        onPreviewUser={onPreviewUser}
        onSendMessage={() => {}}
        onSendThreadReply={() => {}}
        onSyncComposer={() => {}}
        onToggleChannelTools={() => {}}
        onToggleMemberList={() => {}}
        onToggleToolCalls={() => {}}
        selectedMessageCount={1}
        showChannelTools={false}
        showMemberList={false}
        showToolCalls={false}
        t={t}
        theme="light"
        threadDraft={threadDraft}
        threadError=""
        threadLoading={false}
        onThreadDraftChange={setThreadDraft}
        usersById={usersById}
        visibleMessages={[root]}
      />
    );
  }

  return render(<Harness />);
}

describe("ConversationPane", () => {
  it("offers mention choices inside the thread composer", async () => {
    const user = userEvent.setup();
    renderThreadPane();

    await user.type(screen.getByPlaceholderText("Reply in thread"), "@");

    expect(screen.getByText("@manager")).toBeInTheDocument();
  });

  it("keeps keyboard selection while navigating thread mention choices", async () => {
    const user = userEvent.setup();
    renderThreadPane();

    await user.type(screen.getByPlaceholderText("Reply in thread"), "@");
    await user.keyboard("{ArrowDown}");

    expect(screen.getByText("@manager").closest("button")).toHaveClass("active");
  });

  it("scrolls the active thread mention option into view while navigating", async () => {
    const user = userEvent.setup();
    const scrollIntoView = vi.fn();
    const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    try {
      renderThreadPane(roomUsers);

      await user.type(screen.getByPlaceholderText("Reply in thread"), "@");
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
    renderThreadPane(users, onPreviewUser);

    const threadPanel = screen.getByRole("complementary", { name: "Thread" });
    await user.click(within(threadPanel).getByRole("button", { name: "profilePreview manager" }));

    expect(onPreviewUser).toHaveBeenCalledWith(users[1], expect.any(HTMLElement));
  });
});
