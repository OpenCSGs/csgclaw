import { renderHook, act, waitFor } from "@testing-library/react";
import { vi } from "vitest";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { IMConversation, IMData, IMMessage, IMUser, TranslateFn } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";

const subscribeIMEventsMock = vi.fn();

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: (handler: (payload: unknown) => void) => subscribeIMEventsMock(handler),
}));

const t: TranslateFn = (key) => key;

const users: IMUser[] = [
  { id: "u-admin", name: "admin", handle: "admin", role: "admin", avatar: "AD", accent_hex: "#dc2626" },
  {
    id: "u-demo",
    name: "demo",
    handle: "demo",
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
    onRefreshAgentState?: (agentID: string) => Promise<AgentLike | null>;
  } = {},
) {
  const agents: AgentLike[] = options.agents ?? [
    {
      id: "u-demo",
      name: "demo",
      handle: "demo",
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
        onRefreshAgentState: options.onRefreshAgentState ?? vi.fn(),
        onUpgradeStatusChange: vi.fn(),
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

  it("refreshes a non-running agent when that agent sends a message", () => {
    let eventHandler: ((payload: unknown) => void) | null = null;
    subscribeIMEventsMock.mockImplementation((handler: (payload: unknown) => void) => {
      eventHandler = handler;
      return () => {};
    });
    const onRefreshAgentState = vi.fn(async () => null);
    renderConversationController({
      agents: [
        {
          id: "u-demo",
          name: "demo",
          handle: "demo",
          role: "worker",
          runtime_kind: "picoclaw_sandbox",
          status: "unknown",
        },
      ],
      onRefreshAgentState,
    });

    act(() => {
      eventHandler?.({
        message: {
          content: "reply",
          sender_id: "u-demo",
        },
        room_id: directConversation.id,
        type: "message.created",
      });
    });

    expect(onRefreshAgentState).toHaveBeenCalledWith("u-demo");
  });
});
