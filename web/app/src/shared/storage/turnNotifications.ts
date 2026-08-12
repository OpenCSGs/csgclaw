import {
  DEFAULT_TURN_NOTIFICATION_MODE,
  normalizeTurnNotificationMode,
  type TurnNotificationMode,
} from "@/models/turnNotifications";
import { TURN_NOTIFICATION_MODE_STORAGE_KEY } from "./keys";

export function readStoredTurnNotificationMode(): TurnNotificationMode {
  if (typeof window === "undefined") {
    return DEFAULT_TURN_NOTIFICATION_MODE;
  }
  try {
    return normalizeTurnNotificationMode(window.localStorage.getItem(TURN_NOTIFICATION_MODE_STORAGE_KEY));
  } catch {
    return DEFAULT_TURN_NOTIFICATION_MODE;
  }
}

export function writeStoredTurnNotificationMode(mode: TurnNotificationMode): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(TURN_NOTIFICATION_MODE_STORAGE_KEY, mode);
  } catch {
    // Local storage can be unavailable in restricted browser contexts.
  }
}
