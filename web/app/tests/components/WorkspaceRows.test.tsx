import { render, screen } from "@testing-library/react";
import {
  WorkspaceAgentRow,
  WorkspaceConversationRow,
  WorkspaceHumanRow,
  WorkspaceThreadRow,
} from "@/pages/WorkspacePage/components/WorkspaceRows";
import { avatarFallbackText } from "@/shared/avatar";
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
  it("renders a Feishu badge on connected agent rows", () => {
    render(
      <WorkspaceAgentRow
        active={false}
        item={{
          id: "u-dev",
          name: "dev",
          provider: "api",
          model_id: "gpt-test",
          participants: [
            {
              agent_id: "u-dev",
              channel: "feishu",
              channel_user_kind: "app_id",
              id: "dev",
              type: "agent",
            },
          ],
        }}
        onSelect={() => {}}
        t={t}
      />,
    );

    const badge = screen.getByLabelText("feishuConnected");
    expect(badge).toBeInTheDocument();
    expect(badge.querySelector("img")).toHaveAttribute("src", "icons/feishu.png");
  });

  it("renders a Feishu badge on connected human rows", () => {
    render(
      <WorkspaceHumanRow
        active={false}
        user={{
          id: "admin",
          name: "Admin",
          participants: [
            {
              channel: "feishu",
              channel_user_kind: "open_id",
              channel_user_ref: "ou_admin",
              id: "admin",
              type: "human",
            },
          ],
        }}
        onSelect={() => {}}
        t={t}
      />,
    );

    const badge = screen.getByLabelText("feishuConnected");
    expect(badge).toBeInTheDocument();
    expect(badge.querySelector("img")).toHaveAttribute("src", "icons/feishu.png");
  });

  it("renders direct message rows without room avatar placeholders", () => {
    const usersById = new Map([
      ["u-local", { id: "u-local", name: "本地用户", avatar: "LU" }],
      ["u-agent", { id: "u-agent", name: "Alice Bob", avatar: "" }],
    ]);
    const conversation: IMConversation = {
      id: "dm-1",
      is_direct: true,
      members: ["u-local", "u-agent"],
      messages: [{ content: "hello", created_at: "2026-05-21T08:00:00Z" }],
      title: "Alice",
    };

    render(
      <WorkspaceConversationRow
        active={false}
        conversation={conversation}
        currentUserID="u-local"
        locale="en"
        onSelect={() => {}}
        onPreviewUser={() => {}}
        t={t}
        usersById={usersById}
      />,
    );

    const row = screen.getAllByRole("button")[0];
    expect(row).toHaveTextContent("Alice");
    expect(row).not.toHaveTextContent("#");
    expect(row).toHaveTextContent(avatarFallbackText("", "Alice Bob", "", "u-agent"));
  });

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
    expect(row).not.toHaveAttribute("title");
  });
});
