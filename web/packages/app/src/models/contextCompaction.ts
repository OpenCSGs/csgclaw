import type { IMConversation, IMMessage, TranslateFn } from "./conversations";
import type { ContextUsage } from "./modelMetadata";

export type CompactionTarget = {
  agent_id: string;
  user_id: string;
  name: string;
  status: string;
  reason?: string;
  before?: ContextUsage;
  after?: ContextUsage;
};
export type CompactionOperation = {
  revision?: number;
  id: string;
  epoch: number;
  running: boolean;
  batch?: boolean;
  started_at: string;
  finished_at?: string;
  targets: CompactionTarget[];
};
export function messageCompaction(message: IMMessage): CompactionOperation | undefined {
  if (message.kind !== "event") return;
  const raw = message.metadata?.context_compaction;
  if (!raw || typeof raw !== "object") return;
  const op = raw as Partial<CompactionOperation>;
  if (typeof op.id !== "string" || !Array.isArray(op.targets) || typeof op.running !== "boolean") return;
  if (
    op.targets.some(
      (target) =>
        !target ||
        typeof target !== "object" ||
        typeof target.agent_id !== "string" ||
        typeof target.name !== "string" ||
        (target.reason !== undefined && typeof target.reason !== "string") ||
        typeof target.status !== "string",
    )
  )
    return;
  if (op.batch !== undefined && typeof op.batch !== "boolean") return;
  if (op.revision !== undefined && (!Number.isSafeInteger(op.revision) || op.revision < 0)) return;
  return op as CompactionOperation;
}
export function conversationCompacting(room: IMConversation): boolean {
  return room.messages.some((message) => {
    const op = messageCompaction(message);
    return op?.running && (op.epoch ?? 0) === (room.context_usage_epoch ?? 0);
  });
}
export function manualCompactionEligible(usage?: ContextUsage): boolean {
  return (
    usage?.used_tokens != null &&
    usage.context_window > 0 &&
    usage.used_tokens >= Math.max(8192, usage.context_window / 10)
  );
}

// A finished operation is terminal. Revisions also protect partial batch
// progress and subsequent usage measurements from delayed polling/SSE.
export function newerCompactionMessage<T extends IMMessage>(previous: T, incoming: T): T {
  const old = messageCompaction(previous),
    next = messageCompaction(incoming);
  if (!old || !next || old.id !== next.id || (old.epoch ?? 0) !== (next.epoch ?? 0)) return incoming;
  if ((old.revision ?? 0) > (next.revision ?? 0) || (!old.running && next.running)) return previous;
  return incoming;
}

const failureReasons = new Set([
  "scoped_session_unsupported",
  "session_binding_missing",
  "session_identity_changed",
  "session_restore_failed",
  "session_history_unreadable",
  "session_history_invalid",
  "agent_unavailable",
  "member_removed",
]);

export function compactionTargetNotice(target: CompactionTarget, t: TranslateFn): string {
  return t(
    ["unavailable", "unsupported"].includes(target.status) && target.reason && failureReasons.has(target.reason)
      ? `contextCompactReason_${target.reason}`
      : `contextCompactStatus_${target.status}`,
  );
}
