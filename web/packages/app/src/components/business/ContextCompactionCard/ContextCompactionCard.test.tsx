import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ContextCompactionCard } from "./ContextCompactionCard";
import { createTranslator } from "@/shared/i18n";
import type { CompactionOperation } from "@/models/contextCompaction";
import type { ContextUsage } from "@/models/modelMetadata";

const t = createTranslator("zh");
const operation: CompactionOperation = {
  id: "one",
  epoch: 0,
  running: false,
  started_at: "2026-11-16T00:00:00Z",
  finished_at: "2026-11-16T00:00:05Z",
  targets: [{ agent_id: "a", user_id: "u", name: "manager", status: "completed" }],
};
const usage: ContextUsage = {
  session_id: "s",
  model_id: "m",
  used_tokens: 200000,
  context_window: 1000000,
  context_source: "catalog",
  auto_compact: true,
  compact_threshold: 750000,
  compacting: false,
  estimated: false,
  updated_at: "",
};
describe("compaction results", () => {
  it("explains missing measurements without duplicated completion or a false promise", () => {
    render(
      <ContextCompactionCard
        operation={{ ...operation, targets: [{ ...operation.targets[0], before: usage }] }}
        t={t}
      />,
    );
    expect(screen.getByText("manager 的上下文已压缩")).toBeInTheDocument();
    expect(screen.getByText("对话历史已整理为摘要，可继续当前对话。")).toBeInTheDocument();
    expect(screen.getByText("压缩前：200 K tokens。运行时未提供本次压缩后的用量。")).toBeInTheDocument();
    expect(screen.queryByText(/等待|待更新|压缩完成，压缩完成/)).not.toBeInTheDocument();
    expect(screen.getByText("耗时 5 秒")).toBeInTheDocument();
  });
  it("shows measured before/after usage and labels native estimates", () => {
    render(
      <ContextCompactionCard
        operation={{
          ...operation,
          targets: [
            { ...operation.targets[0], before: usage, after: { ...usage, used_tokens: 29000, estimated: true } },
          ],
        }}
        t={t}
      />,
    );
    expect(screen.getByText("用量：200 K tokens → 29 K tokens")).toBeInTheDocument();
    expect(screen.getByText("减少 171 K tokens · 估算值")).toBeInTheDocument();
  });
  it("distinguishes batch successes from agents that were skipped", () => {
    render(
      <ContextCompactionCard
        operation={{
          ...operation,
          targets: [
            { ...operation.targets[0], name: "qa" },
            { agent_id: "b", user_id: "v", name: "dev", status: "unnecessary" },
          ],
        }}
        t={t}
      />,
    );
    expect(screen.getByText("批量压缩已结束")).toBeInTheDocument();
    expect(screen.getByText("已压缩 1 个 Agent · 跳过 1 个 Agent")).toBeInTheDocument();
    expect(screen.getByText("当前上下文较少，无需压缩")).toBeInTheDocument();
  });
  it("shows a running status without a fabricated percentage", () => {
    render(
      <ContextCompactionCard
        operation={{ ...operation, running: true, targets: [{ ...operation.targets[0], status: "running" }] }}
        t={t}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("正在压缩上下文");
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });
});
