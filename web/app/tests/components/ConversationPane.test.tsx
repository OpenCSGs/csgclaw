import { createRef, useRef, useState } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationPane } from "@/pages/ConversationPage/components/ConversationPane";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import {
  CONVERSATION_ACTIVITY_ACTION_SEEN_STORAGE_KEY,
  CONVERSATION_ACTIVITY_PANEL_WIDTH_STORAGE_KEY,
} from "@/shared/storage/keys";
import type { ConversationPaneProps } from "@/components/business/ConversationPane";
import type { IMConversation, IMUser, ThreadView, TranslateFn } from "@/models/conversations";
import {
  getCollapsedSelectionTextOffset,
  parseComposerSegments,
  segmentsToPlainText,
  type ComposerSegment,
} from "@/models/composer";

vi.mock("@/api/im", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/im")>();
  return { ...actual, fetchMessagesRequest: vi.fn(async () => []) };
});

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: vi.fn(() => () => undefined),
}));

const users: IMUser[] = [
  {
    accent_hex: "#8b1d2c",
    avatar: "AD",
    id: "u-admin",
    name: "本地用户",
    role: "admin",
  },
  {
    accent_hex: "#0f5b66",
    avatar: "MG",
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
    id: "u-dev",
    name: "dev",
    role: "worker",
  },
  users[1],
  {
    accent_hex: "#1f57c8",
    avatar: "Q",
    id: "u-qa",
    name: "qa",
    role: "worker",
  },
  {
    accent_hex: "#047857",
    avatar: "U",
    id: "u-ux",
    name: "ux",
    role: "worker",
  },
  {
    accent_hex: "#0f5b66",
    avatar: "S",
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
    localIdentityFallback: "Local user",
    agentWorking: "{name} is working",
    agentActivityEmpty: "No activity yet",
    agentActivityChronological: "Chronological",
    agentActivityEventsCount: "{count} events",
    agentActivityLoadFailed: "Failed to load activity",
    agentActivityLoading: "Loading activity",
    agentActivityNoFilteredResults: "No matching activity",
    agentActivitySummaryLabel: "Activity summary",
    agentActivitySortLabel: "Activity order",
    agentActivityTimelineLabel: "Activity timeline",
    agentActivityTitle: "Activity",
    agentActivityToolCallsCount: "{count} tool calls",
    agentActivityNewestFirst: "Newest first",
    conversationActivityOpen: "Activity records",
    conversationActivityView: "View activity",
    conversationActivityActorFilter: "Filter activity by participant",
    conversationActivityAllActors: "All",
    conversationActivityRoomTimeline: "Current conversation · {name} · Room activity",
    conversationActivityResize: "Resize activity side panel",
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
  const label = labels[key] ?? key;
  return Object.entries(params).reduce((text, [name, value]) => text.replace(`{${name}}`, String(value)), label);
};

function renderThreadPane({
  agents = [],
  conversationMembers = users,
  isDirect = true,
  messages,
  onClearRoomMessages = vi.fn(),
  onCloseThread = vi.fn(),
  onDeleteRoom = vi.fn(),
  onCancelProfilePreviewClose = vi.fn(),
  onCloseProfilePreview = vi.fn(),
  onOpenAgentDetail = vi.fn(),
  onPreviewUser = vi.fn(),
  onRemoveMember = vi.fn(),
  replies = [],
  showToolCalls = false,
  mentionCandidates = [],
  mentionIndex = 0,
  visibleMessages,
  memberActionBusyID = "",
  memberActionError = "",
  onClearMemberActionError = vi.fn(),
  onApplyMention = vi.fn(),
  agentDetailPanelProps = null,
  usersByIdOverride = usersById,
}: {
  agents?: NonNullable<ConversationPaneProps["agents"]>;
  agentDetailPanelProps?: ConversationPaneProps["agentDetailPanelProps"];
  conversationMembers?: IMUser[];
  isDirect?: boolean;
  messages?: IMConversation["messages"];
  visibleMessages?: IMConversation["messages"];
  mentionCandidates?: IMUser[];
  mentionIndex?: number;
  onClearRoomMessages?: (id: string) => void;
  onCloseThread?: () => void;
  onDeleteRoom?: (id: string) => void;
  onApplyMention?: (user: IMUser) => void;
  onCancelProfilePreviewClose?: () => void;
  onCloseProfilePreview?: () => void;
  onOpenAgentDetail?: NonNullable<ConversationPaneProps["onOpenAgentDetail"]>;
  onPreviewUser?: (user: IMUser) => void;
  onRemoveMember?: (memberID: string) => void;
  memberActionBusyID?: string;
  memberActionError?: string;
  onClearMemberActionError?: () => void;
  replies?: ThreadView["replies"];
  showToolCalls?: boolean;
  usersByIdOverride?: ConversationPaneProps["usersById"];
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
        agents={agents}
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
        mentionableUsersByName={new Map([["manager", users[1]]])}
        messageActionBusy=""
        messageActionFeedback={{}}
        messageListRef={createRef<HTMLElement>()}
        memberActionBusyID={memberActionBusyID}
        memberActionError={memberActionError}
        onApplyMention={onApplyMention}
        onClearRoomMessages={onClearRoomMessages}
        onClearMemberActionError={onClearMemberActionError}
        onCloseThread={onCloseThread}
        onComposerCompositionEnd={() => {}}
        onComposerCompositionStart={() => {}}
        onComposerKeyDown={() => {}}
        onDeleteRoom={onDeleteRoom}
        onInviteAction={() => {}}
        onMessageAction={() => {}}
        onCancelProfilePreviewClose={onCancelProfilePreviewClose}
        onCloseProfilePreview={onCloseProfilePreview}
        onOpenAgentDetail={onOpenAgentDetail}
        onOpenThread={() => {}}
        onRemoveMember={onRemoveMember}
        onProviderLogin={() => {}}
        onPreviewUser={onPreviewUser}
        onSendMessage={() => {}}
        onSendThreadReply={() => {}}
        onSyncComposer={() => {}}
        onToggleChannelTools={setShowChannelTools}
        onToggleMemberList={setShowMemberList}
        onToggleToolCalls={() => {}}
        selectedMessageCount={timelineMessages.length}
        agentDetailPanelProps={agentDetailPanelProps}
        showChannelTools={showChannelTools}
        showMemberList={showMemberList}
        showToolCalls={showToolCalls}
        t={t}
        theme="light"
        threadDraftSegments={threadDraftSegments}
        threadError=""
        threadLoading={false}
        onThreadDraftChange={setThreadDraftSegments}
        usersById={usersByIdOverride}
        visibleMessages={visibleMessages ?? timelineMessages}
      />
    );
  }

  return render(<Harness />);
}

describe("ConversationPane", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("opens the room activity side panel and closes the active thread", async () => {
    const user = userEvent.setup();
    const onCloseThread = vi.fn();

    renderThreadPane({
      agents: [{ id: "u-manager", name: "manager", role: "worker" }],
      isDirect: false,
      onCloseThread,
    });

    expect(document.querySelector(".thread-panel")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Activity records" }));

    expect(onCloseThread).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("heading", { name: "Activity" })).toBeInTheDocument();
    expect(document.querySelector(".conversation-activity-panel")).toBeInTheDocument();
    expect(document.querySelector(".thread-panel")).not.toBeInTheDocument();
  });

  it("shows newest activity first by default", async () => {
    const user = userEvent.setup();
    renderThreadPane({
      agents: [{ id: "u-manager", name: "manager", role: "worker" }],
      isDirect: false,
      messages: [
        { id: "older", sender_id: "u-manager", created_at: "2026-07-16T10:00:00Z", content: "older event" },
        { id: "newer", sender_id: "u-manager", created_at: "2026-07-16T10:01:00Z", content: "newer event" },
      ],
    });

    await user.click(screen.getByRole("button", { name: "Activity records" }));

    expect(screen.getByRole("button", { name: "Newest first" })).toHaveAttribute("aria-pressed", "true");
    const rows = within(document.querySelector(".conversation-activity-panel") as HTMLElement).getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("newer event");
    expect(rows[1]).toHaveTextContent("older event");
  });

  it("includes user prompts in the conversation activity timeline", async () => {
    const user = userEvent.setup();
    renderThreadPane({
      agents: [{ id: "u-manager", name: "manager", role: "worker" }],
      conversationMembers: [users[1]],
      isDirect: false,
      messages: [
        { id: "prompt", sender_id: "u-admin", created_at: "2026-07-16T10:00:00Z", content: "check Chengdu weather" },
        { id: "reply", sender_id: "u-manager", created_at: "2026-07-16T10:01:00Z", content: "checking now" },
      ],
    });

    await user.click(screen.getByRole("button", { name: "Activity records" }));

    const panel = document.querySelector(".conversation-activity-panel") as HTMLElement;
    expect(within(panel).getAllByText("message")).toHaveLength(2);
    expect(within(panel).getByText("check Chengdu weather")).toBeInTheDocument();
    expect(within(panel).getAllByText("本地用户")).toHaveLength(2);
    expect(within(panel).getByRole("button", { name: "All" })).toHaveAttribute("aria-pressed", "true");

    await user.click(within(panel).getByRole("button", { name: "本地用户" }));

    expect(within(panel).getByText("check Chengdu weather")).toBeInTheDocument();
    expect(within(panel).queryByText("checking now")).not.toBeInTheDocument();
  });

  it("restores and persists the activity panel width", async () => {
    window.localStorage.setItem(CONVERSATION_ACTIVITY_PANEL_WIDTH_STORAGE_KEY, "512");
    const user = userEvent.setup();
    renderThreadPane({
      agents: [{ id: "u-manager", name: "manager", role: "worker" }],
      isDirect: false,
    });

    await user.click(screen.getByRole("button", { name: "Activity records" }));
    const separator = screen.getByRole("separator", { name: "Resize activity side panel" });
    expect(separator).toHaveAttribute("aria-valuenow", "512");

    separator.focus();
    await user.keyboard("{ArrowLeft}");

    expect(separator).toHaveAttribute("aria-valuenow", "536");
    await waitFor(() => {
      expect(window.localStorage.getItem(CONVERSATION_ACTIVITY_PANEL_WIDTH_STORAGE_KEY)).toBe("536");
    });
  });

  it("hides legacy zero-reply thread summaries from the timeline", () => {
    const { container } = renderThreadPane({
      messages: [
        {
          content: "No reply yet",
          created_at: "2026-07-14T09:29:00Z",
          id: "msg-root",
          sender_id: "u-manager",
          thread: {
            reply_count: 0,
            root_id: "msg-root",
          },
        },
      ],
    });

    expect(container.querySelector(".messages .message-thread-actions")).not.toBeInTheDocument();
  });

  it("shows visible and total message counts when some messages are filtered", () => {
    const messages = [
      {
        content: "visible answer",
        created_at: "2026-05-25T08:13:00Z",
        id: "msg-visible",
        sender_id: "u-manager",
      },
      {
        content: toolActivityContent("hidden command"),
        created_at: "2026-05-25T08:14:00Z",
        id: "msg-hidden-tool",
        sender_id: "u-manager",
      },
    ];

    renderThreadPane({ messages, visibleMessages: [messages[0]] });

    expect(screen.getByText("1/2")).toBeInTheDocument();
  });

  it("previews human message avatars on hover without opening details on click", async () => {
    const user = userEvent.setup();
    const onCancelProfilePreviewClose = vi.fn();
    const onCloseProfilePreview = vi.fn();
    const onOpenAgentDetail = vi.fn();
    const onPreviewUser = vi.fn();

    renderThreadPane({
      onCancelProfilePreviewClose,
      onCloseProfilePreview,
      onOpenAgentDetail,
      onPreviewUser,
    });

    const avatar = screen.getAllByRole("button", { name: "profilePreview manager" })[0] as HTMLElement;
    await user.hover(avatar);

    expect(onCancelProfilePreviewClose).toHaveBeenCalled();
    expect(onPreviewUser).toHaveBeenCalledWith(expect.objectContaining({ id: "u-manager" }), avatar);

    await user.unhover(avatar);
    expect(onCloseProfilePreview).toHaveBeenCalled();

    await user.click(avatar);
    expect(onOpenAgentDetail).not.toHaveBeenCalled();
  });

  it("opens agent details instead of profile details when clicking an agent avatar", async () => {
    const user = userEvent.setup();
    const onOpenAgentDetail = vi.fn();
    const onPreviewUser = vi.fn();

    renderThreadPane({
      agents: [{ id: "u-manager", name: "manager", role: "worker" }],
      onOpenAgentDetail,
      onPreviewUser,
    });

    const avatar = screen.getAllByRole("button", { name: "profilePreview manager" })[0] as HTMLElement;
    await user.hover(avatar);
    expect(onPreviewUser).toHaveBeenCalledWith(expect.objectContaining({ id: "u-manager" }), avatar);

    await user.click(avatar);
    expect(onOpenAgentDetail).toHaveBeenCalledWith(expect.objectContaining({ id: "u-manager" }), avatar);
  });

  it("opens agent details when the message sender id is a channel identity alias", async () => {
    const user = userEvent.setup();
    const weatherUser: IMUser = {
      avatar: "O",
      id: "user-openclaw-weather",
      name: "openclaw-weather",
      role: "worker",
    };
    const userAliases = new Map<string, IMUser>([
      ...usersById,
      [weatherUser.id, weatherUser],
      ["openclaw-weather", weatherUser],
    ]);
    const onOpenAgentDetail = vi.fn();

    renderThreadPane({
      agents: [{ id: "openclaw-weather", name: "OpenClaw weather", role: "worker" }],
      conversationMembers: [users[0], weatherUser],
      messages: [
        {
          content: "Weather report",
          created_at: "2026-05-25T08:13:00Z",
          id: "msg-weather",
          sender_id: "openclaw-weather",
        },
      ],
      onOpenAgentDetail,
      usersByIdOverride: userAliases,
    });

    await user.click(screen.getByRole("button", { name: "profilePreview openclaw-weather" }));

    expect(onOpenAgentDetail).toHaveBeenCalledWith(
      expect.objectContaining({ id: "openclaw-weather" }),
      expect.any(HTMLElement),
    );
  });

  it("renders agent details as a modal drawer with keyboard dismissal", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const onOpenDM = vi.fn();
    const { container } = renderThreadPane({
      agentDetailPanelProps: {
        item: {
          id: "u-manager",
          name: "manager",
          role: "worker",
        },
        t,
        onClose,
        onDelete: vi.fn(),
        onInvite: vi.fn(),
        onOpenDM,
        onRecreate: vi.fn(),
        onStart: vi.fn(),
        onStop: vi.fn(),
      },
    });

    const dialog = screen.getByRole("dialog", { name: "agentDetailPanel" });
    const closeButton = within(dialog).getByRole("button", { name: /close/i });
    const backdrop = document.querySelector(".agent-detail-drawer-backdrop");

    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(container).not.toContainElement(dialog);
    expect(backdrop).toBeInTheDocument();
    expect(dialog.querySelector(".agent-detail-side-panel-bar")?.firstElementChild).toBe(closeButton);
    await waitFor(() => expect(closeButton).toHaveFocus());

    await user.click(closeButton);
    expect(onClose).toHaveBeenCalledTimes(1);

    onClose.mockClear();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);

    onClose.mockClear();
    await user.click(within(dialog).getByRole("button", { name: "openDM" }));
    expect(onClose).toHaveBeenCalledWith(false);
    expect(onOpenDM).toHaveBeenCalledWith(expect.objectContaining({ id: "u-manager" }));
  });

  it("keeps agent details open when unsaved changes block direct-message navigation", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn(() => false);
    const onOpenDM = vi.fn();
    renderThreadPane({
      agentDetailPanelProps: {
        item: {
          id: "u-manager",
          name: "manager",
          role: "worker",
        },
        t,
        onClose,
        onDelete: vi.fn(),
        onInvite: vi.fn(),
        onOpenDM,
        onRecreate: vi.fn(),
        onStart: vi.fn(),
        onStop: vi.fn(),
      },
    });

    await user.click(
      within(screen.getByRole("dialog", { name: "agentDetailPanel" })).getByRole("button", { name: "openDM" }),
    );

    expect(onClose).toHaveBeenCalledWith(false);
    expect(onOpenDM).not.toHaveBeenCalled();
  });

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
          mentionableUsersByName={new Map()}
          messageActionBusy=""
          messageActionFeedback={{}}
          messageListRef={createRef<HTMLElement>()}
          memberActionBusyID=""
          memberActionError=""
          onApplyMention={() => {}}
          onApplySlashCandidate={() => {}}
          onClearRoomMessages={() => {}}
          onClearMemberActionError={() => {}}
          onCloseThread={() => {}}
          onComposerCompositionEnd={() => {}}
          onComposerCompositionStart={() => {}}
          onComposerKeyDown={() => {}}
          onDeleteRoom={() => {}}
          onInviteAction={() => {}}
          onMessageAction={() => {}}
          onOpenThread={() => {}}
          onRemoveMember={() => {}}
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

  it("opens conversation activity from the working indicator", async () => {
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
          draftSegments={[]}
          draftText=""
          editorRef={createRef<HTMLDivElement>()}
          inviteActionLabel="Invite"
          locale="en"
          logAgent={{ id: "u-manager", name: "manager" }}
          managerProfile={null}
          managerProfileIncomplete={false}
          memberMenuRef={createRef<HTMLDivElement>()}
          mentionCandidates={[]}
          mentionIndex={0}
          mentionableUsersByName={new Map()}
          messageActionBusy=""
          messageActionFeedback={{}}
          messageListRef={createRef<HTMLElement>()}
          onApplyMention={() => {}}
          onClearRoomMessages={() => {}}
          onClearMemberActionError={() => {}}
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
          onSyncComposer={() => {}}
          onThreadDraftChange={() => {}}
          onToggleChannelTools={setShowChannelTools}
          onToggleMemberList={setShowMemberList}
          onToggleToolCalls={() => {}}
          selectedMessageCount={0}
          showChannelTools={showChannelTools}
          showMemberList={showMemberList}
          showToolCalls={false}
          t={t}
          theme="light"
          threadDraftSegments={[]}
          threadError=""
          threadLoading={false}
          usersById={usersById}
          visibleMessages={[]}
          workingParticipants={[
            { id: "u-atlas", name: "Atlas" },
            { id: "u-bram", name: "Bram" },
          ]}
        />
      );
    }

    const view = render(<Harness />);

    expect(screen.getByRole("status")).toHaveTextContent("Atlas is working");
    expect(screen.getByRole("status")).toHaveTextContent("Bram is working");
    const activityAction = screen.getByRole("button", { name: "View activity" });
    expect(activityAction).toHaveClass("needs-attention");
    expect(activityAction).toHaveClass("btn-tertiary-gray");
    expect(window.localStorage.getItem(CONVERSATION_ACTIVITY_ACTION_SEEN_STORAGE_KEY)).toBeNull();

    await user.click(activityAction);

    expect(activityAction).not.toHaveClass("needs-attention");
    expect(window.localStorage.getItem(CONVERSATION_ACTIVITY_ACTION_SEEN_STORAGE_KEY)).toBe("seen");
    expect(screen.getByRole("heading", { name: "Activity" })).toBeInTheDocument();
    expect(document.querySelector(".conversation-activity-panel")).toBeInTheDocument();

    view.unmount();
    render(<Harness />);
    expect(screen.getByRole("button", { name: "View activity" })).not.toHaveClass("needs-attention");
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
        { text: "5月11日", title: null, tooltip: "2026-05-11 10:25:00" },
        { text: "5月12日", title: null, tooltip: "2026-05-12 09:15:00" },
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
