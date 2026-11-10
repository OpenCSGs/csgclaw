// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { diagnosticTimestamp } from "@/models/diagnostics";
import { DiagnosticTimeline } from "./DiagnosticTimeline";
import type { TurnDiagnostic } from "@/api/diagnostics";
const record: TurnDiagnostic = {
  id: "d",
  room_id: "r",
  source_id: "s",
  agent_id: "a",
  turn_id: "t",
  started_at: "2026-09-29T04:00:00Z",
  status: "succeeded",
  total_ms: 500,
  spans: [
    {
      id: "later",
      name: "tool.shell",
      owner: "tool",
      start_ms: 200,
      end_ms: 250,
      status: "completed",
      details: { label: "Run shell command", command: "printf hello", directory: "/workspace" },
    },
    {
      id: "early",
      name: "event.deliver",
      owner: "csgclaw",
      start_ms: 10,
      end_ms: 11,
      status: "completed",
      details: { event_type: "progress" },
    },
  ],
};
describe("diagnostic chronology", () => {
  it("numbers all events before filtering and exposes timestamp and command on keyboard focus", async () => {
    const props = { record, hiddenCategories: [], owner: "", selected: "", onSelect: vi.fn(), t: (key: string) => key };
    const view = render(<DiagnosticTimeline {...props} />);
    expect(
      screen
        .getAllByRole("button")
        .slice(0, 2)
        .map((el) => el.dataset.eventNumber),
    ).toEqual(["1", "2"]);
    view.rerender(<DiagnosticTimeline {...props} hiddenCategories={["progress", "persistence"]} />);
    expect(screen.getAllByRole("button").map((el) => el.dataset.eventNumber)).toEqual(["2"]);
    view.rerender(<DiagnosticTimeline {...props} owner="tool" />);
    const button = screen.getByRole("button", { name: /Run shell command/ });
    expect(button.dataset.eventNumber).toBe("2");
    expect(button).toHaveTextContent("Run shell command +200 ms");
    expect(button.querySelector('[aria-label="diagStartOffset: 200 ms"]')).toHaveTextContent("+200 ms");
    fireEvent.focus(button);
    await waitFor(() => expect(screen.getAllByText("printf hello").length).toBeGreaterThan(0));
    expect(screen.getAllByText("diagStartOffset").length).toBeGreaterThan(0);
    expect(screen.getAllByText(diagnosticTimestamp(record, 200)).length).toBeGreaterThan(0);
    fireEvent.click(button);
    expect(props.onSelect).toHaveBeenCalledWith("later");
    view.unmount();
  });
});
