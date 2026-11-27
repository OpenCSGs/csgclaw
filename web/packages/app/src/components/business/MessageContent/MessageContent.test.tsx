// @vitest-environment jsdom

import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { MessageContent } from "./MessageContent";

const translate = createTranslator("zh");

function streamingFrame(reasoning: string, content = "\u200b", status = "running") {
  return (
    <MessageContent
      t={translate}
      content={content}
      message={{
        content,
        metadata: {
          csgclaw: {
            turn_progress: {
              id: "streaming-turn",
              revision: 1,
              status,
              started_at: "2026-11-26T08:00:00Z",
              items: [{ id: "reasoning", kind: "reasoning", text: reasoning }],
            },
          },
        },
      }}
    />
  );
}

describe("streaming agent messages", () => {
  it("reveals appended reasoning and answer text with just one active cursor, then renders the final Markdown", () => {
    vi.useFakeTimers();
    try {
      const { rerender, container } = render(streamingFrame("先检查"));
      const activeText = () => container.querySelector("[data-streaming-text]");
      expect(activeText()).toHaveTextContent("先检查");

      rerender(streamingFrame("先检查配置并运行测试"));
      expect(screen.queryByText("先检查配置并运行测试")).not.toBeInTheDocument();
      act(() => vi.advanceTimersByTime(24));
      expect(activeText()).toHaveTextContent(/^先检查配置并$/);

      rerender(streamingFrame("先检查配置并运行测试", "检查结果"));
      expect(screen.getByText("先检查配置并运行测试").closest("[data-streaming-text]")).toBeNull();
      expect(activeText()).toHaveTextContent(/^检查结果$/);
      expect(container.querySelectorAll("[data-streaming-text]")).toHaveLength(1);

      rerender(streamingFrame("先检查配置并运行测试", "检查结果已发送👩‍💻"));
      expect(activeText()).toHaveTextContent(/^检查结果$/);
      act(() => vi.advanceTimersByTime(24));
      expect(activeText()).toHaveTextContent(/^检查结果已发送$/);
      act(() => vi.advanceTimersByTime(24));
      expect(activeText()).toHaveTextContent(/^检查结果已发送👩‍💻$/);

      rerender(streamingFrame("先检查配置并运行测试", "**检查结果已发送👩‍💻**", "succeeded"));
      expect(activeText()).toBeNull();
      expect(screen.getByText("检查结果已发送👩‍💻").tagName).toBe("STRONG");
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows deltas immediately when reduced motion is enabled", () => {
    vi.stubGlobal("matchMedia", () => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
    try {
      const { rerender } = render(streamingFrame("先检查"));
      rerender(streamingFrame("先检查配置并运行测试"));
      expect(screen.getByText("先检查配置并运行测试")).toBeVisible();
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

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

it("localizes recorded stop notices when the language changes and preserves ordinary messages", () => {
  const content = "Conversation interrupted";
  const message = { metadata: { csgclaw: { delivery_kind: "turn_stopped" } } };
  const { rerender } = render(<MessageContent content={content} message={message} t={createTranslator("en")} />);
  expect(screen.getByText(content)).toBeVisible();

  rerender(<MessageContent content={content} message={message} t={createTranslator("zh")} />);
  expect(screen.getByText("会话已中断")).toBeVisible();

  rerender(<MessageContent content={content} t={createTranslator("zh")} />);
  expect(screen.getByText(content)).toBeVisible();
});

describe("MessageContent question transcripts", () => {
  it("highlights both tagged and plain mentions resolved by the server", () => {
    const content =
      '请 @dev 检查，<at user_id="user-manager">manager</at> 验收，@unknown 保持普通文本。`@dev` [@dev](https://example.com) @"notes @dev" /@dev/file';
    const { container } = render(
      <MessageContent
        content={content}
        message={{
          content,
          mentions: [
            { id: "user-manager", name: "manager" },
            { id: "user-ljbpmz", name: "dev" },
          ],
        }}
      />,
    );

    const mentions = [...container.querySelectorAll<HTMLElement>(".message-mention")];
    expect(mentions.map((mention) => [mention.textContent, mention.dataset.userId])).toEqual([
      ["@dev", "user-ljbpmz"],
      ["@manager", "user-manager"],
    ]);
    expect(screen.getByRole("link", { name: "@dev" })).toBeVisible();
  });

  it("renders backend-provided runtime error content without client-side rewriting", () => {
    const content =
      "Runtime error: unexpected status 402 Payment Required. The model service balance is insufficient. Add funds or contact an administrator.\n\n👉 [Recharge your account](https://opencsg-stg.com/settings/billing) to continue., url: http://127.0.0.1:49229/api/v1/agents/agent-manager/llm/responses, request id: secret";
    render(
      <MessageContent
        content={content}
        message={{ id: "runtime-error-1", metadata: { csgclaw: { runtime_error: true } } }}
        t={(key) => {
          if (key === "errors.insufficient_balance") return "模型服务余额不足，请充值或联系管理员后重试。";
          if (key === "rechargeAccount") return "前往充值";
          return key;
        }}
      />,
    );

    expect(screen.getByText(/unexpected status 402 Payment Required/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Recharge your account" })).toHaveAttribute(
      "href",
      "https://opencsg-stg.com/settings/billing",
    );
    expect(screen.getByText(/127\.0\.0\.1/)).toBeInTheDocument();
  });

  it("does not localize human-authored text that quotes a runtime error", () => {
    const content =
      "Runtime error: unexpected status 402 Payment Required. The model service balance is insufficient.\n\n👉 [Recharge your account](https://example.com/settings/billing) to continue.";
    render(
      <MessageContent
        content={content}
        message={{ id: "human-message-1", metadata: { codex: { delivery_kind: "final" } } }}
        t={(key) => key}
      />,
    );

    expect(screen.getByText(/unexpected status 402 Payment Required/)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "rechargeAccount" })).not.toBeInTheDocument();
  });

  it("does not parse legacy runtime status strings", () => {
    render(
      <MessageContent
        content="Runtime error: unexpected status 503 Service Unavailable: Unknown error, url: http://127.0.0.1:54212/api/v1/agents/agent-1/llm/responses"
        message={{ id: "runtime-error-503", metadata: { csgclaw: { runtime_error: true } } }}
        t={(key) => (key === "errors.upstream_unavailable" ? "模型服务暂时不可用，请稍后重试。" : key)}
      />,
    );

    expect(screen.getAllByText(/Unknown error|127\.0\.0\.1/).length).toBeGreaterThan(0);
  });

  it("uses structured metadata for a pending interactive question", () => {
    const message = questionMessage("pending");
    render(<MessageContent content={message.content} message={message} t={(key) => key} />);

    expect(screen.getByText("questionRequest")).toBeInTheDocument();
    expect(screen.getByText("What kind of demo?")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Questions" })).not.toBeInTheDocument();
  });

  it("opens the selected question in the composer with the original option number", () => {
    const message = questionMessage("pending");
    const questions = message.metadata.csgclaw.agent_activity.content.question.questions;
    questions.push({
      id: "scope",
      header: "Scope",
      question: "Which scope?",
      options: [{ label: "Current conversation", description: "" }],
    });
    const onQuestionSelect = vi.fn();
    render(
      <MessageContent
        content={message.content}
        message={message}
        onQuestionSelect={onQuestionSelect}
        t={(key) => key}
      />,
    );

    expect(screen.queryByText("Which scope?")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "questionNext" }));
    expect(screen.getByText("Which scope?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Current conversation" }));
    expect(onQuestionSelect).toHaveBeenLastCalledWith("request-1", "scope", 1);
    fireEvent.click(screen.getByRole("button", { name: "questionAnswer" }));
    expect(onQuestionSelect).toHaveBeenLastCalledWith("request-1", "scope", undefined);
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
