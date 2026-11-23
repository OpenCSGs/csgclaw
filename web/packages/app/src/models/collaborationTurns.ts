import { userIDForLocalIdentity, type IMMessage } from "./conversations";
import { isAutonomousCoordinationNotice, isAutonomousCoordinationTool } from "./agentActivity";
import { parseTurnProgress, type TurnProgress } from "./turnProgress";

export type CollaborationTurn = {
  key: string;
  id: string;
  index: number;
  senderID: string;
  sourceID: string;
  source?: IMMessage;
  threadRootID: string;
  startedAt: string;
  endedAt?: string;
  status: string;
  progress?: TurnProgress;
  messages: IMMessage[];
};

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}
function text(...values: unknown[]): string {
  return values.find((value): value is string => typeof value === "string" && Boolean(value.trim()))?.trim() || "";
}
function identity(message: IMMessage) {
  const csg = record(message.metadata?.csgclaw);
  const codex = record(message.metadata?.codex);
  const diagnostics = record(message.metadata?.diagnostics);
  const progress = parseTurnProgress(message);
  const turnID = text(progress?.id, csg.turn_id, codex.turn_id, diagnostics.turn_id);
  const sourceID = text(
    csg.source_message_id,
    csg.request_id,
    codex.source_message_id,
    codex.request_id,
    codex.requestId,
    turnID ? diagnostics.source_id : undefined,
  );
  const thread = message.relates_to?.rel_type === "m.thread" ? message.relates_to.event_id || "" : "";
  const scope = JSON.stringify([userIDForLocalIdentity(message.sender_id), thread]);
  return { turnID, sourceID, thread, scope, progress };
}

// Never infer a turn from adjacent timestamps: members and threads can overlap.
// Request-only legacy records join a known turn only when that association is unique.
export function collaborationTurns(messages: readonly IMMessage[]): {
  turns: CollaborationTurn[];
  unassigned: IMMessage[];
} {
  const scopedMessages: { message: IMMessage; meta: ReturnType<typeof identity> }[] = [];
  const groups = new Map<string, CollaborationTurn>();
  const sources = new Map<string, Set<string>>();
  const messageByID = new Map<IMMessage["id"], IMMessage>();
  for (const message of messages) {
    if (isAutonomousCoordinationNotice(message)) continue;
    const meta = identity(message);
    scopedMessages.push({ message, meta });
    messageByID.set(message.id, message);
    if (!meta.turnID) continue;
    const key = JSON.stringify([meta.scope, meta.turnID]);
    if (!groups.has(key))
      groups.set(key, {
        key,
        id: meta.turnID,
        index: 0,
        senderID: message.sender_id || "",
        sourceID: meta.sourceID,
        threadRootID: meta.thread,
        startedAt: meta.progress?.started_at || message.created_at || "",
        status: "unknown",
        messages: [],
      });
    if (meta.sourceID) {
      const sourceKey = JSON.stringify([meta.scope, meta.sourceID]);
      const matches = sources.get(sourceKey) || new Set<string>();
      matches.add(key);
      sources.set(sourceKey, matches);
    }
  }
  const unassigned: IMMessage[] = [];
  for (const { message, meta } of scopedMessages) {
    let key = meta.turnID ? JSON.stringify([meta.scope, meta.turnID]) : "";
    if (!key && meta.sourceID) {
      const sourceKey = JSON.stringify([meta.scope, meta.sourceID]);
      const matches = sources.get(sourceKey);
      if (matches?.size === 1) key = [...matches][0];
      else if (!matches?.size) {
        key = JSON.stringify([meta.scope, `request:${meta.sourceID}`]);
        if (!groups.has(key))
          groups.set(key, {
            key,
            id: "",
            index: 0,
            senderID: message.sender_id || "",
            sourceID: meta.sourceID,
            threadRootID: meta.thread,
            startedAt: message.created_at || "",
            status: "unknown",
            messages: [],
          });
      }
    }
    const group = groups.get(key);
    if (!group) {
      unassigned.push(message);
      continue;
    }
    group.messages.push(message);
    if (meta.sourceID) group.sourceID = meta.sourceID;
    if (meta.progress && (!group.progress || meta.progress.revision >= group.progress.revision)) {
      group.progress = meta.progress;
      group.status = meta.progress.status;
      group.startedAt = meta.progress.started_at;
      group.endedAt = meta.progress.ended_at;
    } else if (!group.progress && timestamp(message.created_at) < timestamp(group.startedAt)) {
      group.startedAt = message.created_at || group.startedAt;
    }
  }
  const turns = [...groups.values()].sort((a, b) => timestamp(a.startedAt) - timestamp(b.startedAt));
  turns.forEach((turn, index) => {
    turn.index = index + 1;
    turn.source = messageByID.get(turn.sourceID);
  });
  return { turns, unassigned };
}

export function collaborationTurnExcerpt(content?: string): string {
  return (content || "")
    .replace(/\u200b/g, "")
    .replace(/<at\b[^>]*>(.*?)<\/at>/g, "@$1")
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[#*_`>]+/g, "")
    .replace(/\s+/g, " ")
    .trim();
}
export function collaborationTurnMatches(turn: CollaborationTurn, query: string): boolean {
  const words = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return true;
  const corpus = [
    turn.id,
    turn.sourceID,
    turn.source?.content || "",
    ...turn.messages.map((message) => {
      const progress = parseTurnProgress(message);
      // Search the same observable records shown in the inspector, never hidden reasoning.
      return progress
        ? [
            progress.final_text,
            progress.error,
            message.content,
            ...progress.items
              .filter((item) => item.kind !== "reasoning" && !isAutonomousCoordinationTool(item.tool?.name ?? ""))
              .map((item) => item.text || JSON.stringify(item.tool)),
          ].join(" ")
        : message.content;
    }),
  ]
    .join(" ")
    .toLocaleLowerCase();
  return words.every((word) => corpus.includes(word));
}
function timestamp(value?: string): number {
  return Date.parse(value || "") || 0;
}
