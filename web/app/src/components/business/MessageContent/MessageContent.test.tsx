// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MessageContent } from "./MessageContent";

function questionMessage(status: "pending" | "answered") {
  return {
    id: "question-request-1",
    content: "## Questions\n\n- demo_kind：What kind of demo?\n  - Bug fix (Recommended) (Plans a focused repair.)",
    metadata: {
      csgclaw: {
        agent_activity: {
          type: "com.opencsg.csgclaw.agent.activity",
          version: 1,
          event_id: "question-request-1",
          sender: "u-manager",
          channel: "csgclaw",
          room_id: "room-1",
          origin_server_ts: 1,
          content: {
            msgtype: "com.opencsg.csgclaw.agent.question",
            body: `Question ${status}`,
            question: {
              id: "request-1",
              status,
              questions: [
                {
                  id: "demo_kind",
                  header: "Demo kind",
                  question: "What kind of demo?",
                  options: [
                    {
                      label: "Bug fix (Recommended)",
                      description: "Plans a focused repair.",
                    },
                  ],
                },
              ],
            },
          },
        },
      },
    },
  };
}

describe("MessageContent question transcripts", () => {
  it("uses structured metadata for a pending interactive question", () => {
    const message = questionMessage("pending");
    render(<MessageContent content={message.content} message={message} t={(key) => key} />);

    expect(screen.getByText("questionRequest")).toBeInTheDocument();
    expect(screen.getByText("What kind of demo?")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Questions" })).not.toBeInTheDocument();
  });

  it("renders the same resolved question message as ordinary Markdown", () => {
    const message = questionMessage("answered");
    render(<MessageContent content={message.content} message={message} t={(key) => key} />);

    expect(screen.getByRole("heading", { name: "Questions" })).toBeInTheDocument();
    expect(screen.getByText(/demo_kind：What kind of demo/)).toBeInTheDocument();
    expect(screen.queryByText("questionRequest")).not.toBeInTheDocument();
  });

  it("keeps historical resolved JSON activity readable as an activity card", () => {
    const message = questionMessage("answered");
    const legacyContent = JSON.stringify(message.metadata.csgclaw.agent_activity);
    render(
      <MessageContent content={legacyContent} message={{ id: message.id, content: legacyContent }} t={(key) => key} />,
    );

    expect(screen.getByText("questionRequest")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Questions" })).not.toBeInTheDocument();
  });

  it("renders a local-user answer transcript as ordinary Markdown", () => {
    const content = "## Answers\n\n- demo_kind：Bug fix (Recommended) (Plans a focused repair.)";
    const message = {
      id: "answer-request-1",
      content,
      metadata: {
        csgclaw: {
          request_user_input: { kind: "answer", request_id: "request-1" },
        },
      },
    };
    render(<MessageContent content={content} message={message} />);

    expect(screen.getByRole("heading", { name: "Answers" })).toBeInTheDocument();
    expect(screen.getByText(/demo_kind：Bug fix/)).toBeInTheDocument();
  });
});
