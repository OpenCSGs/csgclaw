import { act, renderHook } from "@testing-library/react";
import { buildUsersById, type IMServerEvent, type TranslateFn } from "@/models/conversations";
import { buildTurnNotificationTag, TurnNotificationModes, type TurnNotificationMode } from "@/models/turnNotifications";
import { useAgentTurnNotifications } from "@/hooks/workspace/useAgentTurnNotifications";
import type { DesktopBridge } from "@/shared/platform/desktopBridge";

type NotificationRecord = {
  body: string;
  silent: boolean;
  close: ReturnType<typeof vi.fn>;
  notification: FakeNotification;
  tag: string;
  title: string;
};

const notificationRecords: NotificationRecord[] = [];

class FakeNotification {
  static permission: NotificationPermission = "granted";
  static requestPermission = vi.fn(async () => FakeNotification.permission);

  onclick: ((this: Notification, ev: Event) => unknown) | null = null;
  onerror: ((this: Notification, ev: Event) => unknown) | null = null;
  private readonly closeMock = vi.fn();

  constructor(title: string, options: NotificationOptions = {}) {
    notificationRecords.push({
      body: options.body ?? "",
      silent: options.silent ?? false,
      close: this.closeMock,
      notification: this,
      tag: options.tag ?? "",
      title,
    });
  }

  close() {
    this.closeMock();
  }
}

const t: TranslateFn = (key, params = {}) => {
  if (key === "turnNotificationTitle") {
    return `${params.agent} finished replying`;
  }
  if (key === "turnNotificationBody") {
    return `${params.room}: ${params.message}`;
  }
  if (key === "turnNotificationRoomBody") {
    return `Room: ${params.room}`;
  }
  if (key === "turnNotificationDefaultBody") {
    return "The agent turn has finished.";
  }
  if (key === "errors.upstream_unavailable") {
    return "The model service is temporarily unavailable.";
  }
  return key;
};

function workEvent(overrides: Partial<NonNullable<IMServerEvent["work"]>> = {}): IMServerEvent {
  return {
    type: "participant.work.updated",
    work: {
      expires_at: "2026-08-11T12:00:15Z",
      kind: "agent_turn",
      lease_id: "lease-1",
      participant_id: "pt-worker",
      reason: "started",
      registry_epoch: "epoch-1",
      request_id: "message-1",
      revision: 1,
      room_id: "room-1",
      state: "working",
      user_id: "user-worker",
      ...overrides,
    },
  };
}

function renderNotifications(
  mode: TurnNotificationMode = TurnNotificationModes.whenUnfocused,
  options: { isDirect?: boolean; roomTitle?: string; soundEnabled?: boolean } = {},
) {
  const onSelectConversation = vi.fn();
  const users = [
    { id: "user-admin", name: "Admin" },
    { id: "user-worker", name: "Worker roster" },
  ];
  const rendered = renderHook(() =>
    useAgentTurnNotifications({
      agents: [{ id: "agent-worker", name: "Research Agent", user_id: "user-worker" }],
      currentUserID: "user-admin",
      mode,
      soundEnabled: options.soundEnabled,
      onSelectConversation,
      rooms: [
        {
          id: "room-1",
          is_direct: options.isDirect,
          members: ["user-admin", "user-worker"],
          messages: [
            {
              content: "The report is ready.",
              id: "reply-1",
              sender_id: "user-worker",
              metadata: { codex: { delivery_kind: "final", request_id: "message-1" } },
            },
          ],
          title: options.roomTitle ?? "Research room",
        },
      ],
      t,
      usersById: buildUsersById(users),
    }),
  );
  return { ...rendered, onSelectConversation };
}

function desktopBridge(showWindow: DesktopBridge["showWindow"]): DesktopBridge {
  return {
    getRuntimeInfo: vi.fn(),
    openOAuth: vi.fn(),
    checkForUpdates: vi.fn(),
    installDownloadedUpdate: vi.fn(),
    restartSidecar: vi.fn(),
    showWindow,
    showTurnNotification: vi.fn().mockResolvedValue(undefined),
    onTurnNotificationClick: vi.fn(() => vi.fn()),
    setUpdateChannel: vi.fn(),
    setThemeSource: vi.fn(),
    onUpdateStatus: vi.fn(),
    getWindowState: vi.fn(),
    onWindowState: vi.fn(),
    browserCommand: vi.fn(),
    onBrowserState: vi.fn(),
  };
}

describe("useAgentTurnNotifications", () => {
  beforeEach(() => {
    notificationRecords.length = 0;
    FakeNotification.permission = "granted";
    FakeNotification.requestPermission.mockClear();
    vi.stubGlobal("Notification", FakeNotification);
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
    vi.spyOn(window, "focus").mockImplementation(() => undefined);
  });

  afterEach(() => {
    delete window.csgclawDesktop;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("notifies once when an observed agent turn is released", () => {
    const { result, onSelectConversation } = renderNotifications();

    act(() => result.current.handleRealtimeEvent(workEvent()));
    expect(notificationRecords).toHaveLength(0);

    const released = workEvent({ reason: "released", revision: 2, state: "idle" });
    act(() => result.current.handleRealtimeEvent(released));
    act(() => result.current.handleRealtimeEvent(released));

    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]).toMatchObject({
      body: "Research room: The report is ready.",
      tag: buildTurnNotificationTag(JSON.stringify(["room-1", "agent-worker", "turn:message-1"])),
      title: "Research Agent finished replying",
    });

    notificationRecords[0]?.notification.onclick?.call(
      notificationRecords[0].notification as unknown as Notification,
      new Event("click"),
    );
    expect(notificationRecords[0]?.close).toHaveBeenCalledTimes(1);
    expect(window.focus).toHaveBeenCalledTimes(1);
    expect(onSelectConversation).toHaveBeenCalledWith("room-1");
  });

  it("uses the native desktop notification and selects its room on click", async () => {
    const bridge = desktopBridge(vi.fn());
    window.csgclawDesktop = bridge;
    const { result, onSelectConversation } = renderNotifications(TurnNotificationModes.always);
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));
    expect(bridge.showTurnNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        body: "Research room: The report is ready.",
        roomID: "room-1",
        title: "Research Agent finished replying",
      }),
    );
    expect(notificationRecords).toHaveLength(0);
    const listener = vi.mocked(bridge.onTurnNotificationClick).mock.calls[0]?.[0];
    act(() => listener?.("room-1"));
    expect(onSelectConversation).toHaveBeenCalledWith("room-1");
  });

  it.each([true, false])("always notifies the focused desktop and respects sound=%s", (soundEnabled) => {
    vi.mocked(document.hasFocus).mockReturnValue(true);
    const bridge = desktopBridge(vi.fn());
    window.csgclawDesktop = bridge;
    const { result } = renderNotifications(TurnNotificationModes.always, { soundEnabled });
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", state: "idle" })));
    expect(bridge.showTurnNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        silent: !soundEnabled,
        title: "Research Agent finished replying",
        body: "Research room: The report is ready.",
      }),
    );
  });

  it("keeps browser notification banners when sounds are disabled", () => {
    const { result } = renderNotifications(TurnNotificationModes.always, { soundEnabled: false });
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", state: "idle" })));
    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]?.silent).toBe(true);
  });

  it("uses the shared notification permission API on desktop", async () => {
    FakeNotification.permission = "denied";
    const bridge = desktopBridge(vi.fn());
    window.csgclawDesktop = bridge;
    const { result } = renderNotifications(TurnNotificationModes.always);
    expect(result.current.permission).toBe("denied");
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", state: "idle" })));
    expect(bridge.showTurnNotification).not.toHaveBeenCalled();
    FakeNotification.requestPermission.mockImplementationOnce(async () => {
      FakeNotification.permission = "granted";
      return "granted";
    });
    await act(async () => {
      await expect(result.current.requestPermission()).resolves.toBe("granted");
    });
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", state: "idle" })));
    expect(bridge.showTurnNotification).toHaveBeenCalledOnce();
    expect(result.current.permission).toBe("granted");
    expect(FakeNotification.requestPermission).toHaveBeenCalledOnce();
  });

  it("uses an explicit final reply for agents without work lease events", () => {
    const { result } = renderNotifications(TurnNotificationModes.always);

    act(() =>
      result.current.handleRealtimeEvent({
        message: {
          content: "Fallback reply",
          metadata: { codex: { delivery_kind: "final" } },
          id: "reply-2",
          sender_id: "user-worker",
        },
        room_id: "room-1",
        type: "message.created",
      }),
    );

    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]?.body).toBe("Research room: Fallback reply");
  });

  it.each([false, true])("sends a plain-text first-line preview with desktop=%s", (desktop) => {
    const bridge = desktopBridge(vi.fn());
    if (desktop) window.csgclawDesktop = bridge;
    const { result } = renderNotifications(TurnNotificationModes.always, { isDirect: true });
    act(() =>
      result.current.handleRealtimeEvent({
        type: "message.created",
        room_id: "room-1",
        message: {
          id: "markdown-reply",
          sender_id: "user-worker",
          content: "## 🌤️ **上海今日天气**\n根据中国天气网的数据：\n| 项目 | 内容 |",
          metadata: { codex: { delivery_kind: "final" } },
        },
      }),
    );
    if (desktop) {
      expect(bridge.showTurnNotification).toHaveBeenCalledWith(expect.objectContaining({ body: "🌤️ 上海今日天气……" }));
      expect(notificationRecords).toHaveLength(0);
    } else {
      expect(notificationRecords).toHaveLength(1);
      expect(notificationRecords[0]?.body).toBe("🌤️ 上海今日天气……");
    }
  });

  it("does not describe a runtime failure as a completed reply", () => {
    const { result } = renderNotifications(TurnNotificationModes.always);

    act(() =>
      result.current.handleRealtimeEvent({
        message: {
          content: "The model service is temporarily unavailable. Try again later.",
          id: "reply-runtime-error",
          metadata: { csgclaw: { runtime_error: true } },
          sender_id: "user-worker",
        },
        room_id: "room-1",
        type: "message.created",
      }),
    );

    expect(notificationRecords).toHaveLength(0);
  });

  it("keeps the visible final reply fallback after observing work and deduplicates the release", () => {
    const { result } = renderNotifications(TurnNotificationModes.always);

    act(() => result.current.handleRealtimeEvent(workEvent()));
    act(() =>
      result.current.handleRealtimeEvent({
        message: {
          content: "Fallback after reconnect",
          id: "reply-3",
          metadata: {
            codex: { delivery_kind: "final", request_id: "message-1" },
          },
          sender_id: "user-worker",
        },
        room_id: "room-1",
        type: "message.created",
      }),
    );
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));

    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]).toMatchObject({
      body: "Research room: Fallback after reconnect",
      tag: buildTurnNotificationTag(JSON.stringify(["room-1", "agent-worker", "turn:message-1"])),
    });
  });

  it("uses a bounded tag for a scheduled task and retries after a native notification error", () => {
    const { result } = renderNotifications(TurnNotificationModes.always);
    const scheduledRequestID = "agent-task-task-scheduled-run-1-created";
    const released = workEvent({
      reason: "released",
      request_id: scheduledRequestID,
      revision: 2,
      state: "idle",
    });

    act(() => result.current.handleRealtimeEvent(released));

    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]?.tag).toBe(
      buildTurnNotificationTag(JSON.stringify(["room-1", "agent-worker", `turn:${scheduledRequestID}`])),
    );
    expect(notificationRecords[0]?.tag).toHaveLength(24);
    expect(notificationRecords[0]?.body).toBe("Room: Research room");

    act(() => {
      notificationRecords[0]?.notification.onerror?.call(
        notificationRecords[0].notification as unknown as Notification,
        new Event("error"),
      );
    });
    act(() => result.current.handleRealtimeEvent(released));

    expect(notificationRecords).toHaveLength(2);
  });

  it("suppresses unfocused-mode notifications while the app has focus", () => {
    vi.mocked(document.hasFocus).mockReturnValue(true);
    const { result } = renderNotifications();

    act(() => result.current.handleRealtimeEvent(workEvent()));
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));

    expect(notificationRecords).toHaveLength(0);
  });

  it("ignores the initial zero-width progress message and notifies on its final update", () => {
    const { result } = renderNotifications(TurnNotificationModes.always);
    const message = (status: string, content: string): IMServerEvent => ({
      type: "message.created",
      room_id: "room-1",
      message: {
        id: "same-message",
        sender_id: "user-worker",
        content,
        metadata: {
          codex: { request_id: "message-1", delivery_kind: status === "succeeded" ? "final" : "activity" },
          csgclaw: {
            turn_progress: {
              id: "turn-1",
              revision: status === "running" ? 1 : 2,
              status,
              started_at: "2026-10-15T00:00:00Z",
              items: [],
            },
          },
        },
      },
    });
    act(() => result.current.handleRealtimeEvent(message("running", "\u200b")));
    act(() => result.current.handleRealtimeEvent(message("waiting", "Still working")));
    expect(notificationRecords).toHaveLength(0);
    act(() => result.current.handleRealtimeEvent(message("succeeded", "Current final reply")));
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", state: "idle" })));
    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]?.body).toBe("Research room: Current final reply");
  });

  it.each(["thought", "activity", "task_reported", "turn_stopped", undefined])(
    "does not label %s messages as completed turns",
    (deliveryKind) => {
      const { result } = renderNotifications(TurnNotificationModes.always);
      act(() =>
        result.current.handleRealtimeEvent({
          type: "message.created",
          room_id: "room-1",
          message: {
            id: "nonfinal",
            sender_id: "user-worker",
            content: "Agent-authored notification or intermediate output",
            metadata: { codex: { delivery_kind: deliveryKind } },
          },
        }),
      );
      expect(notificationRecords).toHaveLength(0);
    },
  );

  it.each(["failed", "cancelled"])("does not notify completion after a %s turn is released", (status) => {
    const { result } = renderNotifications(TurnNotificationModes.always);
    act(() =>
      result.current.handleRealtimeEvent({
        type: "message.created",
        room_id: "room-1",
        message: {
          id: "nonfinal",
          sender_id: "user-worker",
          content: "\u200b",
          metadata: {
            codex: { request_id: "message-1", delivery_kind: "activity" },
            csgclaw: {
              turn_progress: { id: "turn-1", revision: 2, status, started_at: "2026-10-15T00:00:00Z", items: [] },
            },
          },
        },
      }),
    );
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", state: "idle" })));
    expect(notificationRecords).toHaveLength(0);
  });

  it("requests notification permission from a user action", async () => {
    FakeNotification.permission = "default";
    FakeNotification.requestPermission.mockImplementation(async () => {
      FakeNotification.permission = "granted";
      return "granted";
    });
    const { result } = renderNotifications();

    await act(async () => {
      await expect(result.current.requestPermission()).resolves.toBe("granted");
    });

    expect(result.current.permission).toBe("granted");
  });

  it("omits the duplicated agent name from a direct-chat notification body", () => {
    const { result } = renderNotifications(TurnNotificationModes.always, {
      isDirect: true,
      roomTitle: "Research Agent",
    });

    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));

    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]).toMatchObject({
      body: "The report is ready.",
      title: "Research Agent finished replying",
    });
  });

  it("keeps the room name in a group-chat notification body", () => {
    const { result } = renderNotifications(TurnNotificationModes.always, {
      isDirect: false,
      roomTitle: "11",
    });

    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));

    expect(notificationRecords).toHaveLength(1);
    expect(notificationRecords[0]).toMatchObject({
      body: "11: The report is ready.",
      title: "Research Agent finished replying",
    });
  });
});
