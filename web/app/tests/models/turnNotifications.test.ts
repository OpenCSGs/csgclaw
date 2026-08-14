import type { TranslateFn } from "@/models/conversations";
import {
  DEFAULT_TURN_NOTIFICATION_MODE,
  formatTurnNotificationBody,
  isCompletedAgentTurnEvent,
  normalizeTurnNotificationMode,
  resolveTurnNotificationRoomLabel,
  shouldShowTurnNotification,
  TurnNotificationModes,
} from "@/models/turnNotifications";

const t: TranslateFn = (key, params = {}) => {
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

  it("omits a room label that repeats the agent name or belongs to a direct chat", () => {
    expect(resolveTurnNotificationRoomLabel("codex03", "codex03", { is_direct: true })).toBe("");
    expect(resolveTurnNotificationRoomLabel("codex03", " Codex03 ", { is_direct: false })).toBe("");
    expect(resolveTurnNotificationRoomLabel("codex03", "standup", { is_direct: true })).toBe("");
    expect(resolveTurnNotificationRoomLabel("dev", "11", { is_direct: false })).toBe("11");
  });

  it("formats notification bodies without repeating the agent name in direct chats", () => {
    expect(
      formatTurnNotificationBody(t, {
        agentName: "codex03",
        preview: "Runtime error: Codex stopped responding after 5m0s.",
        room: { is_direct: true },
        roomTitle: "codex03",
      }),
    ).toBe("Runtime error: Codex stopped responding after 5m0s.");

    expect(
      formatTurnNotificationBody(t, {
        agentName: "dev",
        preview: "already asked a question",
        room: { is_direct: false },
        roomTitle: "11",
      }),
    ).toBe("11: already asked a question");

    expect(
      formatTurnNotificationBody(t, {
        agentName: "codex03",
        preview: "",
        room: { is_direct: true },
        roomTitle: "codex03",
      }),
    ).toBe("The agent turn has finished.");
  });
});
