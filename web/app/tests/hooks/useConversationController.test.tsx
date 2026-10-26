import { createQueryWrapper } from "../helpers/queryClient";
import { renderHook, act, waitFor } from "@testing-library/react";
import { vi } from "vitest";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import { WorkspacePaneTypes } from "@/models/routing";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";
import { MAX_ATTACHMENT_FILE_BYTES } from "@/models/attachments";
import { renderComposerSegments, type ComposerSegment } from "@/models/composer";
import { resourceSegment } from "@/models/conversationResources";
import {
  applyIMEvent,
  RoomTypes,
  type IMConversation,
  type IMData,
  type IMMessage,
  type IMUser,
  type ThreadView,
  type TranslateFn,
} from "@/models/conversations";
import type { AgentLike } from "@/models/agents";
import type { AgentProfileLike } from "@/models/agents";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane";
import type { SendMessageRequestOptions } from "@/api/im";
import type { OpenCSGAuthGuard } from "@/hooks/workspace/useOpenCSGAuthGuard";

const subscribeIMEventsMock = vi.fn();
const apiMocks = vi.hoisted(() => ({
  deleteRoomRequest: vi.fn(),
  fetchThreadRequest: vi.fn(),
  fetchMessagesRequest: vi.fn(),
  inviteRoomUsersRequest: vi.fn(),
  removeRoomUserRequest: vi.fn(),
  sendMessageRequest: vi.fn(),
}));

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: (handler: (payload: unknown) => void) => subscribeIMEventsMock(handler),
}));

vi.mock("@/api/im", async () => {
  const actual = await vi.importActual<typeof import("@/api/im")>("@/api/im");
  return {
    ...actual,
    deleteRoomRequest: apiMocks.deleteRoomRequest,
    fetchThreadRequest: apiMocks.fetchThreadRequest,
    fetchMessagesRequest: apiMocks.fetchMessagesRequest,
    inviteRoomUsersRequest: apiMocks.inviteRoomUsersRequest,
    removeRoomUserRequest: apiMocks.removeRoomUserRequest,
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

function groupConversationData(conversation: IMConversation): IMData {
  return {
    current_user_id: "u-admin",
    rooms: [conversation],
    users: [
      users[0],
      { id: "u-manager", name: "manager", role: "manager", avatar: "MG", accent_hex: "#2563eb" },
      { id: "u-dev", name: "dev", role: "worker", avatar: "DV", accent_hex: "#16a34a" },
      { id: "u-qa", name: "qa", role: "worker", avatar: "QA", accent_hex: "#9333ea" },
    ],
  };
}

function successfulMessage(content = "hello"): IMMessage {
  return {
    id: "msg-user",
    content,
    created_at: "2026-09-16T10:00:00Z",
    sender_id: "u-admin",
  };
}

function composerEditorWithMention(userID: string, userName: string, text: string): HTMLDivElement {
  const editor = document.createElement("div");
  const mention = document.createElement("span");
  mention.dataset.userId = userID;
  mention.dataset.userName = userName;
  mention.textContent = `@${userName}`;
  editor.append(mention, document.createTextNode(text));
  return editor;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function renderConversationController(
  options: {
    activeConversationId?: string;
    routedConversationId?: string;
    agents?: AgentLike[];
    data?: IMData;
    managerProfile?: AgentProfileLike | null;
    managerProfileIncomplete?: boolean;
    authStatuses?: Parameters<typeof useConversationController>[0]["authStatuses"];
    locale?: "en" | "zh";
    t?: TranslateFn;
    managerRuntimeUnavailable?: boolean;
    managerRuntimeWarning?: string;
    messageListActive?: boolean;
    openCSGAuthGuard?: OpenCSGAuthGuard;
    setBootstrapData?: (updater: unknown) => void;
    workingParticipantsForRoom?: (roomID: string | null | undefined) => ConversationWorkingParticipant[];
  } = {},
) {
  const agents: AgentLike[] = options.agents ?? [
    {
      id: "u-demo",
      name: "demo",
      role: "worker",
      avatar: "GI",
      runtime_kind: "codex",
      status: "running",
    },
  ];
  const defaultData = dataWithMessages([]);

  return renderHook(
    ({ activeConversationId = directConversation.id, data = defaultData, messageListActive = true }) =>
      useConversationController({
        activeConversationId,
        activePane: { type: WorkspacePaneTypes.conversation, id: options.routedConversationId ?? activeConversationId },
        agents,
        authBusyProvider: "",
        authStatuses: options.authStatuses ?? {},
        data,
        locale: options.locale ?? "en",
        managerProfile: options.managerProfile ?? null,
        managerProfileIncomplete: options.managerProfileIncomplete ?? false,
        managerRuntimeUnavailable: options.managerRuntimeUnavailable,
        managerRuntimeWarning: options.managerRuntimeWarning,
        hasObservedWorkLease: () => false,
        messageActionBusy: "",
        messageActionFeedback: { key: "", message: "" },
        messageListActive,
        navigatePane: vi.fn(),
        onMessageAction: vi.fn(),
        onProviderLogin: vi.fn(),
        openCSGAuthGuard: options.openCSGAuthGuard ?? openCSGAuthGuardStub(),
        rooms: data.rooms,
        selectComputer: vi.fn(),
        selectConversation: vi.fn(),
        setActiveConversationId: vi.fn(),
        setBootstrapData: options.setBootstrapData ?? vi.fn(),
        setShowToolCalls: vi.fn(),
        showToolCalls: false,
        t: options.t ?? t,
        theme: "light",
        workingParticipantsForRoom: options.workingParticipantsForRoom ?? (() => []),
      }),
    {
      wrapper: createQueryWrapper().wrapper,
      initialProps: {
        activeConversationId: options.activeConversationId,
        data: options.data,
        messageListActive: options.messageListActive,
      },
    },
  );
}

describe("useConversationController", () => {
  it.each([
    ["zh", "网络连接失败，请检查网络后重试。"],
    ["en", "The network request failed. Check your connection and try again."],
  ] as const)("localizes fetch failures in %s", async (locale, expectedMessage) => {
    const { result } = renderConversationController({
      data: dataWithMessages([]),
      locale,
      t: (key) => (key === "sendNetworkError" ? expectedMessage : key),
    });
    apiMocks.sendMessageRequest.mockRejectedValueOnce(new TypeError("Failed to fetch"));
    const editor = document.createElement("div");
    editor.textContent = "hello";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(result.current.conversationViewProps.composerError).toBe(expectedMessage);
  });

  it("does not restore a deleted attachment from a late poll when no thread is open", async () => {
    const original: IMMessage = {
      id: "with-file",
      sender_id: "u-admin",
      content: "File",
      attachments: [{ id: "file", name: "file.txt", url: "file.txt", size: 1, content_type: "text/plain" }],
    };
    let data = dataWithMessages([original]);
    const snapshot = deferred<IMMessage[]>();
    apiMocks.fetchMessagesRequest.mockReturnValueOnce(snapshot.promise);
    const { result } = renderConversationController({
      data,
      setBootstrapData: (updater) => {
        data = typeof updater === "function" ? (updater as (current: IMData) => IMData)(data) : data;
      },
    });
    expect(apiMocks.fetchMessagesRequest).toHaveBeenCalledWith(directConversation.id);
    const event = { type: "room.attachment_deleted", room_id: directConversation.id, attachment_id: "file" };
    act(() => {
      data = applyIMEvent(data, event);
      result.current.handleRealtimeEvent(event);
    });
    await act(async () => {
      snapshot.resolve([original]);
    });
    expect(data.rooms[0].messages[0].attachments).toEqual([]);
  });

  it.each(["thread.created", "thread.updated"])(
    "invalidates an in-flight snapshot on %s without an open thread",
    async (type) => {
      const root = successfulMessage("Question");
      let data = dataWithMessages([root]);
      const snapshot = deferred<IMMessage[]>();
      apiMocks.fetchMessagesRequest.mockReturnValueOnce(snapshot.promise);
      const { result } = renderConversationController({
        data,
        setBootstrapData: (updater) => {
          data = typeof updater === "function" ? (updater as (current: IMData) => IMData)(data) : data;
        },
      });
      const event = {
        type,
        thread: { room_id: directConversation.id, root, summary: { root_id: root.id, reply_count: 1 } },
      };
      act(() => {
        data = applyIMEvent(data, event);
        result.current.handleRealtimeEvent(event);
      });
      await act(async () => {
        snapshot.resolve([root, { ...root, id: "stale" }]);
      });
      expect(data.rooms[0].messages.map((message) => message.id)).toEqual([root.id]);
      expect(data.rooms[0].messages[0].thread?.reply_count).toBe(1);
    },
  );

  it("invalidates snapshots for every room when a user is deleted", async () => {
    const root = successfulMessage("Question");
    const removed = { id: "removed", sender_id: "u-demo", content: "Reply" };
    let data = dataWithMessages([root, removed]);
    data.rooms[0].members = ["u-admin", "u-demo", "u-other"];
    const snapshot = deferred<IMMessage[]>();
    apiMocks.fetchMessagesRequest.mockReturnValueOnce(snapshot.promise);
    const { result } = renderConversationController({
      data,
      setBootstrapData: (updater) => {
        data = typeof updater === "function" ? (updater as (current: IMData) => IMData)(data) : data;
      },
    });
    const event = { type: "user.deleted", user: { id: "u-demo" } };
    act(() => {
      data = applyIMEvent(data, event);
      result.current.handleRealtimeEvent(event);
    });
    await act(async () => {
      snapshot.resolve([root, removed]);
    });
    expect(data.rooms).toHaveLength(1);
    expect(data.rooms[0].messages.map((message) => message.id)).toEqual([root.id]);
  });

  beforeEach(() => {
    localStorage.clear();
    subscribeIMEventsMock.mockReset();
    subscribeIMEventsMock.mockReturnValue(() => {});
    apiMocks.deleteRoomRequest.mockReset();
    apiMocks.fetchThreadRequest.mockReset();
    apiMocks.fetchMessagesRequest.mockReset().mockRejectedValue(new Error("No snapshot configured"));
    apiMocks.inviteRoomUsersRequest.mockReset();
    apiMocks.removeRoomUserRequest.mockReset();
    apiMocks.sendMessageRequest.mockReset();
  });

  it.each(["composer", "thread"])("%s 保留失败选择，重试复用请求身份，成功后清空资源", async (scope) => {
    const root = { id: "root", content: "任务", sender_id: "u-admin" };
    const { result } = renderConversationController({
      data: dataWithMessages([root]),
      agents: [{ id: "u-demo", name: "demo", role: "worker", runtime_kind: "codex", status: "running" }],
    });
    const selected = [{ source: "skill" as const, id: "review", type: "skill" as const, name: "review" }];
    if (scope === "thread") {
      await act(async () => result.current.conversationViewProps.onOpenThread(root));
      act(() => result.current.conversationViewProps.onPickThreadResources(selected));
    } else {
      act(() => result.current.conversationViewProps.onPickResources(selected));
    }
    const send = () =>
      scope === "thread"
        ? result.current.conversationViewProps.onSendThreadReply()
        : result.current.conversationViewProps.onSendMessage();
    const segments = () =>
      scope === "thread"
        ? result.current.conversationViewProps.threadDraftSegments
        : result.current.conversationViewProps.draftSegments;
    apiMocks.sendMessageRequest.mockRejectedValueOnce({ code: "resource_disabled", message: "资源停用" });
    await act(async () => {
      await send();
    });
    expect(segments()).toEqual([expect.objectContaining({ type: "resource", agentID: "u-demo" })]);
    const first = apiMocks.sendMessageRequest.mock.calls[0][0];
    expect(first).toMatchObject({ content: "", resource_refs: [{ source: "skill", id: "review" }] });
    if (scope === "thread") expect(first.relates_to).toEqual({ rel_type: "m.thread", event_id: "root" });
    apiMocks.sendMessageRequest.mockResolvedValueOnce(successfulMessage(""));
    apiMocks.fetchThreadRequest.mockResolvedValue({ root, replies: [] });
    await act(async () => {
      await send();
    });
    expect(apiMocks.sendMessageRequest.mock.calls[1][0].client_message_id).toBe(first.client_message_id);
    expect(segments()).toEqual([]);
    act(() =>
      scope === "thread"
        ? result.current.conversationViewProps.onPickThreadResources([{ ...selected[0], id: "other" }])
        : result.current.conversationViewProps.onPickResources([{ ...selected[0], id: "other" }]),
    );
    apiMocks.sendMessageRequest.mockResolvedValueOnce(successfulMessage(""));
    await act(async () => {
      await send();
    });
    expect(apiMocks.sendMessageRequest.mock.calls[2][0].client_message_id).not.toBe(first.client_message_id);
  });

  it.each(["composer", "thread"])("keeps file and resource identities in %s sends and retries", async (scope) => {
    const root = { id: "root", content: "Review", sender_id: "u-admin" };
    const { result } = renderConversationController({
      data: dataWithMessages([root]),
      agents: [{ id: "u-demo", name: "demo", role: "worker", runtime_kind: "codex", status: "running" }],
    });
    if (scope === "thread") await act(async () => result.current.conversationViewProps.onOpenThread(root));
    const editor = document.createElement("div");
    const setDraft = (path: string, id: string) => {
      const segments: ComposerSegment[] = [
        resourceSegment({ source: "mcp", id, type: "connector", name: "Search" }, "u-demo"),
        {
          type: "file",
          text: "@notes.md",
          reference: { source: "workspace", agent_id: "u-demo", path, label: "notes.md" },
        },
      ];
      act(() => {
        if (scope === "thread") result.current.conversationViewProps.onThreadDraftChange(segments);
        else {
          result.current.conversationViewProps.editorRef.current = editor;
          renderComposerSegments(editor, segments);
          result.current.conversationViewProps.onSyncComposer();
        }
      });
    };
    const send = async () => {
      await act(async () => {
        if (scope === "thread") await result.current.conversationViewProps.onSendThreadReply();
        else await result.current.conversationViewProps.onSendMessage();
      });
      return apiMocks.sendMessageRequest.mock.lastCall![0];
    };
    apiMocks.sendMessageRequest.mockRejectedValue({ message: "Temporary failure" });
    setDraft("docs/notes.md", "search");
    const first = await send();
    expect(first).toMatchObject({
      resource_refs: [{ source: "mcp", id: "search" }],
      file_references: [{ source: "workspace", agent_id: "u-demo", path: "docs/notes.md" }],
    });
    expect((await send()).client_message_id).toBe(first.client_message_id);
    setDraft("other/notes.md", "search");
    const changedFile = await send();
    expect(changedFile.content).toBe(first.content);
    expect(changedFile.client_message_id).not.toBe(first.client_message_id);
    setDraft("other/notes.md", "other-search");
    expect((await send()).client_message_id).not.toBe(changedFile.client_message_id);
  });

  it.each(["composer", "thread"])(
    "continues a worker task in the %s with an incomplete and unavailable Manager",
    async (scope) => {
      const root = { id: "root", content: "first prompt", sender_id: "u-admin" };
      const { result } = renderConversationController({
        managerProfileIncomplete: true,
        managerRuntimeUnavailable: true,
        managerProfile: { provider: "codex", model_id: "", profile_complete: false },
        authStatuses: { codex: { authenticated: false } },
        agents: [
          {
            id: "u-demo",
            role: "worker",
            status: "running",
            model_config: { model_provider_id: "local", model_id: "worker-model", profile_complete: true },
          },
        ],
        data: dataWithMessages([root]),
      });
      apiMocks.sendMessageRequest.mockResolvedValue({ id: "reply", content: "continue", sender_id: "u-admin" });
      if (scope === "thread") {
        await act(async () => {
          await result.current.conversationViewProps.onOpenThread(root);
        });
        act(() => result.current.conversationViewProps.onThreadDraftChange([{ type: "text", text: "continue" }]));
        expect(result.current.conversationViewProps.threadComposerDisabledReason).toBe("");
        await act(async () => {
          await result.current.conversationViewProps.onSendThreadReply();
        });
      } else {
        const editor = document.createElement("div");
        editor.textContent = "continue";
        act(() => {
          result.current.conversationViewProps.editorRef.current = editor;
          result.current.conversationViewProps.onSyncComposer();
        });
        expect(result.current.conversationViewProps.composerDisabledReason).toBe("");
        await act(async () => {
          await result.current.conversationViewProps.onSendMessage();
        });
      }
      expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
      expect(apiMocks.sendMessageRequest.mock.calls[0][0].room_id).toBe("room-1");
    },
  );

  it.each(["opencsg", "codex"])("authenticates the %s room override instead of the Agent default", async (provider) => {
    const guard = { ...openCSGAuthGuardStub(false), requireAuthentication: vi.fn(() => false) };
    const data = dataWithMessages([]);
    data.rooms[0].configuration = { model_provider_id: provider, model_id: "override-model" };
    const { result } = renderConversationController({
      data,
      openCSGAuthGuard: guard,
      authStatuses: { codex: { authenticated: false } },
      agents: [
        {
          id: "u-demo",
          role: "worker",
          status: "running",
          model_config: { model_provider_id: "local", model_id: "default", profile_complete: false },
        },
      ],
    });
    const editor = document.createElement("div");
    editor.textContent = "continue";
    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    expect(result.current.conversationViewProps.composerDisabledReason).toBe("");
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });
    expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();
    if (provider === "opencsg") expect(guard.requireAuthentication).toHaveBeenCalledOnce();
    else expect(result.current.conversationViewProps.composerError).toBe("authRequired");
  });

  it.each([RoomTypes.free, RoomTypes.onDemand])("checks only the selected %s room recipients", async (type) => {
    const data = dataWithMessages([]);
    data.rooms[0] = { ...data.rooms[0], is_direct: false, type, manager_id: "u-demo" };
    const { result } = renderConversationController({
      data,
      managerProfileIncomplete: true,
      agents: [{ id: "u-demo", role: "manager", status: "running", model_config: { profile_complete: false } }],
    });
    const editor = document.createElement("div");
    editor.textContent = "message to people";
    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    apiMocks.sendMessageRequest.mockResolvedValue({ id: "reply", content: "message to people", sender_id: "u-admin" });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });
    if (type === RoomTypes.onDemand) {
      expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();
      expect(result.current.conversationViewProps.composerDisabledReason).toBe("profileIncomplete");
    } else {
      expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
      expect(result.current.conversationViewProps.composerDisabledReason).toBe("");
    }
  });

  it("renders and sends to the routed task while the previous selection is stale", async () => {
    const oldRoom = {
      ...directConversation,
      id: "manager-room",
      title: "Manager",
      messages: [{ id: "old", content: "old manager task", sender_id: "manager" }],
    };
    const nextRoom = {
      ...directConversation,
      id: "worker-task",
      title: "Worker task",
      messages: [{ id: "new", content: "new task", sender_id: "u-admin" }],
    };
    apiMocks.sendMessageRequest.mockResolvedValue({ id: "reply", content: "continue", sender_id: "u-admin" });
    const { result } = renderConversationController({
      activeConversationId: oldRoom.id,
      routedConversationId: nextRoom.id,
      data: { ...dataWithMessages([]), rooms: [oldRoom, nextRoom] },
    });
    expect(result.current.selectedConversation?.id).toBe(nextRoom.id);
    expect(result.current.visibleMessages.map((message) => message.id)).toEqual(["new"]);
    const editor = document.createElement("div");
    editor.textContent = "continue";
    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });
    expect(apiMocks.sendMessageRequest).toHaveBeenCalledWith(
      expect.objectContaining({ room_id: nextRoom.id, content: "continue" }),
      expect.anything(),
    );
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

  it("propagates room deletion failures to the confirmation dialog", async () => {
    apiMocks.deleteRoomRequest.mockRejectedValue({
      status: 409,
      code: "room_has_active_tasks",
      message: "finish or stop active room tasks before deleting the room",
    });
    const { result } = renderConversationController();

    await expect(result.current.conversationViewProps.onDeleteRoom("room-1")).rejects.toMatchObject({
      code: "room_has_active_tasks",
    });
    expect(result.current.conversationViewProps.composerError).toBe("");
  });

  it("keeps the thread root anchored while the thread panel opens and closes", async () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
      window.setTimeout(() => callback(performance.now()), 0),
    );
    vi.stubGlobal("cancelAnimationFrame", (handle: number) => window.clearTimeout(handle));
    try {
      const root: IMMessage = {
        id: "msg-root",
        content: "Start here",
        created_at: "2026-07-14T09:29:00Z",
        sender_id: "u-admin",
      };
      const data = dataWithMessages([root]);
      const { result, rerender } = renderConversationController({ data, messageListActive: false });
      const messageList = createScrollableMessageList(3000, 400);
      let rootDocumentTop = 860;
      const rootRow = document.createElement("div");
      rootRow.className = "message-row";
      rootRow.dataset.messageId = root.id;
      rootRow.getBoundingClientRect = () => new DOMRect(0, rootDocumentTop - messageList.scrollTop, 100, 40);
      messageList.appendChild(rootRow);
      result.current.conversationViewProps.messageListRef.current = messageList;
      rerender({ data, messageListActive: true });
      await act(async () => {
        await nextAnimationFrame();
        await nextAnimationFrame();
      });

      act(() => {
        messageList.scrollTop = 600;
        messageList.dispatchEvent(new Event("scroll"));
      });
      await act(async () => {
        await nextAnimationFrame();
      });

      await act(async () => {
        const opening = result.current.conversationViewProps.onOpenThread(root);
        rootDocumentTop += 1000;
        await opening;
      });
      await act(async () => {
        await nextAnimationFrame();
        await nextAnimationFrame();
      });
      expect(messageList.scrollTop).toBe(1600);

      act(() => {
        result.current.conversationViewProps.onCloseThread();
        rootDocumentTop -= 1000;
      });
      await act(async () => {
        await nextAnimationFrame();
        await nextAnimationFrame();
      });
      expect(messageList.scrollTop).toBe(600);
    } finally {
      vi.unstubAllGlobals();
    }
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

  describe("thread selection changes", () => {
    const firstRoot: IMMessage = {
      id: "root-first",
      content: "First thread",
      sender_id: "u-admin",
      thread: { root_id: "root-first", reply_count: 1 },
    };
    const secondRoot: IMMessage = { id: "root-second", content: "Second thread", sender_id: "u-admin" };
    const firstView: ThreadView = { room_id: "room-1", root: firstRoot, replies: [], summary: firstRoot.thread };
    const secondView: ThreadView = { room_id: "room-1", root: secondRoot, replies: [], summary: null };

    it("keeps replies addressed to the thread shown after an older load finishes", async () => {
      const load = deferred<ThreadView>();
      apiMocks.fetchThreadRequest.mockReturnValueOnce(load.promise).mockResolvedValue(secondView);
      const { result } = renderConversationController({ data: dataWithMessages([firstRoot, secondRoot]) });
      let opening!: Promise<void>;
      act(() => {
        opening = result.current.conversationViewProps.onOpenThread(firstRoot);
      });
      await act(async () => {
        await result.current.conversationViewProps.onOpenThread(secondRoot);
      });
      await act(async () => {
        load.resolve(firstView);
        await opening;
      });

      expect(result.current.conversationViewProps.activeThreadView?.root?.id).toBe(secondRoot.id);
      act(() => {
        result.current.conversationViewProps.onThreadDraftChange([{ type: "text", text: "Reply to this thread" }]);
      });
      apiMocks.sendMessageRequest.mockResolvedValue({
        id: "reply-new",
        content: "Reply to this thread",
        sender_id: "u-admin",
        relates_to: { rel_type: "m.thread", event_id: secondRoot.id },
      });
      await act(async () => {
        await result.current.conversationViewProps.onSendThreadReply();
      });

      expect(apiMocks.sendMessageRequest).toHaveBeenCalledWith(
        expect.objectContaining({ relates_to: { rel_type: "m.thread", event_id: secondRoot.id } }),
      );
    });

    it.each(["close", "switch room"])("discards a delayed thread load after %s", async (action) => {
      const load = deferred<ThreadView>();
      apiMocks.fetchThreadRequest.mockReturnValue(load.promise);
      const data = dataWithMessages([firstRoot]);
      data.rooms.push({ ...directConversation, id: "room-2", messages: [] });
      const { result, rerender } = renderConversationController({ data });
      let opening!: Promise<void>;
      act(() => {
        opening = result.current.conversationViewProps.onOpenThread(firstRoot);
      });
      if (action === "close") {
        act(() => result.current.conversationViewProps.onCloseThread());
      } else {
        rerender({ activeConversationId: "room-2", data });
      }
      await act(async () => {
        load.resolve(firstView);
        await opening;
      });

      expect(result.current.activeThreadRootID).toBe("");
      expect(result.current.conversationViewProps.activeThreadView).toBeNull();
      expect(result.current.conversationViewProps.threadLoading).toBe(false);
    });

    it("ignores a previous load when the same thread is reopened", async () => {
      const oldLoad = deferred<ThreadView>();
      const newLoad = deferred<ThreadView>();
      apiMocks.fetchThreadRequest.mockReturnValueOnce(oldLoad.promise).mockReturnValueOnce(newLoad.promise);
      const { result } = renderConversationController({ data: dataWithMessages([firstRoot, secondRoot]) });
      let oldOpening!: Promise<void>;
      let newOpening!: Promise<void>;
      act(() => {
        oldOpening = result.current.conversationViewProps.onOpenThread(firstRoot);
      });
      await act(async () => {
        await result.current.conversationViewProps.onOpenThread(secondRoot);
      });
      act(() => {
        newOpening = result.current.conversationViewProps.onOpenThread(firstRoot);
      });
      await act(async () => {
        oldLoad.reject(new Error("Previous request failed"));
        await oldOpening;
      });

      expect(result.current.conversationViewProps.threadError).toBe("");
      expect(result.current.conversationViewProps.threadLoading).toBe(true);
      await act(async () => {
        newLoad.resolve(firstView);
        await newOpening;
      });
      expect(result.current.conversationViewProps.activeThreadView).toEqual(firstView);
      expect(result.current.conversationViewProps.threadLoading).toBe(false);
    });

    it("keeps a thread selected from another room when navigation completes", async () => {
      const load = deferred<ThreadView>();
      apiMocks.fetchThreadRequest.mockReturnValue(load.promise);
      const data = dataWithMessages([]);
      data.rooms.push({ ...directConversation, id: "room-2", messages: [firstRoot] });
      const { result, rerender } = renderConversationController({ data });
      let opening!: Promise<void>;
      act(() => {
        opening = result.current.openThreadInConversation("room-2", firstRoot);
      });
      rerender({ activeConversationId: "room-2", data });
      await act(async () => {
        load.resolve({ ...firstView, room_id: "room-2" });
        await opening;
      });

      expect(result.current.activeThreadRootID).toBe(firstRoot.id);
      expect(result.current.conversationViewProps.activeThreadView).toMatchObject({
        room_id: "room-2",
        root: { id: firstRoot.id },
      });
    });

    it("does not replace a reopened thread or its cached messages with an older response", async () => {
      const oldLoad = deferred<ThreadView>();
      const newLoad = deferred<ThreadView>();
      const setBootstrapData = vi.fn();
      apiMocks.fetchThreadRequest.mockReturnValueOnce(oldLoad.promise).mockReturnValueOnce(newLoad.promise);
      const { result } = renderConversationController({
        data: dataWithMessages([firstRoot, secondRoot]),
        setBootstrapData,
      });
      let oldOpening!: Promise<void>;
      let newOpening!: Promise<void>;
      act(() => {
        oldOpening = result.current.conversationViewProps.onOpenThread(firstRoot);
      });
      await act(async () => {
        await result.current.conversationViewProps.onOpenThread(secondRoot);
      });
      act(() => {
        newOpening = result.current.conversationViewProps.onOpenThread(firstRoot);
      });
      const newView = { ...firstView, replies: [{ id: "latest-reply", sender_id: "u-demo", content: "Latest reply" }] };
      await act(async () => {
        newLoad.resolve(newView);
        await newOpening;
      });
      await act(async () => {
        oldLoad.resolve(firstView);
        await oldOpening;
      });

      expect(result.current.conversationViewProps.activeThreadView).toEqual(newView);
      expect(setBootstrapData).toHaveBeenCalledTimes(1);
    });

    it.each(["success", "failure"])(
      "keeps the selected thread after a pending reply refresh ends in %s",
      async (outcome) => {
        const refresh = deferred<ThreadView>();
        apiMocks.fetchThreadRequest.mockResolvedValueOnce(firstView).mockReturnValueOnce(refresh.promise);
        apiMocks.sendMessageRequest.mockResolvedValue({
          id: "reply-new",
          content: "Reply",
          sender_id: "u-admin",
          relates_to: { rel_type: "m.thread", event_id: firstRoot.id },
        });
        const { result } = renderConversationController({ data: dataWithMessages([firstRoot, secondRoot]) });
        await act(async () => {
          await result.current.conversationViewProps.onOpenThread(firstRoot);
        });
        act(() => {
          result.current.conversationViewProps.onThreadDraftChange([{ type: "text", text: "Reply" }]);
        });
        let sending!: Promise<void>;
        await act(async () => {
          sending = result.current.conversationViewProps.onSendThreadReply();
          await Promise.resolve();
        });
        expect(apiMocks.fetchThreadRequest).toHaveBeenCalledTimes(2);
        await act(async () => {
          await result.current.conversationViewProps.onOpenThread(secondRoot);
        });
        await act(async () => {
          if (outcome === "success") {
            refresh.resolve(firstView);
          } else {
            refresh.reject(new Error("Previous thread refresh failed"));
          }
          await sending;
        });

        expect(result.current.activeThreadRootID).toBe(secondRoot.id);
        expect(result.current.conversationViewProps.activeThreadView?.root?.id).toBe(secondRoot.id);
        expect(result.current.conversationViewProps.threadError).toBe("");
      },
    );
  });

  it("blocks thread replies when every conversation agent is offline", async () => {
    const root: IMMessage = {
      id: "msg-root",
      content: "Start here",
      created_at: "2026-07-14T09:29:00Z",
      sender_id: "u-admin",
    };
    const { result } = renderConversationController({
      agents: [
        {
          id: "u-demo",
          name: "demo",
          role: "worker",
          runtime_kind: "codex",
          status: "runtime_unavailable",
        },
      ],
      data: dataWithMessages([root]),
    });

    await act(async () => {
      await result.current.conversationViewProps.onOpenThread(root);
    });
    act(() => {
      result.current.conversationViewProps.onThreadDraftChange([{ type: "text", text: "reply" }]);
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendThreadReply();
    });

    expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.conversationViewProps.threadError).toBe("conversationAgentOffline");
  });

  it.each(["failed", "unknown", "exited", "profile_incomplete"])(
    "submits pending offline Codex %s to backend recovery",
    async (state) => {
      apiMocks.sendMessageRequest.mockResolvedValue({ id: "recovery-message", content: "hi", sender_id: "u-admin" });
      const { result } = renderConversationController({
        agents: [
          {
            id: "u-demo",
            name: "demo",
            role: "worker",
            runtime: { kind: "codex", state },
            model_config: { model_provider_id: "fixture", model_id: "test-model", env_restart_required: true },
          },
        ],
      });
      const editor = document.createElement("div");
      editor.textContent = "hi";
      act(() => {
        result.current.conversationViewProps.editorRef.current = editor;
        result.current.conversationViewProps.onSyncComposer();
      });
      await act(async () => {
        await result.current.conversationViewProps.onSendMessage();
      });
      expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
    },
  );

  it("queues a message when a running gateway is temporarily degraded", async () => {
    apiMocks.sendMessageRequest.mockResolvedValue({
      id: "msg-user",
      content: "hi",
      created_at: "2026-07-30T04:00:00Z",
      sender_id: "u-admin",
    });
    const { result } = renderConversationController({
      agents: [
        {
          id: "u-demo",
          name: "demo",
          role: "worker",
          runtime_kind: "codex",
          status: "running",
          runtime: {
            state: "running",
            availability: { state: "degraded", expires_at: "2099-01-01T00:00:00Z" },
          },
        },
      ],
    });
    const editor = document.createElement("div");
    editor.textContent = "hi";
    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });

    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
  });

  it("keeps the draft and opens login instead of sending with an unauthenticated OpenCSG profile", async () => {
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    const { result } = renderConversationController({
      agents: [
        {
          id: "u-demo",
          model_config: { model_provider_id: "opencsg" },
          name: "demo",
          role: "worker",
          status: "running",
        },
      ],
      openCSGAuthGuard,
    });
    const editor = document.createElement("div");
    editor.textContent = "keep this draft";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(openCSGAuthGuard.requireAuthentication).toHaveBeenCalledOnce();
    expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.conversationViewProps.draftText).toBe("keep this draft");
  });

  it("does not require OpenCSG login when the direct-message agent uses another model provider", async () => {
    apiMocks.sendMessageRequest.mockResolvedValue({
      id: "msg-user",
      content: "hello",
      created_at: "2026-09-16T10:00:00Z",
      sender_id: "u-admin",
    });
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    const { result } = renderConversationController({
      agents: [
        {
          id: "u-demo",
          model_provider_id: "default",
          model_id: "qwen3.8-flash",
          name: "demo",
          role: "worker",
          status: "running",
        },
      ],
      managerProfile: { model_provider_id: "opencsg" },
      openCSGAuthGuard,
    });
    const editor = document.createElement("div");
    editor.textContent = "hello";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(openCSGAuthGuard.requireAuthentication).not.toHaveBeenCalled();
    expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
  });

  it("does not infer OpenCSG authentication from the legacy profile provider", async () => {
    apiMocks.sendMessageRequest.mockResolvedValue(successfulMessage());
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    const { result } = renderConversationController({
      agents: [
        {
          id: "u-demo",
          model_config: { provider: "csghub" },
          name: "demo",
          role: "worker",
          status: "running",
        },
      ],
      openCSGAuthGuard,
    });
    const editor = document.createElement("div");
    editor.textContent = "hello";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(openCSGAuthGuard.requireAuthentication).not.toHaveBeenCalled();
    expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
  });

  it("checks only the manager for a human message in an on-demand group", async () => {
    apiMocks.sendMessageRequest.mockResolvedValue(successfulMessage());
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    const conversation: IMConversation = {
      id: "room-on-demand",
      type: RoomTypes.onDemand,
      manager_id: "u-manager",
      members: ["u-admin", "u-manager", "u-dev", "u-qa"],
      messages: [],
      title: "on-demand team",
    };
    const { result } = renderConversationController({
      activeConversationId: conversation.id,
      agents: [
        {
          id: "u-manager",
          model_config: { model_provider_id: "opencsg" },
          name: "manager",
          role: "manager",
          status: "running",
        },
        {
          id: "u-dev",
          model_config: { model_provider_id: "opencsg" },
          name: "dev",
          role: "worker",
          status: "running",
        },
        {
          id: "u-qa",
          model_config: { model_provider_id: "opencsg" },
          name: "qa",
          role: "worker",
          status: "running",
        },
      ],
      data: groupConversationData(conversation),
      managerProfile: { model_provider_id: "default", model_id: "qwen3.8-flash" },
      openCSGAuthGuard,
    });
    const editor = document.createElement("div");
    editor.textContent = "please plan this work";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(openCSGAuthGuard.requireAuthentication).not.toHaveBeenCalled();
    expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
  });

  it("requires OpenCSG login when the on-demand manager handling the message uses OpenCSG", async () => {
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    const conversation: IMConversation = {
      id: "room-on-demand",
      type: RoomTypes.onDemand,
      manager_id: "u-manager",
      members: ["u-admin", "u-manager", "u-dev"],
      messages: [],
      title: "on-demand team",
    };
    const { result } = renderConversationController({
      activeConversationId: conversation.id,
      agents: [
        {
          id: "u-manager",
          model_provider_id: "default",
          name: "manager",
          role: "manager",
          status: "running",
        },
        {
          id: "u-dev",
          model_provider_id: "default",
          name: "dev",
          role: "worker",
          status: "running",
        },
      ],
      data: groupConversationData(conversation),
      managerProfile: { model_provider_id: "opencsg" },
      openCSGAuthGuard,
    });
    const editor = document.createElement("div");
    editor.textContent = "please plan this work";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(openCSGAuthGuard.requireAuthentication).toHaveBeenCalledOnce();
    expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();
  });

  it("handles an unresolved OpenCSG runtime authentication failure already present when the room loads", async () => {
    const runtimeError: IMMessage = {
      id: "runtime-auth-error",
      content: "Model service authentication failed.",
      created_at: "2026-09-16T08:10:00Z",
      sender_id: "u-manager",
      metadata: { csgclaw: { error_code: "authentication_error", runtime_error: true } },
    };
    const conversation: IMConversation = {
      id: "room-on-demand",
      type: RoomTypes.onDemand,
      manager_id: "u-manager",
      members: ["u-admin", "u-manager"],
      messages: [runtimeError],
      title: "on-demand team",
    };
    const openCSGAuthGuard = openCSGAuthGuardStub();
    openCSGAuthGuard.handleRuntimeAuthenticationError = vi.fn(() => true);

    renderConversationController({
      activeConversationId: conversation.id,
      agents: [{ id: "u-manager", name: "manager", role: "manager", status: "running" }],
      data: groupConversationData(conversation),
      managerProfile: { model_provider_id: "opencsg" },
      openCSGAuthGuard,
    });

    await waitFor(() => {
      expect(openCSGAuthGuard.handleRuntimeAuthenticationError).toHaveBeenCalledWith(runtimeError);
    });
  });

  it("does not treat another provider's runtime authentication failure as an OpenCSG logout", () => {
    const runtimeError: IMMessage = {
      id: "runtime-auth-error",
      content: "Model service authentication failed.",
      created_at: "2026-09-16T08:10:00Z",
      sender_id: "u-manager",
      metadata: { csgclaw: { error_code: "authentication_error", runtime_error: true } },
    };
    const conversation: IMConversation = {
      id: "room-on-demand",
      type: RoomTypes.onDemand,
      manager_id: "u-manager",
      members: ["u-admin", "u-manager"],
      messages: [runtimeError],
      title: "on-demand team",
    };
    const openCSGAuthGuard = openCSGAuthGuardStub();
    openCSGAuthGuard.handleRuntimeAuthenticationError = vi.fn(() => true);

    renderConversationController({
      activeConversationId: conversation.id,
      agents: [{ id: "u-manager", name: "manager", role: "manager", status: "running" }],
      data: groupConversationData(conversation),
      managerProfile: { model_provider_id: "default" },
      openCSGAuthGuard,
    });

    expect(openCSGAuthGuard.handleRuntimeAuthenticationError).not.toHaveBeenCalled();
  });

  it("checks only mentioned agents in a free group", async () => {
    apiMocks.sendMessageRequest.mockResolvedValue(successfulMessage());
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    const conversation: IMConversation = {
      id: "room-free",
      type: RoomTypes.free,
      members: ["u-admin", "u-dev", "u-qa"],
      messages: [],
      notify_all_agents: false,
      title: "free team",
    };
    const agents: AgentLike[] = [
      {
        id: "u-dev",
        model_provider_id: "default",
        name: "dev",
        role: "worker",
        status: "running",
      },
      {
        id: "u-qa",
        model_config: { model_provider_id: "opencsg" },
        name: "qa",
        role: "worker",
        status: "running",
      },
    ];
    const { result } = renderConversationController({
      activeConversationId: conversation.id,
      agents,
      data: groupConversationData(conversation),
      openCSGAuthGuard,
    });
    const editor = composerEditorWithMention("u-dev", "dev", " please review this");

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(openCSGAuthGuard.requireAuthentication).not.toHaveBeenCalled();
    expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
  });

  it("requires OpenCSG login for a mentioned OpenCSG agent or an OpenCSG agent notified by the room", async () => {
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    const agents: AgentLike[] = [
      {
        id: "u-dev",
        model_provider_id: "default",
        name: "dev",
        role: "worker",
        status: "running",
      },
      {
        id: "u-qa",
        model_config: { model_provider_id: "opencsg" },
        name: "qa",
        role: "worker",
        status: "running",
      },
    ];
    const mentionedConversation: IMConversation = {
      id: "room-free-mentioned",
      type: RoomTypes.free,
      members: ["u-admin", "u-dev", "u-qa"],
      messages: [],
      notify_all_agents: false,
      title: "free team",
    };
    const mentioned = renderConversationController({
      activeConversationId: mentionedConversation.id,
      agents,
      data: groupConversationData(mentionedConversation),
      openCSGAuthGuard,
    });
    const mentionedEditor = composerEditorWithMention("u-qa", "qa", " please test this");

    act(() => {
      mentioned.result.current.conversationViewProps.editorRef.current = mentionedEditor;
      mentioned.result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await mentioned.result.current.conversationViewProps.onSendMessage();
    });
    expect(openCSGAuthGuard.requireAuthentication).toHaveBeenCalledOnce();
    expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();

    mentioned.unmount();
    openCSGAuthGuard.requireAuthentication.mockClear();
    const notifyAllConversation: IMConversation = {
      ...mentionedConversation,
      id: "room-free-notify-all",
      notify_all_agents: true,
    };
    const notifyAll = renderConversationController({
      activeConversationId: notifyAllConversation.id,
      agents,
      data: groupConversationData(notifyAllConversation),
      openCSGAuthGuard,
    });
    const notifyAllEditor = document.createElement("div");
    notifyAllEditor.textContent = "please review this";

    act(() => {
      notifyAll.result.current.conversationViewProps.editorRef.current = notifyAllEditor;
      notifyAll.result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await notifyAll.result.current.conversationViewProps.onSendMessage();
    });

    expect(openCSGAuthGuard.requireAuthentication).toHaveBeenCalledOnce();
    expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();
  });

  it("shows the specific manager runtime warning when sending is blocked", async () => {
    const { result } = renderConversationController({
      managerRuntimeUnavailable: true,
      managerRuntimeWarning: "Docker is not running",
      agents: [{ id: "u-demo", role: "manager", status: "running" }],
    });
    const editor = document.createElement("div");
    editor.textContent = "hi";

    act(() => {
      result.current.conversationViewProps.editorRef.current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    expect(apiMocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.conversationViewProps.composerError).toBe("Docker is not running");
  });

  it("keeps member management unavailable in direct messages", () => {
    const { result } = renderConversationController();

    expect(result.current.conversationViewProps.inviteActionLabel).toBe("memberManagement");

    act(() => {
      result.current.conversationViewProps.onInviteAction();
    });

    expect(result.current.conversationViewProps.memberPanelProps).toBeNull();
  });

  it("keeps external membership changes in the current room member panel", async () => {
    const conversation = { ...directConversation, is_direct: false, members: ["u-admin", "u-dev"] };
    const data = groupConversationData(conversation);
    const setBootstrapData = vi.fn();
    const updated = { ...conversation, members: [...conversation.members, "source"] };
    apiMocks.inviteRoomUsersRequest.mockResolvedValue(updated);
    apiMocks.removeRoomUserRequest.mockResolvedValue(conversation);
    const { result } = renderConversationController({ data, setBootstrapData });
    act(() => result.current.conversationViewProps.onInviteAction());
    await act(async () => {
      await result.current.conversationViewProps.memberPanelProps?.onExternalMembershipChange("source", true);
    });
    expect(apiMocks.inviteRoomUsersRequest).toHaveBeenCalledWith({
      room_id: conversation.id,
      inviter_id: "u-admin",
      user_ids: ["source"],
      locale: "en",
    });
    expect(setBootstrapData).toHaveBeenCalled();
    expect(result.current.conversationViewProps.memberPanelProps).not.toBeNull();
    await act(async () => {
      await result.current.conversationViewProps.memberPanelProps?.onExternalMembershipChange("source", false);
    });
    expect(apiMocks.removeRoomUserRequest).toHaveBeenCalledWith({
      room_id: conversation.id,
      inviter_id: "u-admin",
      member_id: "source",
      locale: "en",
    });
    expect(result.current.conversationViewProps.memberPanelProps).not.toBeNull();
  });

  it("retains invitation selection after failure and prevents duplicate submissions", async () => {
    const conversation = { ...directConversation, is_direct: false, members: ["u-admin", "u-dev"] };
    const request = deferred<IMConversation>();
    apiMocks.inviteRoomUsersRequest.mockReturnValue(request.promise);
    const { result } = renderConversationController({ data: groupConversationData(conversation) });
    act(() => result.current.conversationViewProps.onInviteAction());
    act(() => result.current.conversationViewProps.memberPanelProps?.onInviteUserIDsChange(["u-qa"]));
    let pending: Promise<void> | undefined;
    act(() => {
      pending = result.current.conversationViewProps.memberPanelProps?.onInvite();
      void result.current.conversationViewProps.memberPanelProps?.onInvite();
    });
    expect(apiMocks.inviteRoomUsersRequest).toHaveBeenCalledOnce();
    expect(result.current.conversationViewProps.memberPanelProps?.inviteBusy).toBe(true);
    await act(async () => {
      request.reject(new Error("Invitation failed"));
      await pending;
    });
    expect(result.current.conversationViewProps.memberPanelProps).toMatchObject({
      inviteUserIDs: ["u-qa"],
      inviteBusy: false,
      submitError: "Invitation failed",
    });
  });

  it("keeps member panels independent between rooms and ignores the previous room's request error", async () => {
    const conversation = { ...directConversation, is_direct: false, members: ["u-admin", "u-dev"] };
    const nextConversation = { ...conversation, id: "room-next" };
    const data = { ...groupConversationData(conversation), rooms: [conversation, nextConversation] };
    const request = deferred<IMConversation>();
    apiMocks.inviteRoomUsersRequest.mockReturnValue(request.promise);
    const { result, rerender } = renderConversationController({ data });
    act(() => result.current.conversationViewProps.onInviteAction());
    let pending: Promise<void> | undefined;
    act(() => {
      pending = result.current.conversationViewProps.memberPanelProps?.onExternalMembershipChange("source", true);
    });
    rerender({ activeConversationId: "room-next", data, messageListActive: true });
    expect(result.current.conversationViewProps.memberPanelProps).toBeNull();
    act(() => result.current.conversationViewProps.onInviteAction());
    expect(result.current.conversationViewProps.memberPanelProps?.memberActionBusyID).toBe("source");
    await act(async () => {
      request.reject(new Error("Old room request failed"));
      await pending;
    });
    expect(result.current.conversationViewProps.memberPanelProps?.memberActionError).toBe("");
  });

  it("restores each room's member panel without carrying invitation selections or errors", async () => {
    const conversation = { ...directConversation, is_direct: false, members: ["u-admin", "u-dev"] };
    const nextConversation = { ...conversation, id: "room-next" };
    const data = { ...groupConversationData(conversation), rooms: [conversation, nextConversation] };
    apiMocks.inviteRoomUsersRequest.mockRejectedValue(new Error("Invitation failed"));
    const { result, rerender } = renderConversationController({ data });
    act(() => result.current.conversationViewProps.onInviteAction());
    act(() => result.current.conversationViewProps.memberPanelProps?.onInviteUserIDsChange(["u-qa"]));
    await act(async () => result.current.conversationViewProps.memberPanelProps?.onInvite());
    expect(result.current.conversationViewProps.memberPanelProps?.submitError).toBe("Invitation failed");
    rerender({ activeConversationId: "room-next", data, messageListActive: true });
    expect(result.current.conversationViewProps.memberPanelProps).toBeNull();
    act(() => result.current.conversationViewProps.onInviteAction());
    act(() => result.current.conversationViewProps.memberPanelProps?.onInviteUserIDsChange(["u-manager"]));
    rerender({ activeConversationId: conversation.id, data, messageListActive: true });
    expect(result.current.conversationViewProps.memberPanelProps).toMatchObject({
      inviteUserIDs: [],
      submitError: "",
      memberActionError: "",
    });
    rerender({ activeConversationId: "room-next", data, messageListActive: true });
    expect(result.current.conversationViewProps.memberPanelProps?.inviteUserIDs).toEqual([]);
    act(() => result.current.conversationViewProps.memberPanelProps?.onClose());
    rerender({ activeConversationId: conversation.id, data, messageListActive: true });
    expect(result.current.conversationViewProps.memberPanelProps).not.toBeNull();
  });

  it.each([false, true])("completes an invitation with another room selected: %s", async (changeRoom) => {
    const conversation = { ...directConversation, is_direct: false, members: ["u-admin", "u-dev"] };
    const nextConversation = { ...conversation, id: "room-next" };
    const data = { ...groupConversationData(conversation), rooms: [conversation, nextConversation] };
    const request = deferred<IMConversation>();
    apiMocks.inviteRoomUsersRequest.mockReturnValue(request.promise);
    const { result, rerender, unmount } = renderConversationController({ data });
    act(() => result.current.conversationViewProps.onInviteAction());
    act(() => result.current.conversationViewProps.memberPanelProps?.onInviteUserIDsChange(["u-qa"]));
    let pending: Promise<void> | undefined;
    act(() => {
      pending = result.current.conversationViewProps.memberPanelProps?.onInvite();
    });
    if (changeRoom) {
      rerender({ activeConversationId: nextConversation.id, data, messageListActive: true });
      act(() => result.current.conversationViewProps.onInviteAction());
    }
    await act(async () => {
      request.resolve({ ...conversation, members: [...conversation.members, "u-qa"] });
      await pending;
    });
    if (changeRoom) expect(result.current.conversationViewProps.memberPanelProps).not.toBeNull();
    else expect(result.current.conversationViewProps.memberPanelProps).toBeNull();
    unmount();
    const restored = renderConversationController({
      data,
      activeConversationId: changeRoom ? nextConversation.id : conversation.id,
    });
    if (changeRoom) expect(restored.result.current.conversationViewProps.memberPanelProps).not.toBeNull();
    else expect(restored.result.current.conversationViewProps.memberPanelProps).toBeNull();
  });

  it("scopes the saved member panel to the current user and hides it in direct conversations", () => {
    const conversation = { ...directConversation, is_direct: false, members: ["u-admin", "u-dev"] };
    const data = groupConversationData(conversation);
    const { result, rerender } = renderConversationController({ data });
    act(() => result.current.conversationViewProps.onInviteAction());
    rerender({
      activeConversationId: conversation.id,
      data: { ...data, current_user_id: "u-dev" },
      messageListActive: true,
    });
    expect(result.current.conversationViewProps.memberPanelProps).toBeNull();
    rerender({ activeConversationId: conversation.id, data, messageListActive: true });
    expect(result.current.conversationViewProps.memberPanelProps).not.toBeNull();
    rerender({
      activeConversationId: conversation.id,
      data: { ...data, rooms: [{ ...conversation, is_direct: true }] },
      messageListActive: true,
    });
    expect(result.current.conversationViewProps.memberPanelProps).toBeNull();
  });

  it("does not mark a participant working from message send or activity", async () => {
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

    expect(result.current.conversationViewProps.workingParticipants).toEqual([]);

    act(() => {
      result.current.handleRealtimeEvent({
        type: "message.created",
        room_id: "room-1",
        message: {
          id: "msg-tool",
          content: '📄 Web Fetch: from https://example.com {"url":"https://example.com"}',
          created_at: "2026-06-16T10:00:10Z",
          metadata: { codex: { delivery_kind: "tool", request_id: "msg-user" } },
          sender_id: "u-demo",
        },
      });
    });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([]);

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

  it("tracks upload progress, stops an in-flight send, and keeps the draft retryable", async () => {
    apiMocks.sendMessageRequest.mockImplementation(
      (_payload, options: SendMessageRequestOptions) =>
        new Promise((_resolve, reject) => {
          options.signal?.addEventListener(
            "abort",
            () => reject(new DOMException("The request was aborted.", "AbortError")),
            { once: true },
          );
        }),
    );
    const { result } = renderConversationController();
    const file = new File(["report"], "report.pdf", { type: "application/pdf" });

    act(() => {
      result.current.conversationViewProps.onAddAttachments?.([file]);
    });
    let sendPromise: Promise<void> | undefined;
    act(() => {
      sendPromise = result.current.conversationViewProps.onSendMessage() as Promise<void>;
    });

    expect(result.current.conversationViewProps.sendStatus).toBe("sending");
    const options = apiMocks.sendMessageRequest.mock.calls[0][1] as SendMessageRequestOptions;
    act(() => {
      options.onUploadProgress?.(48);
    });
    expect(result.current.conversationViewProps.sendProgress).toBe(48);

    act(() => {
      result.current.conversationViewProps.onStopSend?.();
    });
    await act(async () => {
      await sendPromise;
    });

    expect(options.signal?.aborted).toBe(true);
    expect(result.current.conversationViewProps.sendStatus).toBe("failed");
    expect(result.current.conversationViewProps.sendError).toBe("sendStopped");
    expect(result.current.conversationViewProps.attachmentDrafts).toHaveLength(1);
  });

  it("restores a removed attachment and reports duplicate selections", () => {
    const { result } = renderConversationController();
    const file = new File(["same"], "report.pdf", { type: "application/pdf" });

    act(() => {
      result.current.conversationViewProps.onAddAttachments?.([file]);
    });
    const draftID = result.current.conversationViewProps.attachmentDrafts?.[0]?.id;
    expect(draftID).toBeTruthy();

    act(() => {
      result.current.conversationViewProps.onAddAttachments?.([file]);
    });
    expect(result.current.conversationViewProps.composerError).toBe("attachmentDuplicate");

    act(() => {
      result.current.conversationViewProps.onRemoveAttachment?.(draftID || "");
    });
    expect(result.current.conversationViewProps.attachmentDrafts).toHaveLength(0);
    expect(result.current.conversationViewProps.removedAttachmentCount).toBe(1);
    expect(result.current.conversationViewProps.removedAttachmentName).toBe("report.pdf");

    act(() => {
      result.current.conversationViewProps.onUndoRemoveAttachment?.();
    });
    expect(result.current.conversationViewProps.attachmentDrafts).toHaveLength(1);
    expect(result.current.conversationViewProps.removedAttachmentCount).toBe(0);
    expect(result.current.conversationViewProps.removedAttachmentName).toBe("");
  });

  it("restores a batch of quickly removed attachments in their original order", () => {
    const { result } = renderConversationController();
    const files = [
      new File(["a"], "a.txt", { type: "text/plain" }),
      new File(["b"], "b.txt", { type: "text/plain" }),
      new File(["c"], "c.txt", { type: "text/plain" }),
    ];

    act(() => {
      result.current.conversationViewProps.onAddAttachments?.(files);
    });
    const attachmentIDs = result.current.conversationViewProps.attachmentDrafts?.map((draft) => draft.id) ?? [];
    for (const attachmentID of attachmentIDs) {
      act(() => {
        result.current.conversationViewProps.onRemoveAttachment?.(attachmentID);
      });
    }

    expect(result.current.conversationViewProps.attachmentDrafts).toHaveLength(0);
    expect(result.current.conversationViewProps.removedAttachmentCount).toBe(3);
    expect(result.current.conversationViewProps.removedAttachmentName).toBe("");

    act(() => {
      result.current.conversationViewProps.onUndoRemoveAttachment?.();
    });

    expect(result.current.conversationViewProps.attachmentDrafts?.map((draft) => draft.name)).toEqual([
      "a.txt",
      "b.txt",
      "c.txt",
    ]);
    expect(result.current.conversationViewProps.removedAttachmentCount).toBe(0);
  });

  it("clears duplicate attachment errors when switching conversations", () => {
    const otherConversation: IMConversation = {
      ...directConversation,
      id: "room-2",
      title: "other agent",
    };
    const data: IMData = {
      ...dataWithMessages([]),
      rooms: [directConversation, otherConversation],
    };
    const { result, rerender } = renderConversationController({ data });
    const file = new File(["same"], "report.pdf", { type: "application/pdf" });

    act(() => {
      result.current.conversationViewProps.onAddAttachments?.([file]);
    });
    act(() => {
      result.current.conversationViewProps.onAddAttachments?.([file]);
    });
    expect(result.current.conversationViewProps.composerError).toBe("attachmentDuplicate");

    rerender({ activeConversationId: "room-2", data, messageListActive: true });
    expect(result.current.conversationViewProps.composerError).toBe("");

    rerender({ activeConversationId: directConversation.id, data, messageListActive: true });
    expect(result.current.conversationViewProps.composerError).toBe("");
  });

  describe("attachment warnings", () => {
    const root: IMMessage = { id: "msg-root", content: "Start here", sender_id: "u-admin" };

    function oversizedFile(): File {
      const file = new File(["large"], "large.txt", { type: "text/plain" });
      Object.defineProperty(file, "size", { value: MAX_ATTACHMENT_FILE_BYTES + 1 });
      return file;
    }

    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    it("expires a composer warning after five seconds and restarts the timeout for the same warning", () => {
      const { result } = renderConversationController();
      const file = oversizedFile();

      act(() => result.current.conversationViewProps.onAddAttachments?.([file]));
      expect(result.current.conversationViewProps.composerError).toBe("attachmentTooLarge");
      act(() => vi.advanceTimersByTime(5000));
      expect(result.current.conversationViewProps.composerError).toBe("");

      act(() => result.current.conversationViewProps.onAddAttachments?.([file]));
      act(() => vi.advanceTimersByTime(4000));
      act(() => result.current.conversationViewProps.onAddAttachments?.([file]));
      act(() => vi.advanceTimersByTime(1000));
      expect(result.current.conversationViewProps.composerError).toBe("attachmentTooLarge");
      act(() => vi.advanceTimersByTime(4000));
      expect(result.current.conversationViewProps.composerError).toBe("");
      expect(result.current.conversationViewProps.attachmentDrafts).toHaveLength(0);
    });

    it("clears a warning on room changes without expiring the next room's warning early", () => {
      const data: IMData = {
        ...dataWithMessages([]),
        rooms: [directConversation, { ...directConversation, id: "room-2" }],
      };
      const { result, rerender, unmount } = renderConversationController({ data });
      const file = oversizedFile();

      act(() => result.current.conversationViewProps.onAddAttachments?.([file]));
      act(() => vi.advanceTimersByTime(4000));
      rerender({ activeConversationId: "room-2", data });
      expect(result.current.conversationViewProps.composerError).toBe("");
      act(() => result.current.conversationViewProps.onAddAttachments?.([file]));
      act(() => vi.advanceTimersByTime(1000));
      expect(result.current.conversationViewProps.composerError).toBe("attachmentTooLarge");

      rerender({ activeConversationId: directConversation.id, data });
      expect(result.current.conversationViewProps.composerError).toBe("");
      act(() => result.current.conversationViewProps.onAddAttachments?.([file]));
      unmount();
      expect(vi.getTimerCount()).toBe(0);
    });

    it("expires thread warnings and clears them when the thread changes or closes", async () => {
      const secondRoot: IMMessage = { id: "msg-second", content: "Another thread", sender_id: "u-admin" };
      const { result, unmount } = renderConversationController({ data: dataWithMessages([root, secondRoot]) });
      const file = oversizedFile();
      await act(async () => result.current.conversationViewProps.onOpenThread(root));

      act(() => result.current.conversationViewProps.onAddThreadAttachments?.([file]));
      expect(result.current.conversationViewProps.threadError).toBe("attachmentTooLarge");
      act(() => vi.advanceTimersByTime(5000));
      expect(result.current.conversationViewProps.threadError).toBe("");

      act(() => result.current.conversationViewProps.onAddThreadAttachments?.([file]));
      act(() => vi.advanceTimersByTime(4000));
      await act(async () => result.current.conversationViewProps.onOpenThread(secondRoot));
      expect(result.current.conversationViewProps.threadError).toBe("");
      act(() => result.current.conversationViewProps.onAddThreadAttachments?.([file]));
      act(() => vi.advanceTimersByTime(1000));
      expect(result.current.conversationViewProps.threadError).toBe("attachmentTooLarge");
      act(() => result.current.conversationViewProps.onCloseThread());
      await act(async () => result.current.conversationViewProps.onOpenThread(secondRoot));
      expect(result.current.conversationViewProps.threadError).toBe("");

      act(() => result.current.conversationViewProps.onAddThreadAttachments?.([file]));
      unmount();
      expect(vi.getTimerCount()).toBe(0);
    });

    it.each(["composer", "thread"])("preserves a %s send failure when an attachment warning expires", async (scope) => {
      apiMocks.sendMessageRequest.mockRejectedValue(new Error("Upload failed. Try again."));
      const { result } = renderConversationController({ data: dataWithMessages([root]) });
      const file = new File(["valid"], "valid.txt", { type: "text/plain" });
      if (scope === "thread") {
        await act(async () => result.current.conversationViewProps.onOpenThread(root));
      }
      const addAttachments = () => {
        const props = result.current.conversationViewProps;
        return scope === "thread" ? props.onAddThreadAttachments : props.onAddAttachments;
      };
      const displayedError = () => {
        const props = result.current.conversationViewProps;
        return scope === "thread" ? props.threadError : props.composerError;
      };
      act(() => addAttachments()?.([oversizedFile()]));
      expect(displayedError()).toBe("attachmentTooLarge");
      act(() => addAttachments()?.([file]));
      expect(displayedError()).toBe("");
      act(() => addAttachments()?.([oversizedFile()]));
      expect(displayedError()).toBe("attachmentTooLarge");
      act(() => {
        const props = result.current.conversationViewProps;
        if (scope === "thread") {
          props.onRemoveThreadAttachment?.(props.threadAttachmentDrafts?.[0]?.id ?? "");
        } else {
          props.onRemoveAttachment?.(props.attachmentDrafts?.[0]?.id ?? "");
        }
      });
      expect(displayedError()).toBe("");
      act(() => addAttachments()?.([file]));
      act(() => addAttachments()?.([oversizedFile()]));

      await act(async () => {
        const props = result.current.conversationViewProps;
        await (scope === "thread" ? props.onSendThreadReply() : props.onSendMessage());
      });
      expect(apiMocks.sendMessageRequest).toHaveBeenCalledOnce();
      expect(displayedError()).toBe("Upload failed. Try again.");
      act(() => vi.advanceTimersByTime(5000));
      expect(displayedError()).toBe("Upload failed. Try again.");
      act(() => addAttachments()?.([oversizedFile()]));
      act(() => vi.advanceTimersByTime(5000));
      expect(displayedError()).toBe("Upload failed. Try again.");
    });
  });

  it("does not derive working participants from recent message history", () => {
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
          metadata: { codex: { delivery_kind: "tool", request_id: "msg-user" } },
          sender_id: "u-demo",
        },
      ]),
    });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([]);
  });

  it("uses the participant work lease selector as the authoritative working source", () => {
    const workingParticipantsForRoom = vi.fn(() => [{ id: "u-demo", name: "demo" }]);
    const { result } = renderConversationController({ workingParticipantsForRoom });

    expect(result.current.conversationViewProps.workingParticipants).toEqual([{ id: "u-demo", name: "demo" }]);
    expect(workingParticipantsForRoom).toHaveBeenCalledWith("room-1");
  });

  it("scrolls the message list to the bottom when it becomes active", () => {
    const { result, rerender } = renderConversationController({ messageListActive: false });
    const messageList = createScrollableMessageList(900, 240);
    result.current.conversationViewProps.messageListRef.current = messageList;

    expect(messageList.scrollTop).toBe(0);

    rerender({ messageListActive: true });

    expect(messageList.scrollTop).toBe(900);
  });

  it("scrolls to the bottom when an already-active message list mounts after reload data arrives", async () => {
    const { result, rerender } = renderConversationController({
      data: dataWithMessages([]),
      messageListActive: true,
    });
    const messageList = createScrollableMessageList(900, 240);

    expect(messageList.scrollTop).toBe(0);
    result.current.conversationViewProps.messageListRef.current = messageList;

    rerender({
      data: dataWithMessages([
        {
          id: "msg-after-reload",
          content: "latest message",
          created_at: "2026-07-15T08:00:00Z",
          sender_id: "u-demo",
        },
      ]),
      messageListActive: true,
    });

    await act(async () => {
      await nextAnimationFrame();
    });
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

  it("preserves history position without announcing the user's own message", async () => {
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
    rerender({
      data: dataWithMessages([
        firstMessage,
        {
          id: "msg-user",
          content: "my reply",
          created_at: "2026-06-16T10:01:00Z",
          sender_id: "u-admin",
        },
      ]),
      messageListActive: true,
    });

    await waitFor(() => {
      expect(messageList.scrollTop).toBe(120);
      expect(result.current.conversationViewProps.hasNewMessages).toBe(false);
    });
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
