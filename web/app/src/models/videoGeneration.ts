import type { IMMessage } from "./conversations";
import { progressActive, type TurnProgress } from "./turnProgress";

// Keep runtime controls tied to the runtime turn; only the displayed clock
// follows the asynchronous videos associated with this reply.
export function videoProgressTiming(progress: TurnProgress, messages: readonly IMMessage[]) {
  const tasks = messages.flatMap((message) => {
    const raw = message.metadata?.video_generation;
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) return [];
    const task = raw as Record<string, unknown>;
    const started = Date.parse(String(task.started_at || ""));
    const ended = Date.parse(String(task.ended_at || ""));
    const terminal = ["completed", "failed", "delivery_failed"].includes(String(task.state));
    if (!Number.isFinite(started) || (terminal && (!Number.isFinite(ended) || ended < started))) return [];
    return [{ started, ended, terminal, failed: terminal && task.state !== "completed" }];
  });
  if (!tasks.length) return null;
  const pending = tasks.some((task) => !task.terminal);
  const failed = tasks.some((task) => task.failed);
  const active = progressActive(progress) || pending;
  const responseEnd = Date.parse(progress.ended_at || progress.updated_at);
  const end = Math.max(Number.isFinite(responseEnd) ? responseEnd : 0, ...tasks.map((task) => task.ended || 0));
  return {
    pending,
    failed,
    progress: {
      ...progress,
      status: active ? "running" : failed ? "failed" : progress.status,
      ended_at: active ? undefined : new Date(end).toISOString(),
    },
  };
}
