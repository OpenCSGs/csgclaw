import { useRef, useState } from "react";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import type { IMConversation } from "@/models/conversations";
import { effectiveConversationProfile } from "@/models/taskConfiguration";
import { normalizeAuthProviderName, providerNeedsAuth, resolveAgentChannelUserID } from "@/models/agents";
import { modelProviderConfigUsesOpenCSG } from "@/models/modelProviders";
import { taskConversationTitle } from "@/models/taskConversations";
import { sendMessageRequest } from "@/api/im";
import { errorMessage } from "@/api/client";
import { localizeAPIError } from "@/shared/i18n";
import { createUUID } from "@/shared/lib/uuid";

export function useScheduledTaskConversation() {
  const controller = useWorkspaceControllerContext();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const inFlight = useRef(false);
  // A failed message send reuses its room and request IDs; retries cannot create duplicate conversations.
  const pending = useRef<{ text: string; requestID: string; messageID: string; room: IMConversation | null } | null>(
    null,
  );
  async function send(text: string): Promise<boolean> {
    if (!controller.ready || !text.trim() || inFlight.current) return false;
    const { t, taskEntry, sidebarProps: sidebar } = controller;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const manager = controller.taskViewProps.agents.find((agent) => agent.role === "manager");
      if (!manager) throw new Error(t("taskEntryManagerMissing"));
      const profile = effectiveConversationProfile(manager, undefined, controller.modelProviders);
      if (profile?.profile_complete === false || !profile?.model_id) throw new Error(t("taskEntryModelMissing"));
      if (modelProviderConfigUsesOpenCSG(profile) && !sidebar.onRequireOpenCSGAuthentication?.()) return false;
      const provider = normalizeAuthProviderName(profile?.provider);
      if (providerNeedsAuth(provider) && controller.agentViewProps.authStatuses?.[provider]?.authenticated === false)
        throw new Error(t("authRequired"));
      if (pending.current?.text !== text)
        pending.current = { text, requestID: createUUID(), messageID: createUUID(), room: null };
      const attempt = pending.current;
      if (!attempt.room)
        attempt.room = await taskEntry.create({
          title: taskConversationTitle(text, t("scheduledTaskTextTitle")),
          member_ids: [resolveAgentChannelUserID(manager) || manager.id || ""],
          type: "free",
          is_direct: true,
          client_request_id: attempt.requestID,
          locale: sidebar.locale,
        });
      const message = await sendMessageRequest({
        room_id: attempt.room.id,
        sender_id: sidebar.currentUserID,
        content: t("scheduledTaskTextMessage", {
          text: text.trim(),
        }),
        client_message_id: attempt.messageID,
        locale: sidebar.locale,
      });
      taskEntry.onMessage(attempt.room.id, message);
      taskEntry.open(attempt.room);
      pending.current = null;
      return true;
    } catch (cause) {
      setError(localizeAPIError(cause, t, errorMessage(cause, t("sendFailed"))));
      return false;
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  return { send, busy, error };
}
