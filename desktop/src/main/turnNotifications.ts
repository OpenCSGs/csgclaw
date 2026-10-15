import { Notification } from "electron";
import type { DesktopTurnNotification } from "../shared/desktopBridge.types";
import { logDesktopError } from "./desktopLogger";

export class TurnNotifications {
  // macOS removes delivered notifications when their native objects are collected.
  private readonly active = new Map<string, Notification>();

  constructor(
    private readonly onClick: (roomID: string) => void,
    private readonly create = (input: DesktopTurnNotification): Notification =>
      new Notification(input),
    private readonly supported = () => Notification.isSupported(),
  ) {}

  show(input: unknown): Promise<void> {
    const parsed = parseTurnNotification(input);
    if (!this.supported())
      throw new Error("System notifications are unavailable.");
    if (this.active.has(parsed.id)) return Promise.resolve();
    if (this.active.size >= 128) {
      const oldest = this.active.entries().next().value;
      if (oldest) {
        this.active.delete(oldest[0]);
        oldest[1].close();
      }
    }
    const notification = this.create(parsed);
    this.active.set(parsed.id, notification);
    return new Promise((resolve, reject) => {
      const remove = () => this.active.delete(parsed.id);
      notification.once("show", () => resolve());
      notification.once("failed", (_event, detail) => {
        remove();
        const error = new Error(`System notification failed: ${detail}`);
        logDesktopError("turn-notification-failed", error);
        reject(error);
      });
      notification.once("close", remove);
      notification.once("click", () => {
        remove();
        notification.close();
        this.onClick(parsed.roomID);
      });
      try {
        notification.show();
      } catch (error) {
        remove();
        reject(error);
      }
    });
  }

  dispose(): void {
    for (const notification of this.active.values()) notification.close();
    this.active.clear();
  }
}

export function parseTurnNotification(input: unknown): DesktopTurnNotification {
  if (!input || typeof input !== "object" || Array.isArray(input)) {
    throw new Error("Turn notification is invalid.");
  }
  const value = input as Record<string, unknown>;
  for (const [key, limit] of [
    ["id", 64],
    ["title", 512],
    ["body", 2048],
    ["roomID", 512],
  ] as const) {
    if (typeof value[key] !== "string" || value[key].length > limit) {
      throw new Error(`Turn notification ${key} is invalid.`);
    }
  }
  if (typeof value.silent !== "boolean")
    throw new Error("Turn notification sound setting is invalid.");
  if (!value.id || !value.title)
    throw new Error("Turn notification requires an id and title.");
  return {
    id: value.id,
    title: value.title,
    body: value.body,
    roomID: value.roomID,
    silent: value.silent,
  } as DesktopTurnNotification;
}
