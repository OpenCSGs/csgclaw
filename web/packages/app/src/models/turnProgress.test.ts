import { describe, expect, it } from "vitest";
import { createTranslator } from "@/shared/i18n";
import {
  formatProgressToolDuration,
  newerProgressMessage,
  progressDuration,
  progressGroups,
  toolRowSummary,
} from "./turnProgress";
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
    expect(toolRowSummary(mcp)).toBe("上海天气");
    expect(toolRowSummary(web)).toBe("https://example.test");
    expect(toolRowSummary({ ...mcp, input: '{"arguments":' })).toBe("smart_search");
  });
  it("keeps commands in the details even when a historical snapshot used them as a summary", () => {
    const read = tool("shell", "read").tool!;
    expect(toolRowSummary({ ...read, command: "cat README.md", summary: "cat README.md" })).toBe("");
    expect(toolRowSummary({ ...read, input: '{"command":"cat README.md"}' })).toBe("");
    expect(toolRowSummary({ ...read, actions: ["execute"], summary: "cat README.md" })).toBe("");
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
  it.each([
    [undefined, "", ""],
    [0, "<1 ms", "<1 ms"],
    [19, "19 ms", "19 ms"],
    [1350, "1.4 秒", "1.4s"],
  ] as const)("formats tool duration %s without rounding fast commands to zero", (milliseconds, zh, en) => {
    expect(formatProgressToolDuration(milliseconds, createTranslator("zh"))).toBe(zh);
    expect(formatProgressToolDuration(milliseconds, createTranslator("en"))).toBe(en);
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
