import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TaskConversationControls } from "../TaskConversationControls";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import type { ContextUsage } from "@/models/modelMetadata";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane";
import { messages } from "@/shared/i18n/messages";
const t: TranslateFn = (key, params = {}) =>
  Object.entries(params).reduce(
    (text, [name, value]) => text.replace(`{${name}}`, String(value)),
    String(messages.zh[key as keyof typeof messages.zh] || key),
  );
const agents: AgentLike[] = Array.from({ length: 8 }, (_, i) => ({
  id: `agent-${i}`,
  user_id: `u-${i}`,
  name: `Agent ${i}`,
  runtime_kind: "codex",
}));
const room: IMConversation = { id: "room-one", is_direct: true, members: ["user-admin", "u-0"], messages: [] };
const usage: ContextUsage = {
  session_id: "session-one",
  model_id: "test-model",
  used_tokens: 20000,
  context_window: 100000,
  context_source: "default",
  auto_compact: true,
  compact_threshold: 75000,
  compacting: false,
  estimated: false,
  updated_at: "2026-11-11T00:00:00Z",
};
const participants: ConversationWorkingParticipant[] = [{ id: "u-0", name: "Agent 0", contextUsage: usage }];

describe("conversation context usage", () => {
  it("reads persisted usage while idle and after remount, and clears it with the room", async () => {
    const saved = { ...room, context_usage: { "u-0": usage } };
    const props = { agents, t, conversation: saved, participants: [] };
    const view = render(<TaskConversationControls {...props} />);
    await userEvent.hover(screen.getByRole("button", { name: "上下文已使用 20%" }));
    expect(await screen.findByRole("tooltip")).toHaveTextContent("已用：20 K tokens");
    view.unmount();
    const restarted = render(<TaskConversationControls {...props} />);
    expect(screen.getByRole("button", { name: "上下文已使用 20%" })).toBeInTheDocument();
    restarted.rerender(<TaskConversationControls {...props} conversation={room} />);
    expect(screen.getByRole("button", { name: "尚未获得上下文用量" })).toBeInTheDocument();
  });
  it("uses one circular group entry and directly shows the same full statistics for each agent", async () => {
    const group = {
      ...room,
      is_direct: false,
      members: ["user-admin", ...agents.map((a) => String(a.user_id))],
      context_usage: { "u-0": usage },
    };
    const props = { agents, t, conversation: group, participants: [] };
    const view = render(<TaskConversationControls {...props} />);
    const entry = screen.getByRole("button", { name: "上下文用量（8），最高已使用 20%" });
    expect(entry.querySelector("svg")).toBeInTheDocument();
    await userEvent.hover(entry);
    const panel = await screen.findByRole("list", { name: "上下文用量（8）" });
    expect(within(panel).getAllByRole("listitem")).toHaveLength(8);
    expect(panel.querySelector("summary")).toBeNull();
    expect(screen.queryByRole("heading", { name: "上下文用量（8）" })).not.toBeInTheDocument();
    expect(panel).toHaveTextContent("Agent 0");
    expect(panel).toHaveTextContent("模型：test-model");
    expect(panel).toHaveTextContent("已用：20 K tokens");
    expect(panel).toHaveTextContent("剩余：80 K tokens");
    expect(panel).toHaveTextContent("自动整理：已开启");
    view.rerender(
      <TaskConversationControls
        {...props}
        conversation={{ ...group, context_usage: { "u-0": { ...usage, used_tokens: 80000 } } }}
      />,
    );
    expect(panel).toHaveTextContent("上下文已使用 80%");
  });
  it("ignores notification bots and does not use a thread's usage for the main room", () => {
    const view = render(
      <TaskConversationControls
        conversation={room}
        agents={[{ ...agents[0], bot_type: "notification" }]}
        t={t}
        participants={[]}
      />,
    );
    expect(screen.queryByRole("button", { name: "尚未获得上下文用量" })).not.toBeInTheDocument();
    view.rerender(
      <TaskConversationControls
        conversation={{ ...room, context_usage: { "u-0": usage } }}
        agents={agents}
        t={t}
        participants={[{ ...participants[0], threadRootID: "thread", contextUsage: { ...usage, used_tokens: 90000 } }]}
      />,
    );
    expect(screen.getByRole("button", { name: "上下文已使用 20%" })).toBeInTheDocument();
  });
  it("does not show the active lease's previous usage after the room is cleared", () => {
    render(
      <TaskConversationControls
        conversation={{ ...room, context_usage_epoch: 1 }}
        agents={agents}
        t={t}
        participants={participants}
      />,
    );
    expect(screen.getByRole("button", { name: "尚未获得上下文用量" })).toBeInTheDocument();
  });
});
