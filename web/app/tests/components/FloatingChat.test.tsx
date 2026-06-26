import { createRef } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ConversationPaneProps } from "@/components/business/ConversationPane";
import { FloatingChat } from "@/pages/WorkspacePage/components/FloatingChat";
import type { IMConversation, IMUser, TranslateFn } from "@/models/conversations";

const GUIDE_STORAGE_KEY = "csgclaw:floating-chat:manager-guide:v1";

const labels: Record<string, string> = {
  cancel: "Cancel",
  channelTools: "Channel tools",
  clearRoomMessages: "Clear messages",
  clearRoomMessagesAgentScopeHint: "Clear visible chat messages.",
  clearRoomMessagesConfirm: "Confirm clear",
  close: "Close",
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

function managerChatProps(conversation: IMConversation): ConversationPaneProps {
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
    messageActionError: {},
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
