import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane/types";
import { isEventMessage, isToolCallMessage, localIdentitiesMatch, type IMMessage } from "./conversations";
import { isAutonomousCoordinationNotice, isAutonomousCoordinationTool } from "./agentActivity";
import { parseTurnProgress, progressActive, terminalTool, type TurnProgress } from "./turnProgress";
import { workingParticipantForTurnMessage } from "./turnWorkingParticipant";

export type CollaborationWork = {
  id: string;
  senderID: string;
  message?: IMMessage;
  note?: string;
  progress?: TurnProgress;
  participant?: ConversationWorkingParticipant;
};

// Live leases own controls. Persisted progress owns presentation, including the
// interval before a lease arrives and after a completed lease is released.
export function collaborationWork(
  roomID: string,
  messages: readonly IMMessage[],
  participants: readonly ConversationWorkingParticipant[],
): CollaborationWork[] {
  const represented = new Set<ConversationWorkingParticipant>();
  const latest = new Map<string, CollaborationWork>();
  const active: CollaborationWork[] = [];
  for (const message of messages) {
    const progress = parseTurnProgress(message);
    if (!progress) continue;
    const participant = workingParticipantForTurnMessage(message, roomID, participants);
    if (participant) represented.add(participant);
    const work = { id: progress.id, senderID: message.sender_id || "", message, progress, participant };
    if (progressActive(progress)) active.push(work);
    const previous = latest.get(work.senderID);
    if (!previous || Date.parse(progress.started_at) >= Date.parse(previous.progress!.started_at)) {
      latest.set(work.senderID, work);
    }
  }
  for (const participant of participants) {
    if (!represented.has(participant)) {
      active.push({ id: participant.leaseID || participant.id, senderID: participant.id, participant });
    }
  }
  // Keep the latest failure visible, but retire it once that member starts again.
  const problems = [...latest.values()].filter(
    (work) =>
      (work.progress?.error || ["failed", "interrupted"].includes(work.progress?.status || "")) &&
      !active.some((item) => localIdentitiesMatch(item.senderID, work.senderID)),
  );
  const notes = [...messages]
    .reverse()
    .filter(
      (message) =>
        !parseTurnProgress(message) &&
        !isToolCallMessage(message) &&
        !isEventMessage(message) &&
        !isAutonomousCoordinationNotice(message) &&
        message.content.trim(),
    );
  return [
    ...active.map((work) => ({
      ...work,
      note: notes.find(
        (note) =>
          localIdentitiesMatch(note.sender_id, work.senderID) &&
          Date.parse(note.created_at || "") >=
            Date.parse(work.progress?.started_at || work.participant?.activityAfter || ""),
      )?.content,
    })),
    ...problems,
  ];
}

export function collaborationWorkSummary(work: CollaborationWork): { label: string; detail: string } {
  const { progress, participant } = work;
  if (participant?.stopError) return { label: "collaborationWorkStopFailed", detail: participant.stopError };
  if (participant?.stopping || participant?.stopSending) return { label: "conversationWorkingStopping", detail: "" };
  if (participant?.contextUsage?.compacting) return { label: "contextCompacting", detail: "" };
  if (progress && !progressActive(progress))
    return { label: `progressStatus_${progress.status}`, detail: progress.error || "" };
  if (progress?.status === "waiting") return { label: "progressStatus_waiting", detail: "" };

  const items = progress?.items ?? [];
  const commentary =
    [...items].reverse().find((item) => item.kind === "commentary" && item.text?.trim())?.text || work.note || "";
  const tool = [...items].reverse().find((item) => item.tool && !terminalTool(item.tool.status))?.tool;
  if (tool) {
    if (isAutonomousCoordinationTool(tool.name)) return { label: "conversationWorkingPreparingReply", detail: "" };
    if (/messages_(read|ack)/.test(tool.name) || /message read\s+.*--unread/.test(tool.command || ""))
      return { label: "collaborationWorkCheckingMessages", detail: commentary };
    const action = tool.actions.find((value) => ["search", "web", "read", "list", "edit", "execute"].includes(value));
    const labels: Record<string, string> = {
      search: "conversationWorkingSearching",
      web: "conversationWorkingSearching",
      read: "conversationWorkingReading",
      list: "conversationWorkingReading",
      edit: "conversationWorkingEditing",
      execute: "conversationWorkingRunning",
    };
    return {
      label: labels[action || ""] || "conversationWorkingUsingTool",
      detail: toolSubject(tool.input) || commentary || tool.name,
    };
  }
  if (progress?.private_output && !items.length && !commentary)
    return { label: "collaborationWorkCheckingMessages", detail: "" };
  const stage = participant?.workStage;
  if (stage === "generating_reply") return { label: "conversationWorkingGeneratingReply", detail: commentary };
  if (stage === "thinking" || items.at(-1)?.kind === "reasoning")
    return { label: "conversationWorkingThinking", detail: commentary };
  if (stage === "running_tool")
    return { label: "conversationWorkingUsingTool", detail: commentary || participant?.activity?.toolName || "" };
  return {
    label: items.some((item) => item.kind === "tool")
      ? "collaborationWorkReviewing"
      : "conversationWorkingPreparingReply",
    detail: commentary,
  };
}

// Only display structured subjects. Never dump shell scripts, tool output or
// private reasoning into the compact status surface.
function toolSubject(input?: string): string {
  if (!input) return "";
  try {
    const value: unknown = JSON.parse(input);
    if (!value || typeof value !== "object" || Array.isArray(value)) return "";
    const fields = value as Record<string, unknown>;
    for (const key of ["query", "pattern", "url", "file_path", "path"]) {
      const subject = fields[key];
      if (typeof subject === "string" && subject.trim()) return subject.replace(/\s+/g, " ").trim().slice(0, 180);
    }
  } catch {
    /* Truncated runtime inputs are common. Use the commentary instead. */
  }
  return "";
}
