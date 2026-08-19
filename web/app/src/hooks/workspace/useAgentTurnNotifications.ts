import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentLike } from "@/models/agents";
import {
  formatMessagePreviewText,
  isEventMessage,
  isToolCallMessage,
  localIdentitiesMatch,
  resolveAgentForUser,
  resolveUserByLocalIdentity,
} from "@/models/conversations";
import type { IMConversation, IMMessage, IMServerEvent, TranslateFn, UsersById } from "@/models/conversations";
import {
  formatTurnNotificationBody,
  isCompletedAgentTurnEvent,
  shouldShowTurnNotification,
  TurnNotificationModes,
} from "@/models/turnNotifications";
import type { TurnNotificationMode } from "@/models/turnNotifications";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";
import { localizeRuntimeError } from "@/shared/i18n";

export type SystemNotificationPermission = NotificationPermission | "unsupported";

type UseAgentTurnNotificationsArgs = {
  agents: readonly AgentLike[];
  currentUserID: string;
  mode: TurnNotificationMode;
  onSelectConversation: (roomID: string) => void;
  rooms: readonly IMConversation[];
  t: TranslateFn;
  usersById: UsersById;
};

type AgentTurnNotificationController = {
  handleRealtimeEvent: (event: IMServerEvent) => void;
  permission: SystemNotificationPermission;
  requestPermission: () => Promise<SystemNotificationPermission>;
};

const NOTIFICATION_PREVIEW_MAX_LENGTH = 160;

export function readSystemNotificationPermission(): SystemNotificationPermission {
  if (typeof window === "undefined" || typeof window.Notification !== "function") {
    return "unsupported";
  }
  return window.Notification.permission;
}

export function useAgentTurnNotifications(args: UseAgentTurnNotificationsArgs): AgentTurnNotificationController {
  const refs = useRef(args);
  const notifiedEventKeysRef = useRef(new Set<string>());
  const [permission, setPermission] = useState<SystemNotificationPermission>(readSystemNotificationPermission);

  useEffect(() => {
    refs.current = args;
  }, [args]);

  useEffect(() => {
    const refreshPermission = () => setPermission(readSystemNotificationPermission());
    window.addEventListener("focus", refreshPermission);
    document.addEventListener("visibilitychange", refreshPermission);
    return () => {
      window.removeEventListener("focus", refreshPermission);
      document.removeEventListener("visibilitychange", refreshPermission);
    };
  }, []);

  const requestPermission = useCallback(async (): Promise<SystemNotificationPermission> => {
    if (typeof window.Notification !== "function") {
      setPermission("unsupported");
      return "unsupported";
    }
    try {
      const next = await window.Notification.requestPermission();
      setPermission(next);
      return next;
    } catch {
      const next = readSystemNotificationPermission();
      setPermission(next);
      return next;
    }
  }, []);

  const handleRealtimeEvent = useCallback((event: IMServerEvent) => {
    const current = refs.current;

    if (event.type === "participant.work.updated" && event.work) {
      const agent = resolveEventAgent(current.agents, current.usersById, [
        event.work.user_id,
        event.work.participant_id,
      ]);
      if (!agent) {
        return;
      }
      if (!isCompletedAgentTurnEvent(event)) {
        return;
      }
      showAgentTurnNotification(current, {
        agent,
        eventKey: notificationEventKeyForWork(event.work),
        participantIdentities: [event.work.user_id, event.work.participant_id],
        roomID: event.work.room_id || String(event.room_id || ""),
      });
      return;
    }

    if (event.type !== "message.created" || !event.message || isEventMessage(event.message)) {
      return;
    }
    if (isToolCallMessage(event.message) || !formatMessagePreviewText(event.message.content)) {
      return;
    }
    const senderID = String(event.message.sender_id || "").trim();
    const agent = resolveEventAgent(current.agents, current.usersById, [senderID]);
    if (!agent) {
      return;
    }
    showAgentTurnNotification(current, {
      agent,
      eventKey: notificationEventKeyForMessage(event.message, senderID),
      message: event.message,
      participantIdentities: [senderID],
      roomID: String(event.room_id || event.room?.id || ""),
    });
  }, []);

  function showAgentTurnNotification(
    current: UseAgentTurnNotificationsArgs,
    input: {
      agent: AgentLike;
      eventKey: string;
      message?: IMMessage;
      participantIdentities: string[];
      roomID: string;
    },
  ): void {
    if (
      current.mode === TurnNotificationModes.off ||
      readSystemNotificationPermission() !== "granted" ||
      input.participantIdentities.some((id) => localIdentitiesMatch(id, current.currentUserID)) ||
      notifiedEventKeysRef.current.has(input.eventKey)
    ) {
      return;
    }
    if (
      !shouldShowTurnNotification(current.mode, {
        documentVisible: document.visibilityState === "visible",
        windowFocused: typeof document.hasFocus !== "function" || document.hasFocus(),
      })
    ) {
      return;
    }

    const roomID = input.roomID.trim();
    const room = current.rooms.find((candidate) => candidate.id === roomID);
    const agentName = String(input.agent.name || input.agent.user_name || input.agent.id || "Agent").trim();
    const message = input.message ?? latestVisibleAgentMessage(room, input.participantIdentities);
    const rawPreview = formatMessagePreviewText(message?.content);
    const preview = truncateNotificationPreview(
      isTrustedRuntimeError(message) ? localizeRuntimeError(rawPreview, current.t) : rawPreview,
    );
    const roomTitle = String(room?.title || "").trim();
    const body = formatTurnNotificationBody(current.t, {
      agentName,
      preview,
      room,
      roomTitle,
    });

    try {
      const notification = new window.Notification(current.t("turnNotificationTitle", { agent: agentName }), {
        body,
        icon: "favicon.ico",
        tag: `csgclaw-${input.eventKey}`,
      });
      notifiedEventKeysRef.current.add(input.eventKey);
      notification.onclick = async () => {
        notification.close();
        const desktopBridge = getDesktopBridge();
        if (desktopBridge) {
          try {
            await desktopBridge.showWindow();
          } catch {
            return;
          }
        } else {
          window.focus();
        }
        if (roomID) {
          refs.current.onSelectConversation(roomID);
        }
      };
    } catch {
      setPermission(readSystemNotificationPermission());
    }
  }

  return { handleRealtimeEvent, permission, requestPermission };
}

function isTrustedRuntimeError(message: IMMessage | null | undefined): boolean {
  const metadata = asRecord(message?.metadata);
  const csgclaw = asRecord(metadata?.csgclaw);
  return csgclaw?.runtime_error === true;
}

function resolveEventAgent(
  agents: readonly AgentLike[],
  usersById: UsersById,
  identities: readonly (string | null | undefined)[],
): AgentLike | null {
  const ids = identities.map((value) => String(value || "").trim()).filter(Boolean);
  const users = ids.map((id) => resolveUserByLocalIdentity(id, usersById)).filter((user) => user !== undefined);
  return resolveAgentForUser(agents, users[0] ?? null, [...users.slice(1), ...ids.map((id) => ({ id }))]);
}

function notificationEventKeyForWork(work: NonNullable<IMServerEvent["work"]>): string {
  const requestID = String(work.request_id || "").trim();
  return requestID ? `turn:${requestID}` : `work:${work.registry_epoch}:${work.lease_id}`;
}

function notificationEventKeyForMessage(message: IMMessage, senderID: string): string {
  const requestID = messageRequestID(message);
  return requestID ? `turn:${requestID}` : `message:${String(message.id || message.created_at || "")}:${senderID}`;
}

function messageRequestID(message: IMMessage): string {
  const metadata = asRecord(message.metadata);
  const openclaw = asRecord(metadata?.openclaw);
  const codex = asRecord(metadata?.codex);
  return firstText(
    openclaw?.request_id,
    openclaw?.requestId,
    openclaw?.source_message_id,
    codex?.request_id,
    codex?.requestId,
    codex?.source_message_id,
  );
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function firstText(...values: unknown[]): string {
  for (const value of values) {
    const text = String(value || "").trim();
    if (text) {
      return text;
    }
  }
  return "";
}

function latestVisibleAgentMessage(
  room: IMConversation | null | undefined,
  participantIdentities: readonly string[],
): IMMessage | undefined {
  return [...(room?.messages || [])].reverse().find((message) => {
    if (
      !participantIdentities.some((identity) => localIdentitiesMatch(message.sender_id, identity)) ||
      isEventMessage(message) ||
      isToolCallMessage(message)
    ) {
      return false;
    }
    return Boolean(formatMessagePreviewText(message.content));
  });
}

function truncateNotificationPreview(preview: string): string {
  if (preview.length <= NOTIFICATION_PREVIEW_MAX_LENGTH) {
    return preview;
  }
  return `${preview.slice(0, NOTIFICATION_PREVIEW_MAX_LENGTH - 1).trimEnd()}…`;
}
