import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TaskConversationControls } from "./TaskConversationControls";
import type { IMConversation, IMMessage } from "@/models/conversations";

const conversation: IMConversation = {
  id: "task-1",
  is_direct: true,
  members: ["u-admin", "u-dev"],
  messages: [],
  configuration: { workspace_dir: "/project", execution_mode: "read-only" },
};
const agent = { id: "agent-dev", user_id: "u-dev", name: "Developer", runtime_kind: "codex" as const };
const t = (key: string) => key;

describe("task conversation controls", () => {
  it("shows new task after a chat message arrives and carries the agent and configuration", () => {
    const onNewTask = vi.fn();
    const props = { agents: [agent], participants: [], t, onNewTask };
    const { rerender } = render(<TaskConversationControls {...props} conversation={conversation} />);
    expect(screen.queryByRole("button", { name: "taskEntryNew" })).not.toBeInTheDocument();

    rerender(
      <TaskConversationControls
        {...props}
        conversation={{ ...conversation, messages: [{ id: "message-1", sender_id: "u-admin", content: "Hello" }] }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "taskEntryNew" }));
    expect(onNewTask).toHaveBeenCalledExactlyOnceWith(agent.id, conversation.configuration);

    rerender(<TaskConversationControls {...props} conversation={{ ...conversation, id: "task-2" }} />);
    expect(screen.queryByRole("button", { name: "taskEntryNew" })).not.toBeInTheDocument();
  });

  it("does not count room events as a chat", () => {
    render(
      <TaskConversationControls
        conversation={{ ...conversation, messages: [{ kind: "event", content: "Room created" }] }}
        agents={[agent]}
        participants={[]}
        t={t}
        onNewTask={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: "taskEntryNew" })).not.toBeInTheDocument();
  });

  it("counts an attachment without text as a chat message", () => {
    const message: IMMessage = {
      content: "",
      attachments: [
        {
          id: "attachment-1",
          name: "report.txt",
          kind: "file",
          media_type: "text/plain",
          size_bytes: 1,
          sha256: "test",
          created_at: "2026-10-20T10:00:00Z",
          download_url: "api/v1/attachments/attachment-1",
        },
      ],
    };
    render(
      <TaskConversationControls
        conversation={{ ...conversation, messages: [message] }}
        agents={[agent]}
        participants={[]}
        t={t}
        onNewTask={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "taskEntryNew" })).toBeInTheDocument();
  });
});
