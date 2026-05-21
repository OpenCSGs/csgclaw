import { render, screen } from "@testing-library/react";
import { WorkspaceThreadRow } from "@/pages/WorkspacePage/components/WorkspaceRows";
import type { IMConversation, ThreadView, TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key, params = {}) => {
  if (key === "threadReplies") {
    return `${params.count ?? 0} replies`;
  }
  if (key === "latestThreadReply") {
    return "Latest reply";
  }
  return key;
};

describe("WorkspaceRows", () => {
  it("renders thread rows without markdown code-fence language prefixes", () => {
    const conversation: IMConversation = {
      id: "room-1",
      is_direct: false,
      members: ["u-1"],
      messages: [],
      title: "Room 1",
    };
    const thread: ThreadView = {
      room_id: "room-1",
      root: {
        content: "```text\nthread title should be plain\n```",
        created_at: "2026-05-21T08:00:00Z",
        id: "root-1",
        sender_id: "u-1",
      },
      summary: {
        context_summary: {
          root_excerpt: "```text thread title should be plain ```",
        },
        reply_count: 0,
        root_id: "root-1",
      },
    };

    render(
      <WorkspaceThreadRow
        active={false}
        conversation={conversation}
        locale="en"
        onSelect={() => {}}
        t={t}
        thread={thread}
      />,
    );

    const row = screen.getByRole("button");
    expect(row).toHaveTextContent("thread title should be plain");
    expect(row).not.toHaveTextContent("```text");
    expect(row).toHaveAttribute("title", "thread title should be plain");
  });
});
