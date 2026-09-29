import { diagnosticTimelineRows, diagnosticDefaultHiddenCategories, diagnosticHiddenCategory } from "./diagnostics";
import { diagnosticLocalTime, diagnosticHiddenStats } from "./diagnostics";
import { describe, expect, it } from "vitest";
import {
  diagnosticBreakdown,
  diagnosticEventGroups,
  diagnosticDuration,
  diagnosticOccupied,
  diagnosticPhases,
  diagnosticReference,
  diagnosticFinal,
} from "./diagnostics";
import type { TurnDiagnostic } from "@/api/diagnostics";
const record: TurnDiagnostic = {
  id: "d",
  room_id: "r",
  source_id: "s",
  agent_id: "a",
  turn_id: "t",
  started_at: "2026-09-26T00:00:00Z",
  status: "succeeded",
  total_ms: 1200,
  runtime_start_ms: 100,
  runtime_end_ms: 900,
};
describe("diagnostic timing", () => {
  it("separates time after native completion from Runtime", () => {
    const phases = diagnosticPhases(record);
    expect(phases.map((p) => p.end! - p.start!)).toEqual([100, 800, 300]);
    const missing = diagnosticPhases({ ...record, runtime_start_ms: undefined, runtime_end_ms: undefined });
    expect(missing[1].start).toBeUndefined();
    expect(diagnosticDuration(undefined)).toBe("N/A");
  });
  it("unions nested and concurrent spans instead of adding them", () => {
    expect(
      diagnosticOccupied(
        [
          { id: "1", name: "a", owner: "csgclaw", start_ms: 0, end_ms: 100, status: "completed" },
          { id: "2", name: "b", owner: "csgclaw", start_ms: 50, end_ms: 150, status: "completed" },
          { id: "3", name: "c", owner: "runtime", start_ms: 0, end_ms: 1000, status: "completed" },
        ],
        "csgclaw",
        1200,
      ),
    ).toBe(150);
  });
  it("requires an explicit diagnostic reference", () => {
    expect(diagnosticReference({ codex: { request_id: "s" } })).toBeNull();
    expect(diagnosticReference({ diagnostics: { source_id: "s", room_id: "r", turn_id: "t" } })?.turn).toBe("t");
  });
});

describe("streamed turn completion", () => {
  it.each([
    ["running", false],
    ["waiting", false],
    ["succeeded", true],
    ["failed", true],
    ["canceled", true],
  ])("recognizes %s without relying on delivery_kind", (status, expected) => {
    expect(
      diagnosticFinal({
        codex: { delivery_kind: "activity" },
        csgclaw: { turn_progress: { id: "turn", revision: 1, status, started_at: "2026-09-29T00:00:00Z", items: [] } },
      }),
    ).toBe(expected);
  });
});

describe("wall-clock duration accounting", () => {
  it("makes a 48-second Runtime explainable without inventing LLM time", () => {
    const data = {
      ...record,
      total_ms: 48100,
      runtime_start_ms: 50,
      runtime_end_ms: 48050,
      spans: [
        { id: "tool", name: "tool.exec", owner: "tool", start_ms: 100, end_ms: 400, status: "completed" },
        { id: "event", name: "event.deliver", owner: "csgclaw", start_ms: 500, end_ms: 900, status: "completed" },
      ],
    };
    const parts = diagnosticBreakdown(data);
    expect(parts[0].key).toBe("unattributed");
    expect(parts[0].duration).toBe(47300);
    expect(parts.some((part) => part.key === "llm")).toBe(false);
    expect(parts.reduce((sum, part) => sum + part.duration, 0)).toBe(48100);
  });
  it("accounts for parallel tools and overlapping event handling exactly once", () => {
    const parts = diagnosticBreakdown({
      ...record,
      total_ms: 100,
      runtime_start_ms: 0,
      runtime_end_ms: 100,
      spans: [
        { id: "a", name: "tool.a", owner: "tool", start_ms: 0, end_ms: 70, status: "completed" },
        { id: "b", name: "tool.b", owner: "tool", start_ms: 30, end_ms: 80, status: "completed" },
        { id: "c", name: "llm.request", owner: "llm", start_ms: 50, end_ms: 100, status: "completed" },
      ],
    });
    expect(Object.fromEntries(parts.map((part) => [part.key, part.duration]))).toEqual({
      tool: 50,
      overlap: 30,
      llm: 20,
    });
    expect(parts.reduce((sum, part) => sum + part.duration, 0)).toBe(100);
  });
  it("groups old anonymous events and new typed events without confusing gaps with processing time", () => {
    const groups = diagnosticEventGroups([
      { id: "1", name: "event.deliver", owner: "csgclaw", start_ms: 0, end_ms: 1, status: "completed" },
      { id: "2", name: "event.deliver", owner: "csgclaw", start_ms: 1000, end_ms: 1002, status: "completed" },
    ]);
    expect(groups).toEqual([{ type: "unknown", count: 2, duration: 3, peak: 2 }]);
  });
});

it("uses native phases only for uncovered gaps and does not double-count nested observations", () => {
  const buckets = diagnosticBreakdown({
    id: "d",
    room_id: "r",
    source_id: "s",
    agent_id: "a",
    turn_id: "t",
    started_at: "2026-09-29T00:00:00Z",
    status: "succeeded",
    total_ms: 100,
    runtime_start_ms: 0,
    runtime_end_ms: 100,
    spans: [
      {
        id: "native",
        name: "runtime.native",
        owner: "runtime",
        start_ms: 0,
        end_ms: 100,
        status: "completed",
        details: { source: "codex_otel", category: "prepare" },
      },
      { id: "llm", name: "llm.request", owner: "llm", start_ms: 20, end_ms: 90, status: "completed" },
    ],
  });
  expect(buckets.find((row) => row.key === "llm")?.duration).toBe(70);
  expect(buckets.find((row) => row.key === "native_prepare")?.duration).toBe(30);
  expect(buckets.reduce((sum, row) => sum + row.duration, 0)).toBe(100);
});

it("formats local millisecond timestamps with an explicit offset", () => {
  const text = diagnosticLocalTime("2026-09-29T04:00:00.123Z", "Asia/Shanghai");
  expect(text).toContain("12:00:00.123");
  expect(text).toMatch(/GMT\+8|UTC\+8/);
});
it("reports progress union duration without counting hidden events twice", () => {
  const data = {
    id: "d",
    room_id: "r",
    source_id: "s",
    agent_id: "a",
    turn_id: "t",
    started_at: "2026-09-29T00:00:00Z",
    status: "succeeded",
    total_ms: 100,
    spans: [
      { id: "1", name: "event.deliver", owner: "csgclaw", start_ms: 10, end_ms: 30, status: "completed" },
      { id: "2", name: "event.deliver", owner: "csgclaw", start_ms: 20, end_ms: 40, status: "completed" },
    ],
  };
  expect(diagnosticHiddenStats(data, ["progress"])).toMatchObject({ count: 2, duration: 30 });
});

it("unions hidden categories across owners and follows the owner filter", () => {
  const record: TurnDiagnostic = {
    id: "d",
    room_id: "r",
    source_id: "s",
    agent_id: "a",
    turn_id: "t",
    started_at: "2026-09-29T00:00:00Z",
    status: "succeeded",
    total_ms: 100,
    spans: [
      { id: "1", name: "event.deliver", owner: "csgclaw", start_ms: 10, end_ms: 30, status: "completed" },
      {
        id: "2",
        name: "runtime.native",
        owner: "runtime",
        start_ms: 20,
        end_ms: 50,
        status: "completed",
        details: { source: "codex_otel", category: "persist" },
      },
      {
        id: "3",
        name: "runtime.native",
        owner: "runtime",
        start_ms: 25,
        end_ms: 40,
        status: "completed",
        details: { source: "codex_otel", category: "persist" },
      },
      { id: "4", name: "message.deliver", owner: "csgclaw", start_ms: 50, end_ms: 100, status: "completed" },
    ],
  };
  expect(diagnosticHiddenStats(record, ["progress", "persistence"])).toMatchObject({ count: 3, duration: 40 });
  expect(diagnosticHiddenStats(record, ["persistence"])).toMatchObject({ count: 2, duration: 30 });
  expect(diagnosticHiddenStats(record, ["progress", "persistence"], "csgclaw")).toMatchObject({
    count: 1,
    duration: 20,
  });
  expect(diagnosticHiddenStats(record, [])).toMatchObject({ count: 0, duration: 0 });
  expect(diagnosticHiddenStats(record, ["progress", "persistence"], "llm")).toMatchObject({ count: 0, duration: 0 });
});

it("numbers LLM and tool calls chronologically independently of ranking and native spans", () => {
  const data = {
    ...record,
    spans: [
      { id: "l2", name: "llm.request", owner: "llm", start_ms: 30, end_ms: 80, status: "completed" },
      { id: "t2", name: "tool.exec", owner: "tool", start_ms: 30, end_ms: 70, status: "completed" },
      { id: "native", name: "runtime.native", owner: "runtime", start_ms: 0, end_ms: 100, status: "completed" },
      { id: "l1", name: "llm.request", owner: "llm", start_ms: 10, end_ms: 15, status: "completed" },
      { id: "t1", name: "tool.exec", owner: "tool", start_ms: 20, end_ms: 25, status: "completed" },
    ],
  };
  const rows = diagnosticTimelineRows(data);
  expect(rows.map((row) => [row.span.id, row.ordinal, row.callOrdinal])).toEqual([
    ["native", 1, undefined],
    ["l1", 2, 1],
    ["t1", 3, 1],
    ["l2", 4, 2],
    ["t2", 5, 2],
  ]);
  expect(
    rows
      .filter((row) => row.callOrdinal !== undefined)
      .sort((a, b) => b.duration - a.duration)
      .map((row) => [row.span.id, row.callOrdinal]),
  ).toEqual([
    ["l2", 2],
    ["t2", 2],
    ["l1", 1],
    ["t1", 1],
  ]);
});
it("keeps transport wrappers optional and warns about hidden work without flagging whole-turn scopes as slow", () => {
  const native = (id: string, label: string, start: number, end: number, status = "completed") => ({
    id,
    name: "runtime.native",
    owner: "runtime",
    start_ms: start,
    end_ms: end,
    status,
    details: { source: "codex_otel", label },
  });
  const data = {
    ...record,
    total_ms: 1000,
    spans: [
      native("scope", "session_task.turn", 0, 1000),
      native("hook", "run_hooks_and_record_inputs", 10, 160),
      native("failure", "run_turn_stop_hooks", 800, 801, "failed"),
      native("http", "responses.stream_request", 160, 790),
    ],
  };
  expect(data.spans.map(diagnosticHiddenCategory)).toEqual(["scope", "hooks", "hooks", "http"]);
  expect(diagnosticDefaultHiddenCategories).not.toContain("http");
  expect(diagnosticHiddenStats(data, diagnosticDefaultHiddenCategories)).toMatchObject({ count: 3, duration: 151 });
  expect(diagnosticHiddenStats(data, diagnosticDefaultHiddenCategories).attention).toEqual({
    count: 2,
    failed: 1,
    slow: 1,
    categories: ["hooks"],
  });
});
