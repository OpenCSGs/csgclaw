import { isAutonomousCoordinationNotice, parseAgentActivity } from "./agentActivity";
import { messageResources } from "./conversationResources";
import { isDirectConversation, localIdentitiesMatch, type IMConversation, type IMMessage } from "./conversations";
import { parseTurnProgress, progressActive, type TurnProgress } from "./turnProgress";
import { AgentActivityMsgTypes } from "@/shared/constants/messages";

const silentReply = "<csgclaw-silent/>";

export function usesCollaborationTranscript(conversation: IMConversation): boolean {
  return !isDirectConversation(conversation) && !conversation.cloud_agent;
}

// Keep the stored messages intact. A turn's placeholder is created before its
// public answer, so only the room presentation uses the completion timestamp.
export function collaborationTranscript(messages: readonly IMMessage[]) {
  const activities: IMMessage[] = [];
  const publicMessages: IMMessage[] = [];
  for (const message of messages) {
    if (isAutonomousCoordinationNotice(message)) continue;
    const progress = parseTurnProgress(message);
    if (progress) {
      activities.push(message);
      const recovered = recoverHistoricalReply(message, progress, messages);
      const answer = recovered || message.content.replace(/\u200b/g, "");
      const content = answer.trim() === silentReply ? "" : answer;
      if (
        (!progress.private_output || recovered || (progress.status === "succeeded" && message.attachments?.length)) &&
        !progressActive(progress) &&
        (content.trim() || message.attachments?.length || messageResources(message.metadata).length)
      ) {
        publicMessages.push({
          ...message,
          content,
          created_at: validTime(progress.ended_at) ? progress.ended_at : message.created_at,
        });
      } else if (progress.model_request && progress.model_request.state !== "recovered") {
        // Surface failures and retries without exposing an unfinished or
        // private Agent reply in the collaboration transcript.
        publicMessages.push({ ...message, content: "\u200b", attachments: [] });
      }
      continue;
    }
    const activity = parseAgentActivity(message);
    // Questions, approvals and notices stay in the conversation. They must not
    // disappear behind the execution disclosure while waiting for the human.
    if (activity?.content.msgtype === AgentActivityMsgTypes.tool) activities.push(message);
    else publicMessages.push(message);
  }
  publicMessages.sort((left, right) => time(left.created_at) - time(right.created_at));
  return { messages: publicMessages, activities };
}

// Older versions stored some final replies only in turn progress. Restore
// their display without replaying room events.
function recoverHistoricalReply(message: IMMessage, progress: TurnProgress, messages: readonly IMMessage[]): string {
  if (!progress.private_output || progress.status !== "succeeded") return "";
  const answer = progress.final_text?.trim();
  if (!answer || answer === silentReply) return "";

  const startedAt = time(progress.started_at);
  const endedAt = time(progress.ended_at);
  const thread = (item: IMMessage) => (item.relates_to?.rel_type === "m.thread" ? item.relates_to.event_id || "" : "");
  const alreadySent =
    startedAt &&
    endedAt &&
    messages.some((item) => {
      if (
        item.id === message.id ||
        !localIdentitiesMatch(item.sender_id || "", message.sender_id || "") ||
        thread(item) !== thread(message) ||
        item.content.trim() !== answer
      )
        return false;
      const prior = parseTurnProgress(item);
      if (prior && (prior.private_output || progressActive(prior))) return false;
      if (parseAgentActivity(item)?.content.msgtype === AgentActivityMsgTypes.tool) return false;
      const sentAt = time(prior?.ended_at) || time(item.created_at);
      return sentAt >= startedAt && sentAt <= endedAt;
    });
  return alreadySent ? "" : answer;
}

function validTime(value?: string): boolean {
  return Boolean(value && Number.isFinite(Date.parse(value)));
}

function time(value?: string): number {
  return validTime(value) ? Date.parse(value!) : 0;
}
