import { useEffect, useState } from "react";
import {
  agentMemoryChangedEvent,
  fetchAgentMemoryDocument,
  fetchAgentMemoryMarks,
  forgetAgentMemory,
  updateAgentMemory,
  type AgentMemoryMarks,
  type AgentMemoryChange,
} from "@/api/agents";
import { diagnosticFinal, diagnosticReference } from "@/models/diagnostics";
import type { IMMessage } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";
import {
  isDirectConversation,
  resolveAgentForUser,
  resolveConversationUser,
  type IMConversation,
  type UsersById,
} from "@/models/conversations";

export function conversationMemoryAgent(
  conversation: IMConversation | null | undefined,
  agents: readonly AgentLike[],
  userID: string | undefined,
  usersById: UsersById,
): string {
  if (!conversation || !userID || !isDirectConversation(conversation) || conversation.cloud_agent) return "";
  const agent = resolveAgentForUser(agents, resolveConversationUser(conversation, userID, usersById));
  return agent?.memory_supported ? String(agent.id || "") : "";
}

export function useConversationMemory(agentID: string, roomID: string) {
  const [snapshot, setSnapshot] = useState<{ agentID: string; marks: AgentMemoryMarks } | null>(null);
  const marks = snapshot?.agentID === agentID ? snapshot.marks : null;
  useEffect(() => {
    if (!agentID) return;
    let disposed = false;
    let inFlight = false;
    const apply = (marks: AgentMemoryMarks) => {
      if (disposed) return;
      setSnapshot((current) =>
        current?.agentID === agentID && marks.available && (current.marks.revision ?? 0) > (marks.revision ?? 0)
          ? current
          : { agentID, marks },
      );
    };
    const refresh = async () => {
      if (disposed || inFlight || document.hidden) return;
      inFlight = true;
      try {
        const marks = await fetchAgentMemoryMarks(agentID);
        apply(marks);
      } catch {
        if (!disposed) setSnapshot(null);
      } finally {
        inFlight = false;
      }
    };
    const onChange = (event: Event) => {
      const change = (event as CustomEvent<AgentMemoryChange>).detail;
      if (change.agentID === agentID) {
        apply({
          available: change.document.available,
          enabled: change.document.enabled,
          revision: change.document.revision,
          sources: change.document.entries.flatMap((entry) => (entry.source ? [entry.source] : [])),
        });
      }
    };
    void refresh();
    const timer = marks?.enabled ? window.setInterval(() => void refresh(), 2000) : undefined;
    window.addEventListener(agentMemoryChangedEvent, onChange);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      disposed = true;
      if (timer !== undefined) window.clearInterval(timer);
      window.removeEventListener(agentMemoryChangedEvent, onChange);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [agentID, marks?.enabled]);
  const messages = new Set(
    (marks?.sources || []).filter((source) => source.room_id === roomID).map((s) => s.message_id),
  );

  return {
    enabled: Boolean(marks?.enabled),
    available: Boolean(marks?.available),
    messages,
    sourceFor(message: IMMessage): string {
      if (message.id && messages.has(message.id)) return message.id;
      const reference = diagnosticReference(message.metadata);
      if (!reference || reference.room !== roomID || !messages.has(reference.source)) return "";
      if (!diagnosticFinal(message.metadata)) return "";
      return reference.source;
    },
    async enable() {
      const document = await fetchAgentMemoryDocument(agentID);
      if (document.available && !document.enabled) {
        await updateAgentMemory(agentID, document.revision, { enabled: true });
      }
    },
    async forget(messageID: string) {
      await forgetAgentMemory(agentID, roomID, messageID);
    },
  };
}
