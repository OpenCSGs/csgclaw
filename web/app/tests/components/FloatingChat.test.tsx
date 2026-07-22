import { createRef } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ConversationPaneProps } from "@/components/business/ConversationPane";
import { emptyGitLabConnectorStatus } from "@/models/connectors";
import { FloatingChat } from "@/pages/WorkspacePage/components/FloatingChat";
import type { IMConversation, IMUser, TranslateFn } from "@/models/conversations";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";

const GUIDE_STORAGE_KEY = "csgclaw:floating-chat:manager-guide:v1";

const labels: Record<string, string> = {
  cancel: "Cancel",
  channelTools: "Channel tools",
  clearRoomMessages: "Clear messages",
  clearRoomMessagesAgentScopeHint: "Clear visible chat messages.",
  clearRoomMessagesConfirm: "Confirm clear",
  close: "Close",
  composerAdd: "Add",
  composerAddContent: "Add content",
  composerConnectors: "Connectors",
  composerFiles: "Files",
  composerTip: "Enter to send · Shift + Enter for a new line",
  connectorConnect: "Connect",
  connectorConnected: "Connected",
  connectorDisconnect: "Disconnect",
  connectorGitHub: "GitHub",
  connectorGitLab: "GitLab",
  connectorManage: "Manage",
  connectorNotConnected: "Not connected",
  deleteRoom: "Delete room",
  deleteRoomConfirm: "Confirm delete",
  deleteRoomConfirmBody: "Delete this room.",
  directMessagesSection: "Direct messages",
  enabled: "Enabled",
  floatingChatCollapse: "Collapse floating chat",
  floatingChatGreeting: "Hi, I'm {name}",
  floatingChatGuideDismiss: "Do not show again",
  floatingChatGuideTitle: "Manager moved here",
  floatingChatOpen: "Open floating chat",
  floatingChatInputPlaceholder: "Type a message. Use / for commands or skills",
  floatingChatSuggestionAskQuestion: "What can you do? How do I use CSGClaw?",
  floatingChatSuggestionCreateWorker: "Create a copywriting worker for me",
  floatingChatSuggestionManageWorkspace: "Put all members into one room",
  floatingChatTryAsking: "Try asking",
  inputPlaceholder: "Message",
  membersTitle: "Members",
  profilePreview: "Profile",
  send: "Send",
  toggleToolCallsHide: "Hide tool calls",
  toggleToolCallsShow: "Show tool calls",
};

const users: IMUser[] = [
  {
    avatar: "AD",
    id: "admin",
    name: "admin",
    role: "admin",
  },
  {
    avatar: "MG",
    id: "manager",
    name: "manager",
    role: "manager",
  },
];

const usersById = new Map(users.map((user) => [user.id, user]));

const t: TranslateFn = (key, params) => {
  let value = labels[key] ?? key;
  for (const [name, replacement] of Object.entries(params ?? {})) {
    value = value.replace(`{${name}}`, String(replacement));
  }
  return value;
};

function renderFloatingChat(props: { open?: boolean; onOpenChange?: (open: boolean) => void } = {}) {
  const onOpenChange = props.onOpenChange ?? vi.fn();
  render(
    <FloatingChat
      avatarFallback="M"
      chatProps={null}
      locale="en"
      open={props.open ?? false}
      t={t}
      title="Manager"
      onOpenChange={onOpenChange}
    />,
  );
  return { onOpenChange };
}

function managerChatProps(
  conversation: IMConversation,
  overrides: Partial<ConversationPaneProps> = {},
): ConversationPaneProps {
  return {
    activeThreadRootID: "",
    activeThreadView: null,
    agents: [],
    authBusyProvider: "",
    authStatuses: {},
    channelToolsRef: createRef<HTMLDivElement>(),
    composerError: "",
    conversation,
    conversationMembers: users,
    currentUserID: "admin",
    draftSegments: [],
    draftText: "",
    editorRef: createRef<HTMLDivElement>(),
    inviteActionLabel: "Invite",
    locale: "en",
    logAgent: null,
    managerProfile: null,
    managerProfileIncomplete: false,
    memberMenuRef: createRef<HTMLDivElement>(),
    mentionCandidates: [],
    mentionIndex: 0,
    mentionableUsersByName: new Map(),
    messageActionBusy: "",
    messageActionFeedback: {},
    messageListRef: createRef<HTMLElement>(),
    onApplyMention: () => {},
    onApplySlashCandidate: () => {},
    onApplyThreadSlashCandidate: () => {},
    onClearRoomMessages: () => {},
    onCloseThread: () => {},
    onComposerCompositionEnd: () => {},
    onComposerCompositionStart: () => {},
    onComposerKeyDown: () => {},
    onDeleteRoom: () => {},
    onDismissThreadSlashPicker: () => {},
    onInviteAction: () => {},
    onMessageAction: () => {},
    onOpenThread: () => {},
    onPreviewUser: () => {},
    onProviderLogin: () => {},
    onSendMessage: () => {},
    onSendThreadReply: () => {},
    onSetThreadSlashIndex: () => {},
    onSyncComposer: () => {},
    onThreadDraftChange: () => {},
    onToggleChannelTools: () => {},
    onToggleMemberList: () => {},
    onToggleToolCalls: () => {},
    selectedMessageCount: conversation.messages.length,
    showChannelTools: false,
    showMemberList: false,
    showToolCalls: false,
    slashCandidates: [],
    slashIndex: 0,
    slashPickerLoading: false,
    slashPickerOpen: false,
    t,
    theme: "light",
    threadDraftSegments: [],
    threadError: "",
    threadLoading: false,
    threadSlashCandidates: [],
    threadSlashIndex: 0,
    threadSlashPickerLoading: false,
    threadSlashPickerOpen: false,
    usersById,
    visibleMessages: conversation.messages,
    ...overrides,
  };
}

function renderOpenManagerFloatingChat(conversation: IMConversation) {
  render(
    <FloatingChat
      avatarFallback="M"
      chatProps={managerChatProps(conversation)}
      locale="en"
      open={true}
      t={t}
      title="Manager"
      onOpenChange={vi.fn()}
    />,
  );
}

describe("FloatingChat manager guide", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("shows a first-use manager guide and stores confirmation when opening it", async () => {
    const user = userEvent.setup();
    const { onOpenChange } = renderFloatingChat();

    expect(screen.getByText("Manager moved here")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Manager moved here" }));

    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(window.localStorage.getItem(GUIDE_STORAGE_KEY)).toBe("seen");
  });

  it("does not show the guide after the user has acknowledged it", () => {
    window.localStorage.setItem(GUIDE_STORAGE_KEY, "seen");

    renderFloatingChat();

    expect(screen.queryByText("Manager moved here")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open floating chat" })).toBeInTheDocument();
  });

  it("treats clicking the floating launcher as using the manager entry", async () => {
    const user = userEvent.setup();
    const { onOpenChange } = renderFloatingChat();

    await user.click(screen.getByRole("button", { name: "Open floating chat" }));

    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(window.localStorage.getItem(GUIDE_STORAGE_KEY)).toBe("seen");
  });
});

describe("FloatingChat manager prompts", () => {
  it("shows the connected GitLab account from the shared conversation state", async () => {
    const user = userEvent.setup();
    const conversation: IMConversation = {
      id: "room-manager",
      is_direct: true,
      members: users.map((item) => item.id),
      messages: [],
      title: "Manager",
    };
    render(
      <FloatingChat
        avatarFallback="M"
        chatProps={managerChatProps(conversation, {
          gitlabConnectorStatus: {
            ...emptyGitLabConnectorStatus(),
            account: {
              avatar_url: "",
              email: "",
              html_url: "",
              id: 1,
              login: "hjwang",
              name: "",
            },
            configured: true,
            connected: true,
          },
        })}
        locale="en"
        open={true}
        t={t}
        title="Manager"
        onOpenChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Add content" }));

    expect(screen.getByText("hjwang")).toBeInTheDocument();
    expect(screen.getByText("Connected")).toBeInTheDocument();
  });

  it("uses answer mode and preserves selected options while navigating", async () => {
    const user = userEvent.setup();
    renderOpenManagerFloatingChat({
      id: "room-manager",
      is_direct: true,
      members: users.map((user) => user.id),
      messages: [
        {
          content: JSON.stringify({
            type: CSGCLAW_AGENT_ACTIVITY_TYPE,
            content: {
              msgtype: AgentActivityMsgTypes.question,
              body: "Question pending",
              question: {
                id: "request-1",
                status: "pending",
                auto_resolve_at: new Date(Date.now() + 120_000).toISOString(),
                questions: [
                  {
                    id: "scope",
                    header: "Scope",
                    question: "Choose a scope",
                    options: [{ label: "Workspace" }],
                  },
                  {
                    id: "detail",
                    header: "Detail",
                    question: "Add more detail",
                    options: [],
                  },
                ],
              },
            },
            channel: "csgclaw",
            event_id: "question-request-1",
            origin_server_ts: 1,
            room_id: "room-manager",
            sender: "manager",
            version: 1,
          }),
          created_at: "2026-07-15T08:00:00Z",
          id: "question-request-1",
          sender_id: "manager",
        },
      ],
      title: "manager",
    });

    expect(screen.getAllByText("Choose a scope").length).toBeGreaterThan(0);
    expect(screen.getByRole("timer")).toBeInTheDocument();
    expect(screen.queryByLabelText("questionFreeformAnswer")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "questionPrevious" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "questionNext" })).toBeEnabled();
    expect(screen.queryByLabelText("Type a message. Use / for commands or skills")).not.toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: /Workspace/ }));
    await user.click(screen.getByRole("button", { name: "questionPrevious" }));
    const selectedOption = screen.getByRole("radio", { name: /Workspace/ });
    expect(selectedOption).toHaveAttribute("aria-checked", "true");
    await user.click(selectedOption);
    expect(screen.getByRole("heading", { name: "Add more detail" })).toBeInTheDocument();
  });

  it("shows prompt suggestions when the manager chat only has the bootstrap notice", () => {
    renderOpenManagerFloatingChat({
      id: "room-manager",
      is_direct: true,
      members: users.map((user) => user.id),
      messages: [
        {
          content: "Bootstrap room created for admin and manager.",
          created_at: "2026-06-17T10:41:00+08:00",
          id: "msg-bootstrap",
          sender_id: "manager",
        },
      ],
      title: "manager",
    });

    expect(screen.queryByText("Bootstrap room created for admin and manager.")).not.toBeInTheDocument();
    expect(screen.getByText("Hi, I'm Manager")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create a copywriting worker for me" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Assign this task to the right worker and track progress" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Put all members into one room" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "What can you do? How do I use CSGClaw?" })).toBeInTheDocument();
    expect(screen.getByLabelText("Type a message. Use / for commands or skills")).toBeInTheDocument();
  });

  it("writes a picked prompt into the floating chat composer", async () => {
    const user = userEvent.setup();
    renderOpenManagerFloatingChat({
      id: "room-manager",
      is_direct: true,
      members: users.map((item) => item.id),
      messages: [],
      title: "manager",
    });

    await user.click(screen.getByRole("button", { name: "Create a copywriting worker for me" }));

    expect(screen.getByLabelText("Type a message. Use / for commands or skills")).toHaveTextContent(
      "Create a copywriting worker for me",
    );
  });
});
