import { renderHook, act, waitFor } from "@testing-library/react";
import { vi } from "vitest";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { IMConversation, IMData, IMMessage, IMUser, TranslateFn } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";

const subscribeIMEventsMock = vi.fn();
const apiMocks = vi.hoisted(() => ({
  sendMessageRequest: vi.fn(),
}));

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: (handler: (payload: unknown) => void) => subscribeIMEventsMock(handler),
}));

vi.mock("@/api/im", async () => {
  const actual = await vi.importActual<typeof import("@/api/im")>("@/api/im");
  return {
    ...actual,
    sendMessageRequest: apiMocks.sendMessageRequest,
  };
});

const t: TranslateFn = (key) => key;

const users: IMUser[] = [
  { id: "u-admin", name: "admin", role: "admin", avatar: "AD", accent_hex: "#dc2626" },
  {
    id: "u-demo",
    name: "demo",
    role: "worker",
    avatar: "avatar/cartoon-3.png",
    accent_hex: "#4f46e5",
  },
];

const directConversation: IMConversation = {
  id: "room-1",
  is_direct: true,
  members: ["u-admin", "u-demo"],
  messages: [],
  title: "demo",
};

function createScrollableMessageList(scrollHeight: number, clientHeight: number): HTMLElement {
  const element = document.createElement("section");
  let scrollTop = 0;
  Object.defineProperties(element, {
    clientHeight: { configurable: true, get: () => clientHeight },
    scrollHeight: { configurable: true, get: () => scrollHeight },
    scrollTop: {
      configurable: true,
      get: () => scrollTop,
      set: (value: number) => {
        scrollTop = value;
      },
    },
  });
  return element;
}

function dataWithMessages(messages: IMMessage[]): IMData {
  return {
    current_user_id: "u-admin",
    rooms: [{ ...directConversation, messages }],
    users,
  };
}

function renderConversationController(
  options: {
    agents?: AgentLike[];
    data?: IMData;
    messageListActive?: boolean;
  } = {},
) {
  const agents: AgentLike[] = options.agents ?? [
    {
      id: "u-demo",
      name: "demo",
      role: "worker",
      avatar: "GI",
      runtime_kind: "picoclaw_sandbox",
      status: "running",
    },
  ];
  const defaultData = dataWithMessages([]);

  return renderHook(
    ({ data = defaultData, messageListActive = true }) =>
      useConversationController({
        activeConversationId: directConversation.id,
        activePane: { type: WorkspacePaneTypes.conversation, id: directConversation.id },
        agents,
        authBusyProvider: "",
        authStatuses: {},
        data,
        locale: "en",
        managerProfile: null,
        managerProfileIncomplete: false,
        messageActionBusy: "",
        messageActionError: { key: "", message: "" },
        messageListActive,
        navigatePane: vi.fn(),
        onMessageAction: vi.fn(),
        onProviderLogin: vi.fn(),
        rooms: data.rooms,
        selectComputer: vi.fn(),
        selectConversation: vi.fn(),
        setActiveConversationId: vi.fn(),
        setBootstrapData: vi.fn(),
        setShowToolCalls: vi.fn(),
        showToolCalls: false,
        t,
        theme: "light",
      }),
    { initialProps: { data: options.data, messageListActive: options.messageListActive } },
  );
}

describe("useConversationController", () => {
  beforeEach(() => {
    subscribeIMEventsMock.mockReset();
    subscribeIMEventsMock.mockReturnValue(() => {});
    apiMocks.sendMessageRequest.mockReset();
  });

  it("opens create-room modal from a direct message", () => {
    const { result } = renderConversationController();

    expect(result.current.conversationViewProps.inviteActionLabel).toBe("memberManagement");

    act(() => {
      result.current.conversationViewProps.onInviteAction();
    });

    expect(result.current.createRoomModalProps).toMatchObject({
      roomMemberIDs: ["u-admin", "u-demo"],
      lockedRoomMemberIDs: ["u-admin", "u-demo"],
    });
    expect(result.current.inviteMembersModalProps).toBeNull();
  });

  it("shows a working participant after sending a direct message until the agent replies", async () => {
    apiMocks.sendMessageRequest.mockResolvedValue({
      id: "msg-user",
      content: "hi",
      created_at: "2026-06-16T10:00:00Z",
      sender_id: "u-admin",
    });
    const { result } = renderConversationController();
    const editor = document.createElement("div");
    editor.textContent = "hi";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });

    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([{ id: "u-demo", name: "demo" }]);

    act(() => {
      result.current.handleRealtimeEvent({
        type: "message.created",
        room_id: "room-1",
        message: {
          id: "msg-tool",
          content: '📄 Web Fetch: from https://example.com {"url":"https://example.com"}',
          created_at: "2026-06-16T10:00:10Z",
          sender_id: "u-demo",
        },
      });
    });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([{ id: "u-demo", name: "demo" }]);

    act(() => {
      result.current.handleRealtimeEvent({
        type: "message.created",
        room_id: "room-1",
        message: {
          id: "msg-reply",
          content: "hello",
          created_at: "2026-06-16T10:00:20Z",
          sender_id: "u-demo",
        },
      });
    });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([]);
  });

  it("derives working participants from recent pending direct-message history after refresh", () => {
    const { result } = renderConversationController({
      data: dataWithMessages([
        {
          id: "msg-user",
          content: "hi",
          created_at: new Date().toISOString(),
          sender_id: "u-admin",
        },
        {
          id: "msg-tool",
          content: '📄 Web Fetch: from https://example.com {"url":"https://example.com"}',
          created_at: new Date().toISOString(),
          sender_id: "u-demo",
        },
      ]),
    });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([{ id: "u-demo", name: "demo" }]);
  });

  it("does not derive working participants after the agent has replied", () => {
    const { result } = renderConversationController({
      data: dataWithMessages([
        {
          id: "msg-user",
          content: "hi",
          created_at: new Date().toISOString(),
          sender_id: "u-admin",
        },
        {
          id: "msg-agent",
          content: "hello",
          created_at: new Date().toISOString(),
          sender_id: "u-demo",
        },
      ]),
    });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([]);
  });

  it("scrolls the message list to the bottom when it becomes active", () => {
    const { result, rerender } = renderConversationController({ messageListActive: false });
    const messageList = createScrollableMessageList(900, 240);
    result.current.conversationViewProps.messageListRef.current = messageList;

    expect(messageList.scrollTop).toBe(0);

    rerender({ messageListActive: true });

    expect(messageList.scrollTop).toBe(900);
  });

  it("keeps an active message list pinned when new visible messages arrive", async () => {
    const firstMessage: IMMessage = {
      id: "msg-1",
      content: "hello",
      created_at: "2026-06-16T10:00:00Z",
      sender_id: "u-admin",
    };
    const secondMessage: IMMessage = {
      id: "msg-2",
      content: "reply",
      created_at: "2026-06-16T10:01:00Z",
      sender_id: "u-demo",
    };
    const initialData = dataWithMessages([firstMessage]);
    const { result, rerender } = renderConversationController({
      data: initialData,
      messageListActive: false,
    });
    const messageList = createScrollableMessageList(900, 240);
    result.current.conversationViewProps.messageListRef.current = messageList;
    rerender({ data: initialData, messageListActive: true });

    Object.defineProperty(messageList, "scrollHeight", {
      configurable: true,
      get: () => 1120,
    });
    rerender({
      data: dataWithMessages([firstMessage, secondMessage]),
      messageListActive: true,
    });

    await waitFor(() => expect(messageList.scrollTop).toBe(1120));
  });

  it("does not open its own IM event subscription", () => {
    renderConversationController();

    expect(subscribeIMEventsMock).not.toHaveBeenCalled();
  });
});
