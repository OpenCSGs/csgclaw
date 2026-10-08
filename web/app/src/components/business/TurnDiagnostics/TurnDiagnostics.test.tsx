import userEvent from "@testing-library/user-event";
// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TurnDiagnostics } from "./TurnDiagnostics";
import { fetchDiagnostic, fetchDiagnostics } from "@/api/diagnostics";
vi.mock("@/api/diagnostics", () => ({ fetchDiagnostic: vi.fn(), fetchDiagnostics: vi.fn() }));
vi.mock("@/api/agents", () => ({ fetchAgentLogsRequest: vi.fn().mockResolvedValue("runtime log") }));
const record = {
  id: "d",
  room_id: "r",
  source_id: "s",
  agent_id: "agent-a",
  turn_id: "turn-a",
  started_at: "2026-09-26T00:00:00Z",
  runtime: "codex",
  model: "model",
  status: "failed",
  browser: { first_text_ms: 220, first_text_at: "2026-09-26T00:00:00.220Z" },
  total_ms: 1200,
  runtime_start_ms: 100,
  runtime_end_ms: 900,
  error: { code: "upstream_failure", stage: "execution", message: "Request failed: [redacted]" },
  spans: [
    { id: "event", name: "event.deliver", owner: "csgclaw", start_ms: 100, end_ms: 110, status: "completed" },
    {
      id: "native-persist",
      name: "runtime.native",
      owner: "runtime",
      start_ms: 105,
      end_ms: 120,
      status: "completed",
      details: { source: "codex_otel", category: "persist", label: "persist_rollout_items" },
    },
    { id: "persist", name: "message.deliver", owner: "csgclaw", start_ms: 900, end_ms: 1200, status: "completed" },
  ],
};
afterEach(() => vi.clearAllMocks());
describe("turn diagnostic viewer", () => {
  it("shows human-readable boundaries, missing LLM timing and failure details", async () => {
    vi.mocked(fetchDiagnostics).mockResolvedValue({
      items: [record],
      next_cursor: "",
      total: 1,
      agents: [{ id: record.agent_id, name: "manager" }],
    });
    vi.mocked(fetchDiagnostic).mockResolvedValue(record);
    const view = render(<TurnDiagnostics room="r" source="s" t={(key) => key} onClose={() => {}} />);
    await waitFor(() => expect(screen.getByText("upstream_failure")).toBeInTheDocument());
    expect(screen.getByText("diagLLMUnavailable")).toBeInTheDocument();
    expect(screen.getAllByText("300 ms").length).toBeGreaterThan(0);
    expect(screen.getByText("Request failed: [redacted]")).toBeInTheDocument();
    const user = userEvent.setup();
    expect(document.querySelectorAll("[data-event-number]")).toHaveLength(1);
    expect(document.querySelector("[data-event-number]")?.getAttribute("data-event-number")).toBe("3");
    expect(document.querySelector("[data-hidden-summary]")).toHaveTextContent("20 ms");
    await user.click(screen.getByRole("button", { name: "diagHiddenEvents" }));
    const progress = screen.getByRole("menuitemcheckbox", { name: "diagHidden_progress" });
    const persistence = screen.getByRole("menuitemcheckbox", { name: "diagHidden_persistence" });
    expect(progress).toBeChecked();
    expect(persistence).toBeChecked();
    await user.click(progress);
    expect(document.querySelectorAll("[data-event-number]")).toHaveLength(2);
    expect(document.querySelector("[data-hidden-summary]")).toHaveTextContent("15 ms");
    await user.click(persistence);
    expect(document.querySelectorAll("[data-event-number]")).toHaveLength(3);
    expect(document.querySelector("[data-hidden-summary]")).toHaveTextContent("0 ms");
    await user.click(progress);
    await user.click(persistence);
    await user.keyboard("{Escape}");
    expect(document.querySelectorAll("[data-event-number]")).toHaveLength(1);
    expect(document.querySelector("[data-first-text-marker]")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /diagStage_message_deliver/ }));
    expect(screen.getByText("diagOffset")).toBeInTheDocument();
    view.unmount();
  });
});

it("reveals the categories containing hidden failures and slow events", async () => {
  const data = {
    ...record,
    spans: [
      ...record.spans.map((span) => (span.id === "native-persist" ? { ...span, status: "failed" } : span)),
      {
        id: "hook",
        name: "runtime.native",
        owner: "runtime",
        start_ms: 300,
        end_ms: 500,
        status: "completed",
        details: { source: "codex_otel", label: "run_turn_stop_hooks" },
      },
    ],
  };
  vi.mocked(fetchDiagnostics).mockResolvedValue({
    items: [data],
    next_cursor: "",
    total: 1,
    agents: [{ id: data.agent_id, name: "manager" }],
  });
  vi.mocked(fetchDiagnostic).mockResolvedValue(data);
  const view = render(<TurnDiagnostics room="r" source="s" t={(key) => key} onClose={() => {}} />);
  const warning = await screen.findByRole("button", { name: "diagHiddenAttention" });
  expect(document.querySelectorAll("[data-event-number]")).toHaveLength(1);
  await userEvent.click(warning);
  expect(document.querySelectorAll("[data-event-number]")).toHaveLength(3);
  expect(screen.queryByRole("button", { name: "diagHiddenAttention" })).not.toBeInTheDocument();
  view.unmount();
});

it("keeps the triggering message visible while simplifying a single execution and its logs", async () => {
  const source = {
    id: "s",
    content: "Build a game",
    sender_name: "Local user",
    created_at: record.started_at,
    state: "recorded" as const,
  };
  vi.mocked(fetchDiagnostics).mockResolvedValue({
    items: [{ ...record, agent_name: "manager" }],
    next_cursor: "",
    total: 1,
    agents: [{ id: record.agent_id, name: "manager" }],
    source,
  });
  vi.mocked(fetchDiagnostic).mockResolvedValue({ ...record, agent_name: "manager" });
  const t = (key: string, params?: Record<string, string | number>) => key + (params?.name || "");
  const view = render(<TurnDiagnostics room="r" source="s" t={t} onClose={() => {}} />);
  expect(await screen.findByText("Build a game")).toBeInTheDocument();
  expect(screen.getByText(/Local user/)).toBeInTheDocument();
  expect(await screen.findByRole("heading", { name: "diagAgentExecutionmanager" })).toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "diagAgent" })).toBeNull();
  expect(screen.queryByRole("complementary")).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "diagViewAgentLogsmanager" }));
  expect(await screen.findByText("runtime log")).toBeInTheDocument();
  expect(screen.getByText("diagAgentLogsScope")).toBeInTheDocument();
  expect(screen.getByText("Build a game")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "diagBackToRecords" }));
  expect(await screen.findByRole("heading", { name: "diagAgentExecutionmanager" })).toBeInTheDocument();
  view.unmount();
});

it("does not replace a missing exact execution with another available turn", async () => {
  vi.mocked(fetchDiagnostics).mockResolvedValue({
    items: [],
    total: 0,
    next_cursor: "",
    agents: [{ id: "agent-a", name: "manager" }],
    source: { id: "s", content: "Source", sender_name: "Local user", created_at: record.started_at, state: "recorded" },
  });
  const view = render(<TurnDiagnostics room="r" source="s" turn="missing" t={(key) => key} onClose={() => {}} />);
  expect(await screen.findByText("diagExecutionUnavailable")).toBeInTheDocument();
  expect(fetchDiagnostic).not.toHaveBeenCalled();
  expect(fetchDiagnostics).toHaveBeenCalledWith(
    "r",
    expect.objectContaining({ source_id: "s", turn_id: "missing" }),
    expect.any(AbortSignal),
  );
  view.unmount();
});

it("retains execution agent choices after filtering to an empty result", async () => {
  const options = [
    { id: "agent-a", name: "manager" },
    { id: "agent-b", name: "dev" },
  ];
  vi.mocked(fetchDiagnostics).mockImplementation(async (_room, filters) => ({
    items: filters.agent_id ? [] : [record, { ...record, id: "other", agent_id: "agent-b" }],
    next_cursor: "",
    agents: options,
    total: filters.agent_id ? 0 : 2,
  }));
  vi.mocked(fetchDiagnostic).mockResolvedValue(record);
  const view = render(<TurnDiagnostics room="r" source="s" t={(key) => key} onClose={() => {}} />);
  const filter = await screen.findByRole("combobox", { name: "diagAgent" });
  await userEvent.click(filter);
  await userEvent.click(screen.getByRole("option", { name: "dev" }));
  expect(await screen.findByText("diagNoMatchingRecords")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("combobox", { name: "diagAgent" }));
  expect(screen.getByRole("option", { name: "manager" })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "dev" })).toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  view.unmount();
});

it("recovers a failed execution detail request on refresh without clearing its scope", async () => {
  vi.mocked(fetchDiagnostics).mockResolvedValue({
    items: [record],
    next_cursor: "",
    total: 1,
    agents: [{ id: record.agent_id, name: "manager" }],
  });
  vi.mocked(fetchDiagnostic).mockRejectedValueOnce(new Error("network failure")).mockResolvedValue(record);
  const view = render(
    <TurnDiagnostics room="r" source="s" turn={record.turn_id} t={(key) => key} onClose={() => {}} />,
  );
  expect(await screen.findByRole("alert")).toHaveTextContent("diagLoadFailed");
  await userEvent.click(screen.getByRole("button", { name: "refreshLogs" }));
  expect(await screen.findByText("upstream_failure")).toBeInTheDocument();
  expect(screen.queryByText("diagLoadFailed")).not.toBeInTheDocument();
  expect(fetchDiagnostics).toHaveBeenLastCalledWith(
    "r",
    expect.objectContaining({ source_id: "s", turn_id: record.turn_id }),
    expect.any(AbortSignal),
  );
  view.unmount();
});
