import { act, renderHook } from "@testing-library/react";
import { buildUsersById, type IMServerEvent, type TranslateFn } from "@/models/conversations";
import { TurnNotificationModes, type TurnNotificationMode } from "@/models/turnNotifications";
import { useAgentTurnNotifications } from "@/hooks/workspace/useAgentTurnNotifications";
import type { DesktopBridge } from "@/shared/platform/desktopBridge";

type NotificationRecord = {
  body: string;
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
  private readonly closeMock = vi.fn();

  constructor(title: string, options: NotificationOptions = {}) {
    notificationRecords.push({
      body: options.body ?? "",
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
  options: { isDirect?: boolean; roomTitle?: string } = {},
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
    setUpdateChannel: vi.fn(),
    setThemeSource: vi.fn(),
    onUpdateStatus: vi.fn(),
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
      tag: "csgclaw-turn:message-1",
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

  it("shows and focuses the desktop window before selecting the room", async () => {
    let finishShowingWindow: (() => void) | undefined;
    const showWindow = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finishShowingWindow = resolve;
        }),
    );
    window.csgclawDesktop = desktopBridge(showWindow);
    const { result, onSelectConversation } = renderNotifications();

    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));

    let click: Promise<unknown> | undefined;
    act(() => {
      click = notificationRecords[0]?.notification.onclick?.call(
        notificationRecords[0].notification as unknown as Notification,
        new Event("click"),
      ) as Promise<unknown> | undefined;
    });

    expect(showWindow).toHaveBeenCalledTimes(1);
    expect(window.focus).not.toHaveBeenCalled();
    expect(onSelectConversation).not.toHaveBeenCalled();

    finishShowingWindow?.();
    await act(async () => {
      await click;
    });

    expect(onSelectConversation).toHaveBeenCalledWith("room-1");
  });

  it("uses a visible reply as a fallback for agents without work lease events", () => {
    const { result } = renderNotifications(TurnNotificationModes.always);

    act(() =>
      result.current.handleRealtimeEvent({
        message: {
          content: "Fallback reply",
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

  it("keeps the visible final reply fallback after observing work and deduplicates the release", () => {
    const { result } = renderNotifications(TurnNotificationModes.always);

    act(() => result.current.handleRealtimeEvent(workEvent()));
    act(() =>
      result.current.handleRealtimeEvent({
        message: {
          content: "Fallback after reconnect",
          id: "reply-3",
          metadata: {
            openclaw: { delivery_kind: "final", request_id: "message-1" },
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
      tag: "csgclaw-turn:message-1",
    });
  });

  it("suppresses unfocused-mode notifications while the app has focus", () => {
    vi.mocked(document.hasFocus).mockReturnValue(true);
    const { result } = renderNotifications();

    act(() => result.current.handleRealtimeEvent(workEvent()));
    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));

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
