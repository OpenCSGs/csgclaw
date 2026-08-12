import {
  DEFAULT_TURN_NOTIFICATION_MODE,
  isCompletedAgentTurnEvent,
  normalizeTurnNotificationMode,
  shouldShowTurnNotification,
  TurnNotificationModes,
} from "@/models/turnNotifications";

describe("turn notifications", () => {
  it("normalizes stored modes and defaults to unfocused notifications", () => {
    expect(normalizeTurnNotificationMode(TurnNotificationModes.off)).toBe(TurnNotificationModes.off);
    expect(normalizeTurnNotificationMode(TurnNotificationModes.always)).toBe(TurnNotificationModes.always);
    expect(normalizeTurnNotificationMode(TurnNotificationModes.whenUnfocused)).toBe(
      TurnNotificationModes.whenUnfocused,
    );
    expect(normalizeTurnNotificationMode("invalid")).toBe(DEFAULT_TURN_NOTIFICATION_MODE);
    expect(normalizeTurnNotificationMode(null)).toBe(DEFAULT_TURN_NOTIFICATION_MODE);
  });

  it("honors off, always, and unfocused modes", () => {
    const focused = { documentVisible: true, windowFocused: true };
    const blurred = { documentVisible: true, windowFocused: false };
    const hidden = { documentVisible: false, windowFocused: false };

    expect(shouldShowTurnNotification(TurnNotificationModes.off, hidden)).toBe(false);
    expect(shouldShowTurnNotification(TurnNotificationModes.always, focused)).toBe(true);
    expect(shouldShowTurnNotification(TurnNotificationModes.whenUnfocused, focused)).toBe(false);
    expect(shouldShowTurnNotification(TurnNotificationModes.whenUnfocused, blurred)).toBe(true);
    expect(shouldShowTurnNotification(TurnNotificationModes.whenUnfocused, hidden)).toBe(true);
  });

  it("only treats a released idle agent lease as a completed turn", () => {
    const completed = {
      type: "participant.work.updated",
      work: {
        expires_at: "2026-08-11T12:00:15Z",
        kind: "agent_turn" as const,
        lease_id: "lease-1",
        participant_id: "pt-worker",
        reason: "released" as const,
        registry_epoch: "epoch-1",
        request_id: "message-1",
        revision: 2,
        room_id: "room-1",
        state: "idle" as const,
        user_id: "user-worker",
      },
    };

    expect(isCompletedAgentTurnEvent(completed)).toBe(true);
    expect(isCompletedAgentTurnEvent({ ...completed, work: { ...completed.work, state: "working" } })).toBe(false);
    expect(isCompletedAgentTurnEvent({ ...completed, work: { ...completed.work, reason: "expired" } })).toBe(false);
  });
});
