import { describe, it, expect } from "vitest";
import { scheduledTaskExecutors, scheduledTaskMention, normalizeScheduledTask } from "@/models/scheduledTasks";

describe("scheduled task executors", () => {
  it("includes Manager and puts it before other executors without mutating the list", () => {
    const agents = [
      { id: "agent-linda", name: "Linda", role: "worker" },
      { id: "agent-manager", name: "Manager", role: "manager" },
    ];
    expect(scheduledTaskExecutors(agents).map((item) => item.id)).toEqual(["agent-manager", "agent-linda"]);
    expect(agents[0].id).toBe("agent-linda");
  });
  it.each([
    ["汇总一下@Lin", 8, { query: "Lin", start: 4, end: 8 }],
    ["明天@ 继续报告", 3, { query: "", start: 2, end: 3 }],
    ["@测试工程师 Linda ", 12, null],
    ["明天下午汇总报告", 8, null],
  ])("recognizes mentions at the caret: %s", (text, caret, want) => {
    expect(scheduledTaskMention(text, caret)).toEqual(want);
  });
  it("preserves the server's timezone for recurrence edits", () => {
    expect(normalizeScheduledTask({ id: "task-1", timezone: "Asia/Shanghai" })?.timezone).toBe("Asia/Shanghai");
  });
});
