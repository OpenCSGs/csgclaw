import { DiagnosticTimeline } from "./DiagnosticTimeline";
// @vitest-environment jsdom
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DiagnosticBreakdown, DiagnosticCalls, DiagnosticEvents } from "./DiagnosticAnalysis";
import { ConversationMessageActions } from "@/components/business/ConversationPane/ConversationMessageActions";
import type { TurnDiagnostic } from "@/api/diagnostics";
const record: TurnDiagnostic = {
  id: "d",
  room_id: "r",
  source_id: "s",
  agent_id: "a",
  turn_id: "t",
  started_at: "2026-09-29T00:00:00Z",
  status: "succeeded",
  total_ms: 48000,
  runtime_start_ms: 0,
  runtime_end_ms: 48000,
  spans: [
    {
      id: "tool",
      name: "tool.exec",
      owner: "tool",
      start_ms: 100,
      end_ms: 400,
      status: "completed",
      details: { label: "Read file", command: "cat demo.txt", directory: "/workspace" },
    },
  ],
};
const t = (key: string) => key;
describe("readable diagnostic presentation", () => {
  it("puts the diagnostics icon in the same toolbar as copy and thread reply", () => {
    const { container } = render(
      <ConversationMessageActions
        metadata={{ diagnostics: { source_id: "s", room_id: "r", turn_id: "t" } }}
        content="Reply"
        onOpenThread={() => {}}
        t={t}
      />,
    );
    const toolbar = container.querySelector(".message-action-controls");
    expect(toolbar).toContainElement(screen.getByRole("button", { name: "diagExecutionAction" }));
    expect(toolbar).toContainElement(screen.getByRole("button", { name: "copyToClipboard" }));
    expect(toolbar).toContainElement(screen.getByRole("button", { name: "replyInThread" }));
    expect(screen.getByRole("button", { name: "diagExecutionAction" })).not.toHaveTextContent("diagExecutionAction");
  });
  it("ranks the unmeasured gap first and exposes the actual tool command", () => {
    const { container } = render(
      <>
        <DiagnosticBreakdown record={record} t={t} />
        <DiagnosticCalls record={record} t={t} />
      </>,
    );
    expect(container.querySelector("ol li")).toHaveTextContent("diagBucket_unattributed");
    expect(screen.getAllByText("cat demo.txt").length).toBeGreaterThan(0);
    expect(screen.getByText("/workspace")).toBeInTheDocument();
  });
  it("collapses hundreds of tiny processing events into a count and measured total", () => {
    const spans = Array.from({ length: 226 }, (_, i) => ({
      id: String(i),
      name: "event.deliver",
      owner: "csgclaw",
      start_ms: i * 100,
      end_ms: i * 100 + 1,
      status: "completed",
      details: { event_type: "text_delta" },
    }));
    const { container } = render(
      <DiagnosticEvents spans={spans} t={(key, params) => key + String(params?.count || "")} />,
    );
    expect(container.querySelector("details")).not.toHaveAttribute("open");
    expect(screen.getByText("diagEventSummary226")).toBeInTheDocument();
    expect(screen.getAllByText("226 ms").length).toBeGreaterThan(0);
  });
});

it("matches per-type numbers between duration-ranked calls and filtered timeline", () => {
  const data = {
    ...record,
    spans: [
      ...(record.spans || []),
      { id: "llm-later", name: "llm.request", owner: "llm", start_ms: 500, end_ms: 1500, status: "completed" },
      { id: "llm-first", name: "llm.request", owner: "llm", start_ms: 10, end_ms: 90, status: "completed" },
    ],
  };
  const view = render(
    <>
      <DiagnosticCalls record={data} t={t} />
      <DiagnosticTimeline record={data} owner="llm" selected="" onSelect={() => {}} t={t} />
    </>,
  );
  const ranked = view.container.querySelector('details[data-call-id="llm-later"]');
  const timeline = view.container.querySelector('button[data-call-id="llm-later"]');
  expect(ranked).toHaveAttribute("data-call-number", "2");
  expect(ranked).toHaveTextContent("LLM #2");
  expect(timeline).toHaveAttribute("data-call-number", "2");
  expect(timeline).toHaveTextContent("diagLLMRequest #2");
  expect(view.container.querySelector('button[data-call-id="llm-first"]')).toHaveTextContent("diagLLMRequest #1");
  view.unmount();
});

it("uses the video model name for asynchronous model calls", () => {
  render(
    <DiagnosticCalls
      record={{
        ...record,
        model: "text-model",
        spans: [
          {
            id: "video",
            name: "llm.video",
            owner: "llm",
            status: "completed",
            start_ms: 0,
            end_ms: 40000,
            details: { label: "video-model" },
          },
        ],
      }}
      t={t}
    />,
  );
  expect(screen.getByText("video-model")).toBeTruthy();
  expect(screen.queryByText("text-model")).toBeNull();
  expect(screen.getByText("diagVideoModelHint")).toBeTruthy();
  expect(screen.queryByText("diagFirstResponseHint")).toBeNull();
});
