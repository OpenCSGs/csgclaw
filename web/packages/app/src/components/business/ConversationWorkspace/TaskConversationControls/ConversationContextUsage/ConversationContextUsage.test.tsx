import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TaskConversationControls } from "../TaskConversationControls";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import type { ContextUsage } from "@/models/modelMetadata";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane";
import { compactRoomContext } from "@/api/contextCompaction";
import type { CompactionOperation } from "@/models/contextCompaction";
vi.mock("@/api/contextCompaction", () => ({ compactRoomContext: vi.fn() }));
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
    await userEvent.click(screen.getByRole("button", { name: "上下文已使用 20%" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("20 K / 100 K");
    view.unmount();
    const restarted = render(<TaskConversationControls {...props} />);
    expect(screen.getByRole("button", { name: "上下文已使用 20%" })).toBeInTheDocument();
    restarted.rerender(<TaskConversationControls {...props} conversation={room} />);
    expect(screen.getByRole("button", { name: "上下文" })).toBeInTheDocument();
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
    const entry = screen.getByRole("button", { name: "上下文最高已使用 20%" });
    expect(entry.querySelector("svg")).toBeInTheDocument();
    await userEvent.click(entry);
    const panel = await screen.findByRole("dialog");
    expect(within(panel).getAllByRole("listitem")).toHaveLength(8);
    expect(panel.querySelector("summary")).toBeNull();
    expect(screen.queryByRole("heading", { name: "上下文用量（8）" })).not.toBeInTheDocument();
    expect(panel).toHaveTextContent("Agent 0");
    expect(panel).toHaveTextContent("test-model");
    expect(panel).toHaveTextContent("20 K / 100 K");
    expect(panel).toHaveTextContent("20 K / 100 K");
    expect(panel).not.toHaveTextContent("自动整理");
    expect(panel).not.toHaveTextContent("容量来源");
    expect(panel).not.toHaveTextContent("最近一次运行时上报");
    expect(panel).not.toHaveTextContent("剩余");
    view.rerender(
      <TaskConversationControls
        {...props}
        conversation={{ ...group, context_usage: { "u-0": { ...usage, used_tokens: 80000 } } }}
      />,
    );
    expect(panel).toHaveTextContent("80%");
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
    expect(screen.queryByRole("button", { name: "上下文" })).not.toBeInTheDocument();
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
    expect(screen.getByRole("button", { name: "上下文" })).toBeInTheDocument();
  });
});

it("small contexts give local feedback without a model call", async () => {
  vi.mocked(compactRoomContext).mockClear();
  render(
    <TaskConversationControls
      agents={agents}
      t={t}
      participants={[]}
      conversation={{ ...room, context_usage: { "u-0": { ...usage, used_tokens: 2000 } } }}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "上下文已使用 2%" }));
  await userEvent.click(screen.getByRole("button", { name: "压缩上下文" }));
  expect(screen.getByRole("status")).toHaveTextContent("当前上下文较少，无需压缩");
  expect(compactRoomContext).not.toHaveBeenCalled();
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole("button", { name: "上下文已使用 2%" })).toHaveFocus());
});
it("busy agents retain a disabled action and explanation", async () => {
  render(
    <TaskConversationControls
      agents={agents}
      t={t}
      participants={[{ ...participants[0], leaseID: "working" }]}
      conversation={room}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "上下文已使用 20%" }));
  expect(screen.getByRole("button", { name: "压缩上下文" })).toBeDisabled();
  expect(screen.getByText("Agent 正在工作，请本轮完成后再操作")).toBeInTheDocument();
});
it("submits one scoped operation and shows the returned result", async () => {
  vi.mocked(compactRoomContext).mockResolvedValue({
    id: "request",
    epoch: 0,
    running: false,
    started_at: "",
    targets: [{ agent_id: "agent-0", user_id: "u-0", name: "Agent 0", status: "no_history" }],
  });
  render(<TaskConversationControls agents={agents} t={t} participants={participants} conversation={room} />);
  await userEvent.click(screen.getByRole("button", { name: "上下文已使用 20%" }));
  await userEvent.click(screen.getByRole("button", { name: "压缩上下文" }));
  await waitFor(() => expect(compactRoomContext).toHaveBeenCalledWith("room-one", ["agent-0"], expect.any(String)));
  expect(screen.queryByText("暂无可压缩的历史，无需压缩")).not.toBeInTheDocument();
});

it("opens on hover and keeps panel actions reachable across the pointer gap", async () => {
  const user = userEvent.setup();
  render(<TaskConversationControls agents={agents} t={t} participants={participants} conversation={room} />);
  const entry = screen.getByRole("button", { name: "上下文已使用 20%" });
  await user.hover(entry);
  const panel = await screen.findByRole("dialog");
  expect(entry).not.toHaveFocus();
  await user.unhover(entry);
  await user.hover(panel);
  expect(screen.getByRole("button", { name: "压缩上下文" })).toBeEnabled();
  await user.click(entry);
  expect(panel).toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("closes a hover panel after leaving both the entry and panel", async () => {
  const user = userEvent.setup();
  render(<TaskConversationControls agents={agents} t={t} participants={participants} conversation={room} />);
  const entry = screen.getByRole("button", { name: "上下文已使用 20%" });
  await user.hover(entry);
  expect(await screen.findByRole("dialog")).toBeInTheDocument();
  await user.unhover(entry);
  await screen.findByRole("button", { name: "上下文已使用 20%" });
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});

it("uses newer durable usage even when a stale lease is still represented", async () => {
  const view = render(
    <TaskConversationControls agents={agents} t={t} participants={participants} conversation={room} />,
  );
  await userEvent.hover(screen.getByRole("button", { name: "上下文已使用 20%" }));
  view.rerender(
    <TaskConversationControls
      agents={agents}
      t={t}
      participants={participants}
      conversation={{
        ...room,
        context_usage: { "u-0": { ...usage, used_tokens: 2000, updated_at: "2026-11-11T00:01:00Z" } },
      }}
    />,
  );
  expect(screen.getByRole("button", { name: "上下文已使用 2%" })).toBeInTheDocument();
  expect(screen.getByRole("dialog")).toHaveTextContent("2 K / 100 K");
  expect(screen.getByRole("dialog")).not.toHaveTextContent("20 K / 100 K");
});

it("sends individual and batch compaction on HTTP without crypto.randomUUID", async () => {
  const original = globalThis.crypto;
  vi.stubGlobal("crypto", { getRandomValues: original.getRandomValues.bind(original) });
  vi.mocked(compactRoomContext).mockClear();
  vi.mocked(compactRoomContext).mockResolvedValue({
    id: "test",
    epoch: 0,
    running: false,
    started_at: "",
    targets: [],
  });
  try {
    render(
      <TaskConversationControls
        agents={agents}
        t={t}
        participants={participants}
        conversation={{ ...room, is_direct: false, members: ["user-admin", "u-0", "u-1"] }}
      />,
    );
    await userEvent.hover(screen.getByRole("button", { name: "上下文最高已使用 20%" }));
    await userEvent.click(screen.getAllByRole("button", { name: "压缩上下文" })[0]);
    await waitFor(() => expect(compactRoomContext).toHaveBeenCalledTimes(1));
    await userEvent.click(screen.getByRole("button", { name: "压缩全部上下文" }));
    await waitFor(() => expect(compactRoomContext).toHaveBeenCalledTimes(2));
    for (const call of vi.mocked(compactRoomContext).mock.calls)
      expect(call[2]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  } finally {
    vi.unstubAllGlobals();
  }
});

it("ignores a pending response from before another client cleared the room", async () => {
  let accept!: (operation: CompactionOperation) => void;
  vi.mocked(compactRoomContext).mockImplementationOnce(
    () => new Promise<CompactionOperation>((resolve) => (accept = resolve)),
  );
  const onCompactingChange = vi.fn();
  const props = { agents, t, participants: [], onCompactingChange };
  const view = render(
    <TaskConversationControls {...props} conversation={{ ...room, context_usage: { "u-0": usage } }} />,
  );
  await userEvent.click(screen.getByRole("button", { name: "上下文已使用 20%" }));
  await userEvent.click(screen.getByRole("button", { name: "压缩上下文" }));
  await waitFor(() => expect(onCompactingChange).toHaveBeenLastCalledWith(true));
  view.rerender(<TaskConversationControls {...props} conversation={{ ...room, context_usage_epoch: 1 }} />);
  accept({ id: "old-operation", epoch: 0, running: true, started_at: "", targets: [] });
  await waitFor(() => expect(onCompactingChange).toHaveBeenLastCalledWith(false));
  expect(screen.getByRole("button", { name: "压缩上下文" })).toBeEnabled();
});

it("keeps a current-epoch response pending until its status card arrives", async () => {
  const operation: CompactionOperation = {
    id: "current-operation",
    epoch: 1,
    running: true,
    started_at: "",
    targets: [],
  };
  vi.mocked(compactRoomContext).mockResolvedValueOnce(operation);
  const onCompactingChange = vi.fn();
  const props = { agents, t, participants: [], onCompactingChange };
  const conversation = { ...room, context_usage_epoch: 1, context_usage: { "u-0": usage } };
  const view = render(<TaskConversationControls {...props} conversation={conversation} />);
  await userEvent.click(screen.getByRole("button", { name: "上下文已使用 20%" }));
  await userEvent.click(screen.getByRole("button", { name: "压缩上下文" }));
  await waitFor(() => expect(onCompactingChange).toHaveBeenLastCalledWith(true));
  expect(screen.getByRole("button", { name: "压缩上下文" })).toBeDisabled();
  view.rerender(
    <TaskConversationControls
      {...props}
      conversation={{
        ...conversation,
        messages: [{ id: "card", kind: "event", content: "", metadata: { context_compaction: operation } }],
      }}
    />,
  );
  expect(onCompactingChange).toHaveBeenLastCalledWith(true);
  view.rerender(
    <TaskConversationControls
      {...props}
      conversation={{
        ...conversation,
        messages: [
          {
            id: "card",
            kind: "event",
            content: "",
            metadata: { context_compaction: { ...operation, running: false } },
          },
        ],
      }}
    />,
  );
  await waitFor(() => expect(onCompactingChange).toHaveBeenLastCalledWith(false));
  expect(screen.getByRole("button", { name: "压缩上下文" })).toBeEnabled();
});
