import { describe, expect, it } from "vitest";
import { newerProgressMessage, progressDuration, progressGroups, toolRowPreview } from "./turnProgress";
import type { ProgressItem, TurnProgress } from "./turnProgress";

const tool = (id: string, action: string, status = "completed"): ProgressItem => ({
  id,
  kind: "tool",
  tool: { name: id, actions: [action], status },
});
describe("turn progress projection", () => {
  it("groups only adjacent tools and counts calls rather than action types", () => {
    const groups = progressGroups([
      tool("a", "read"),
      tool("b", "read"),
      { id: "text", kind: "commentary", text: "Check" },
      tool("c", "execute"),
    ]);
    expect(groups.map((group) => group.items.length)).toEqual([2, 1, 1]);
    expect(groups.filter((group) => group.kind === "tool").map((group) => group.firstToolNumber)).toEqual([1, 3]);
  });
  it("summarizes historical tool input without showing raw JSON", () => {
    const mcp = { ...tool("smart_search", "tool").tool!, input: '{"arguments":{"q":"上海天气"}}' };
    const web = { ...tool("web", "web").tool!, input: '{"action":{"type":"openPage","url":"https://example.test"}}' };
    expect(toolRowPreview(mcp)).toBe("上海天气");
    expect(toolRowPreview(web)).toBe("https://example.test");
    expect(toolRowPreview({ ...mcp, input: '{"arguments":' })).toBe("smart_search");
  });
  it("ignores stale and duplicate snapshots", () => {
    const message = (revision: number) => ({
      metadata: {
        csgclaw: {
          turn_progress: { id: "turn", revision, status: "running", started_at: "2026-09-26T00:00:00Z", items: [] },
        },
      },
    });
    const current = message(5);
    expect(newerProgressMessage(current, message(4))).toBe(current);
    expect(newerProgressMessage(current, message(5))).toBe(current);
    expect(newerProgressMessage(current, message(6))).not.toBe(current);
  });
  it("freezes elapsed time at the terminal server timestamp", () => {
    const progress = { started_at: "2026-09-26T00:00:00Z", ended_at: "2026-09-26T00:01:23Z" } as TurnProgress;
    expect(progressDuration(progress, Date.now())).toBe("1m 23s");
  });
  it("continues numbering across both text kinds and starts each turn from one", () => {
    const groups = progressGroups([
      tool("a", "read"),
      { id: "explanation", kind: "commentary", text: "Next" },
      tool("b", "execute"),
      { id: "thought", kind: "reasoning", text: "Check" },
      tool("c", "search"),
      tool("d", "read"),
    ]);
    expect(groups.filter((group) => group.kind === "tool").map((group) => group.firstToolNumber)).toEqual([1, 2, 3]);
    expect(progressGroups([tool("next-turn", "execute")])[0].firstToolNumber).toBe(1);
  });
});
