import { renderHook, act, waitFor } from "@testing-library/react";
import { vi } from "vitest";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { IMConversation, IMData, IMMessage, IMUser, TranslateFn } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";

const subscribeIMEventsMock = vi.fn();
const apiMocks = vi.hoisted(() => ({
  fetchThreadRequest: vi.fn(),
  sendMessageRequest: vi.fn(),
}));

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: (handler: (payload: unknown) => void) => subscribeIMEventsMock(handler),
}));

vi.mock("@/api/im", async () => {
  const actual = await vi.importActual<typeof import("@/api/im")>("@/api/im");
  return {
    ...actual,
    fetchThreadRequest: apiMocks.fetchThreadRequest,
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

type ScrollableMessageList = HTMLElement & {
  setScrollHeight: (value: number) => void;
};

function nextAnimationFrame(): Promise<void> {
  return new Promise((resolve) => {
    if (typeof window.requestAnimationFrame === "function") {
      window.requestAnimationFrame(() => resolve());
      return;
    }
    window.setTimeout(resolve, 0);
  });
}

function createScrollableMessageList(scrollHeight: number, clientHeight: number): ScrollableMessageList {
  const element = document.createElement("section");
  let scrollHeightValue = scrollHeight;
  let scrollTop = 0;
  Object.defineProperties(element, {
    clientHeight: { configurable: true, get: () => clientHeight },
    scrollHeight: { configurable: true, get: () => scrollHeightValue },
    scrollTop: {
      configurable: true,
      get: () => scrollTop,
      set: (value: number) => {
        scrollTop = value;
      },
    },
  });
  return Object.assign(element, {
    setScrollHeight(value: number) {
      scrollHeightValue = value;
    },
  });
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
    apiMocks.fetchThreadRequest.mockReset();
    apiMocks.sendMessageRequest.mockReset();
  });

  it("keeps an unopened thread local until the first reply is sent", async () => {
    const root: IMMessage = {
      id: "msg-root",
      content: "Start here",
      created_at: "2026-07-14T09:29:00Z",
      sender_id: "u-admin",
    };
    const { result } = renderConversationController({ data: dataWithMessages([root]) });

    await act(async () => {
      await result.current.conversationViewProps.onOpenThread(root);
    });

    expect(apiMocks.fetchThreadRequest).not.toHaveBeenCalled();
    expect(result.current.activeThreadRootID).toBe("msg-root");
    expect(result.current.conversationViewProps.activeThreadView).toMatchObject({
      room_id: "room-1",
      root,
      replies: [],
      summary: null,
    });

    act(() => {
      result.current.conversationViewProps.onCloseThread();
    });

    expect(result.current.activeThreadRootID).toBe("");
    expect(result.current.conversationViewProps.activeThreadView).toBeNull();
  });

  it("loads a persisted thread when the root already has replies", async () => {
    const root: IMMessage = {
      id: "msg-root",
      content: "Start here",
      created_at: "2026-07-14T09:29:00Z",
      sender_id: "u-admin",
      thread: {
        reply_count: 1,
        root_id: "msg-root",
      },
    };
    apiMocks.fetchThreadRequest.mockResolvedValue({
      room_id: "room-1",
      root,
      replies: [{ id: "msg-reply", content: "Reply", sender_id: "u-demo" }],
      summary: root.thread,
    });
    const { result } = renderConversationController({ data: dataWithMessages([root]) });

    await act(async () => {
      await result.current.conversationViewProps.onOpenThread(root);
    });

    expect(apiMocks.fetchThreadRequest).toHaveBeenCalledWith("room-1", "msg-root");
    expect(result.current.conversationViewProps.activeThreadView?.replies).toHaveLength(1);
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
          metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
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

  it("keeps a sent direct message working participant past the local timeout until the agent replies", async () => {
    vi.useFakeTimers();
    try {
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

      act(() => {
        vi.advanceTimersByTime(121_000);
      });

      expect(result.current.conversationViewProps.workingParticipants).toEqual([{ id: "u-demo", name: "demo" }]);
    } finally {
      vi.useRealTimers();
    }
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
          metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
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

  it("keeps an active message list pinned when an existing visible message changes", async () => {
    const firstMessage: IMMessage = {
      id: "msg-1",
      content: "loading",
      created_at: "2026-06-16T10:00:00Z",
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

    messageList.setScrollHeight(1120);
    rerender({
      data: dataWithMessages([{ ...firstMessage, content: "final answer with more rendered content" }]),
      messageListActive: true,
    });

    await waitFor(() => expect(messageList.scrollTop).toBe(1120));
  });

  it("does not follow incoming messages when the user has scrolled away from the bottom", async () => {
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
    await nextAnimationFrame();

    act(() => {
      messageList.scrollTop = 120;
      messageList.dispatchEvent(new Event("scroll"));
    });
    await act(async () => {
      await nextAnimationFrame();
    });

    messageList.setScrollHeight(1120);
    rerender({
      data: dataWithMessages([firstMessage, secondMessage]),
      messageListActive: true,
    });
    await act(async () => {
      await nextAnimationFrame();
    });

    expect(messageList.scrollTop).toBe(120);
  });

  it("follows an outgoing message even when the user has scrolled away from the bottom", async () => {
    const firstMessage: IMMessage = {
      id: "msg-1",
      content: "hello",
      created_at: "2026-06-16T10:00:00Z",
      sender_id: "u-demo",
    };
    const initialData = dataWithMessages([firstMessage]);
    apiMocks.sendMessageRequest.mockResolvedValue({
      id: "msg-user",
      content: "my reply",
      created_at: "2026-06-16T10:01:00Z",
      sender_id: "u-admin",
    });
    const { result, rerender } = renderConversationController({
      data: initialData,
      messageListActive: false,
    });
    const messageList = createScrollableMessageList(900, 240);
    result.current.conversationViewProps.messageListRef.current = messageList;
    rerender({ data: initialData, messageListActive: true });
    await nextAnimationFrame();

    act(() => {
      messageList.scrollTop = 120;
      messageList.dispatchEvent(new Event("scroll"));
    });
    await act(async () => {
      await nextAnimationFrame();
    });
    messageList.setScrollHeight(1120);

    const editor = document.createElement("div");
    editor.textContent = "my reply";
    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    await waitFor(() => expect(messageList.scrollTop).toBe(1120));
  });

  it("keeps a single message-list scroll listener across data rerenders", () => {
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
    const addEventListenerSpy = vi.spyOn(messageList, "addEventListener");
    result.current.conversationViewProps.messageListRef.current = messageList;

    rerender({ data: initialData, messageListActive: true });
    rerender({ data: dataWithMessages([firstMessage, secondMessage]), messageListActive: true });

    expect(addEventListenerSpy.mock.calls.filter(([event]) => event === "scroll")).toHaveLength(1);
  });

  it("does not open its own IM event subscription", () => {
    renderConversationController();

    expect(subscribeIMEventsMock).not.toHaveBeenCalled();
  });
});
