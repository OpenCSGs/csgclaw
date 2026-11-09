import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentLike } from "@/models/agents";
import {
  isEventMessage,
  isToolCallMessage,
  localIdentitiesMatch,
  resolveAgentForUser,
  resolveUserByLocalIdentity,
} from "@/models/conversations";
import type { IMConversation, IMMessage, IMServerEvent, TranslateFn, UsersById } from "@/models/conversations";
import {
  buildTurnNotificationTag,
  formatTurnNotificationBody,
  formatTurnNotificationPreview,
  isCompletedAgentTurnEvent,
  isCompletedAgentTurnMessage,
  shouldShowTurnNotification,
  TurnNotificationModes,
} from "@/models/turnNotifications";
import type { TurnNotificationMode } from "@/models/turnNotifications";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";

export type SystemNotificationPermission = NotificationPermission | "unsupported";

type UseAgentTurnNotificationsArgs = {
  agents: readonly AgentLike[];
  currentUserID: string;
  mode: TurnNotificationMode;
  soundEnabled?: boolean;
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

export function readSystemNotificationPermission(): SystemNotificationPermission {
  if (typeof window === "undefined" || typeof window.Notification !== "function") {
    return "unsupported";
  }
  return window.Notification.permission;
}

export function useAgentTurnNotifications(args: UseAgentTurnNotificationsArgs): AgentTurnNotificationController {
  const refs = useRef(args);
  const notifiedEventKeysRef = useRef(new Set<string>());
  const turnMessagesRef = useRef(new Map<string, IMMessage>());
  const [permission, setPermission] = useState<SystemNotificationPermission>(readSystemNotificationPermission);

  useEffect(() => {
    refs.current = args;
  }, [args]);

  useEffect(() => {
    return getDesktopBridge()?.onTurnNotificationClick((roomID) => {
      if (roomID) refs.current.onSelectConversation(roomID);
    });
  }, []);

  const refreshPermission = useCallback(() => {
    setPermission(readSystemNotificationPermission());
  }, []);

  useEffect(() => {
    refreshPermission();
    window.addEventListener("focus", refreshPermission);
    document.addEventListener("visibilitychange", refreshPermission);
    return () => {
      window.removeEventListener("focus", refreshPermission);
      document.removeEventListener("visibilitychange", refreshPermission);
    };
  }, [refreshPermission]);

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

  const showAgentTurnNotification = useCallback(
    (
      current: UseAgentTurnNotificationsArgs,
      input: {
        agent: AgentLike;
        eventKey: string;
        message?: IMMessage;
        participantIdentities: string[];
        roomID: string;
      },
    ): void => {
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
      const preview = formatTurnNotificationPreview(input.message?.content);
      const roomTitle = String(room?.title || "").trim();
      const body = formatTurnNotificationBody(current.t, {
        agentName,
        preview,
        room,
        roomTitle,
      });

      const eventKey = input.eventKey;
      notifiedEventKeysRef.current.add(eventKey);
      const onError = () => {
        notifiedEventKeysRef.current.delete(eventKey);
        refreshPermission();
      };
      try {
        const desktopBridge = getDesktopBridge();
        if (desktopBridge) {
          void desktopBridge
            .showTurnNotification({
              title: current.t("turnNotificationTitle", { agent: agentName }),
              body,
              id: buildTurnNotificationTag(eventKey),
              roomID,
              silent: current.soundEnabled === false,
            })
            .catch(onError);
          return;
        }
        const notification = new window.Notification(current.t("turnNotificationTitle", { agent: agentName }), {
          body,
          icon: "favicon.ico",
          tag: buildTurnNotificationTag(input.eventKey),
          silent: current.soundEnabled === false,
        });
        notification.onerror = onError;
        notification.onclick = () => {
          notification.close();
          window.focus();
          if (roomID) {
            refs.current.onSelectConversation(roomID);
          }
        };
      } catch {
        onError();
      }
    },
    [refreshPermission],
  );

  const handleRealtimeEvent = useCallback(
    (event: IMServerEvent) => {
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
        const roomID = event.work.room_id || String(event.room_id || "");
        const requestKey = notificationEventKeyForWork(event.work);
        const eventKey = scopedEventKey(agent, roomID, requestKey);
        const message =
          turnMessagesRef.current.get(eventKey) ??
          latestAgentTurnMessage(
            current.rooms.find((room) => room.id === roomID),
            [event.work.user_id, event.work.participant_id],
            requestKey,
          );
        if (message && !isCompletedAgentTurnMessage(message)) return;
        showAgentTurnNotification(current, {
          agent,
          eventKey,
          message,
          participantIdentities: [event.work.user_id, event.work.participant_id],
          roomID,
        });
        return;
      }

      if (event.type !== "message.created" || !event.message || isEventMessage(event.message)) {
        return;
      }
      if (isToolCallMessage(event.message)) {
        return;
      }
      const senderID = String(event.message.sender_id || "").trim();
      const agent = resolveEventAgent(current.agents, current.usersById, [senderID]);
      if (!agent) {
        return;
      }
      const roomID = String(event.room_id || event.room?.id || "");
      const eventKey = scopedEventKey(agent, roomID, notificationEventKeyForMessage(event.message, senderID));
      turnMessagesRef.current.set(eventKey, event.message);
      if (turnMessagesRef.current.size > 512) {
        turnMessagesRef.current.delete(turnMessagesRef.current.keys().next().value!);
      }
      if (!isCompletedAgentTurnMessage(event.message)) return;
      showAgentTurnNotification(current, {
        agent,
        eventKey,
        message: event.message,
        participantIdentities: [senderID],
        roomID,
      });
    },
    [showAgentTurnNotification],
  );

  return { handleRealtimeEvent, permission, requestPermission };
}

function scopedEventKey(agent: AgentLike, roomID: string, key: string): string {
  return JSON.stringify([roomID, agent.id, key]);
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
  const codex = asRecord(metadata?.codex);
  const csgclaw = asRecord(metadata?.csgclaw);
  return firstText(csgclaw?.request_id, codex?.request_id, codex?.requestId, codex?.source_message_id);
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

function latestAgentTurnMessage(
  room: IMConversation | null | undefined,
  participantIdentities: readonly string[],
  requestKey: string,
): IMMessage | undefined {
  return [...(room?.messages || [])].reverse().find((message) => {
    if (
      !participantIdentities.some((identity) => localIdentitiesMatch(message.sender_id, identity)) ||
      notificationEventKeyForMessage(message, message.sender_id || "") !== requestKey ||
      isEventMessage(message) ||
      isToolCallMessage(message)
    ) {
      return false;
    }
    return true;
  });
}
