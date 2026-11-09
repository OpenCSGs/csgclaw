import type { AgentLike } from "@/models/agents";
import { emptyCompletedProgress, parseTurnProgress } from "@/models/turnProgress";
import { parseAgentActivity } from "@/models/agentActivity";
import {
  formatMessagePreviewText,
  isEventMessage,
  resolveAgentForUser,
  resolveUserByLocalIdentity,
  type IMMessage,
  type IMUser,
  type UsersById,
} from "@/models/conversations";

export type MessageTimelineItem = {
  id: string;
  label: string;
  author: string;
  createdAt?: string;
  preview: string;
};

export function resolveMessageAgent(agents: readonly AgentLike[], user: IMUser, senderID: string | null | undefined) {
  const senderIdentity = String(senderID || "").trim();
  const alternateUser =
    senderIdentity && senderIdentity !== user.id ? { ...user, id: senderIdentity, user_id: user.id } : null;
  return resolveAgentForUser(agents, user, alternateUser ? [alternateUser] : []);
}

export function buildMessageTimelineItems(
  messages: readonly IMMessage[],
  agents: readonly AgentLike[],
  usersById: UsersById,
  attachmentLabel: string,
): MessageTimelineItem[] {
  const items: MessageTimelineItem[] = [];
  let current: MessageTimelineItem | null = null;
  for (const message of messages) {
    if (isEventMessage(message) || emptyCompletedProgress(message)) continue;
    const user = resolveUserByLocalIdentity(message.sender_id, usersById);
    if (!user) continue;
    const fromAgent = Boolean(
      resolveMessageAgent(agents, user, message.sender_id) ||
      ["agent", "manager", "worker"].includes(user.role || "") ||
      user.participants?.some((participant) => participant.type === "agent" || participant.agent_id),
    );
    if (fromAgent) {
      if (current && !current.preview && !parseTurnProgress(message) && !parseAgentActivity(message)) {
        current.preview = timelineExcerpt(message.content).slice(0, 320);
      }
      continue;
    }
    current = message.id
      ? {
          id: message.id,
          label: timelineExcerpt(message.content).slice(0, 160) || message.attachments?.[0]?.name || attachmentLabel,
          author: user.name || user.id,
          createdAt: message.created_at,
          preview: "",
        }
      : null;
    if (current) items.push(current);
  }
  return items;
}

function timelineExcerpt(content: string): string {
  return formatMessagePreviewText(
    content
      .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
      .replace(/(^|\n)\s{0,3}(?:#{1,6}\s+|>\s?|[-*+]\s+)/g, "$1")
      .replace(/\*\*|__|`/g, ""),
  );
}

export function nearestMessageIndex(positions: readonly number[], position: number): number {
  let low = 0;
  let high = positions.length;
  while (low < high) {
    const middle = (low + high) >>> 1;
    if (positions[middle] < position) low = middle + 1;
    else high = middle;
  }
  if (low === 0) return 0;
  if (low === positions.length) return Math.max(0, low - 1);
  return position - positions[low - 1] <= positions[low] - position ? low - 1 : low;
}

export function messageTimelineMarks(count: number, limit = 25): number[] {
  const size = Math.min(count, limit);
  if (size < 2) return size ? [0] : [];
  const marks = Array.from({ length: size }, (_, index) => Math.round((index * (count - 1)) / (size - 1)));
  return marks;
}

export function timelineMarkPresentation(position: number, pointer: number | null, density: number) {
  if (pointer === null) return { width: 6 };
  const distance = (position - pointer) * Math.max(1, density - 1);
  const influence = Math.exp(-(distance * distance) / (2 * 1.6 * 1.6));
  return {
    width: 6 + 20 * influence,
  };
}
