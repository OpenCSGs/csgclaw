import { describe, expect, it } from "vitest";
import {
  buildMessageTimelineItems,
  messageTimelineMarks,
  nearestMessageIndex,
  timelineMarkPresentation,
} from "./timelineUtils";
import type { MessageAttachment } from "@/models/attachments";

function attachment(name: string): MessageAttachment {
  return {
    name,
    id: "file",
    kind: "file",
    created_at: "",
    download_url: "files/file",
    media_type: "application/pdf",
    sha256: "",
    size_bytes: 100,
  };
}

describe("message directory entries", () => {
  it("pairs each prompt with its own reply, strips Markdown, and ignores tool activity", () => {
    const users = new Map([
      ["jared", { id: "jared", name: "Jared" }],
      ["agent", { id: "agent", role: "worker" }],
    ]);
    const entries = buildMessageTimelineItems(
      [
        { id: "q1", sender_id: "jared", content: "**First** question" },
        {
          id: "tool",
          sender_id: "agent",
          content: "Command output",
          metadata: {
            csgclaw: {
              agent_activity: {
                type: "com.opencsg.csgclaw.agent.activity",
                content: { msgtype: "com.opencsg.csgclaw.agent.tool", body: "Tool details" },
              },
            },
          },
        },
        { id: "a1", sender_id: "agent", content: "## Answer\nUse the [guide](https://example.com) and `settings`." },
        { id: "q2", sender_id: "jared", content: "Unanswered" },
        { id: "q3", sender_id: "jared", content: "Third" },
        { id: "a3", sender_id: "agent", content: "Third answer" },
      ],
      [],
      users,
      "Attachment",
    );
    expect(entries.map(({ label, preview }) => ({ label, preview }))).toEqual([
      { label: "First question", preview: "Answer Use the guide and settings." },
      { label: "Unanswered", preview: "" },
      { label: "Third", preview: "Third answer" },
    ]);
  });
  it("indexes rendered human messages, including attachments and aliased senders, without events or agent replies", () => {
    const users = new Map([
      ["jared", { id: "jared", name: "Jared", role: "admin" }],
      ["worker", { id: "worker", name: "Worker", role: "worker" }],
      ["assistant", { id: "assistant", name: "Assistant" }],
      [
        "removed-agent",
        { id: "removed-agent", name: "Old agent", participants: [{ type: "agent", agent_id: "deleted" }] },
      ],
    ]);
    expect(
      buildMessageTimelineItems(
        [
          { id: "q1", sender_id: "user-jared", content: "First\n  question" },
          { id: "reply", sender_id: "assistant", content: "Reply" },
          { id: "event", sender_id: "jared", kind: "event", content: "Joined" },
          { id: "worker-reply", sender_id: "worker", content: "Reply" },
          { id: "old-reply", sender_id: "removed-agent", content: "Old reply" },
          { id: "unknown", sender_id: "unknown", content: "Not rendered" },
          { sender_id: "jared", content: "No stable target" },
          { id: "file", sender_id: "jared", content: "", attachments: [attachment("plan.pdf")] },
          { id: "image", sender_id: "jared", content: "", attachments: [attachment("")] },
        ],
        [{ id: "assistant", name: "Assistant" }],
        users,
        "Attachment",
      ),
    ).toEqual([
      { id: "q1", label: "First question", author: "Jared", createdAt: undefined, preview: "Reply" },
      { id: "file", label: "plan.pdf", author: "Jared", createdAt: undefined, preview: "" },
      { id: "image", label: "Attachment", author: "Jared", createdAt: undefined, preview: "" },
    ]);
  });
});

describe("message directory geometry", () => {
  it.each([
    [-20, 0],
    [0, 0],
    [120, 1],
    [180, 1],
    [1000, 2],
    [9000, 2],
  ])("finds the nearest message in unevenly spaced history at %s", (position, expected) => {
    expect(nearestMessageIndex([20, 120, 1000], position)).toBe(expected);
  });
  it("keeps long histories sparse with stable evenly spaced marks", () => {
    const marks = messageTimelineMarks(1000);
    expect(marks).toHaveLength(25);
    expect(marks[0]).toBe(0);
    expect(marks.at(-1)).toBe(999);
    expect([...marks].sort((a, b) => a - b)).toEqual(marks);
    expect(new Set(marks).size).toBe(marks.length);
    expect(messageTimelineMarks(3)).toEqual([0, 1, 2]);
    expect(messageTimelineMarks(0)).toEqual([]);
  });
  it("expands neighboring marks continuously and returns them to uniform idle strokes", () => {
    const center = timelineMarkPresentation(0.5, 0.5, 25);
    const nearby = timelineMarkPresentation(0.5, 0.55, 25);
    const far = timelineMarkPresentation(0.5, 0.9, 25);
    expect(center.width).toBeGreaterThan(nearby.width);
    expect(nearby.width).toBeGreaterThan(far.width);
    expect(Math.abs(timelineMarkPresentation(0.5, 0.551, 25).width - nearby.width)).toBeLessThan(1);
    expect(timelineMarkPresentation(0.2, null, 25)).toEqual(timelineMarkPresentation(0.8, null, 25));
  });
});
