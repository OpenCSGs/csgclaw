import { describe, expect, it } from "vitest";
import { taskConversationTitle, uniqueTaskConversations } from "@/models/taskConversations";

describe("task conversations", () => {
  it("generates titles without splitting emoji and falls back for empty tasks", () => {
    expect(taskConversationTitle("  hello\n world ", "Task")).toBe("hello world");
    expect(taskConversationTitle(" ", "Task")).toBe("Task");
    expect(taskConversationTitle("😀".repeat(40), "Task")).toBe("😀".repeat(32) + "…");
  });
  it("deduplicates conversations when live ordering changes between pages", () => {
    const room = { id: "a", title: "Before", members: [], messages: [] };
    expect(
      uniqueTaskConversations([
        { rooms: [room], total: 2, has_more: true },
        { rooms: [{ ...room, title: "After" }], total: 2, has_more: false },
      ]),
    ).toEqual([{ ...room, title: "After" }]);
  });
});
