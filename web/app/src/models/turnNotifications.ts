import type { IMServerEvent } from "@/models/conversations";

export const TurnNotificationModes = {
  off: "off",
  always: "always",
  whenUnfocused: "when_unfocused",
} as const;

export type TurnNotificationMode = (typeof TurnNotificationModes)[keyof typeof TurnNotificationModes];

export const DEFAULT_TURN_NOTIFICATION_MODE: TurnNotificationMode = TurnNotificationModes.whenUnfocused;

export function normalizeTurnNotificationMode(value: unknown): TurnNotificationMode {
  return Object.values(TurnNotificationModes).includes(value as TurnNotificationMode)
    ? (value as TurnNotificationMode)
    : DEFAULT_TURN_NOTIFICATION_MODE;
}

export function shouldShowTurnNotification(
  mode: TurnNotificationMode,
  appState: { documentVisible: boolean; windowFocused: boolean },
): boolean {
  if (mode === TurnNotificationModes.off) {
    return false;
  }
  if (mode === TurnNotificationModes.always) {
    return true;
  }
  return !appState.documentVisible || !appState.windowFocused;
}

export function isCompletedAgentTurnEvent(event: IMServerEvent | null | undefined): boolean {
  return Boolean(
    event?.type === "participant.work.updated" &&
    event.work?.kind === "agent_turn" &&
    event.work.state === "idle" &&
    event.work.reason === "released",
  );
}
