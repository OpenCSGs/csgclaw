import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MessageContent } from "@/components/business/MessageContent";
import {
  ACTION_REBUILD_MANAGER,
  AgentActivityMsgTypes,
  CSGCLAW_AGENT_ACTIVITY_TYPE,
  CSGCLAW_ACTION_CARD_TYPE,
  CSGCLAW_NOTIFY_CARD_TYPE,
} from "@/shared/constants/messages";

const longMessageLabels: Record<string, string> = {
  messageLongCollapse: "收起",
  messageLongExpand: "展开全文",
};

function longMessageT(key: string): string {
  return longMessageLabels[key] ?? key;
}

function mockLongMessageLayout() {
  const scrollHeightSpy = vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockImplementation(function (
    this: HTMLElement,
  ) {
    const text = (this.textContent || "").replace(/\s+/g, " ").trim();
    if (!text) {
      return 0;
    }
    return text.length > 100 ? 520 : 120;
  });
  return () => {
    scrollHeightSpy.mockRestore();
  };
}

describe("MessageContent", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders an animated three-dot indicator for blank turn placeholders", () => {
    render(<MessageContent content={"\u200b"} />);

    const indicator = screen.getByLabelText("Waiting for response");
    expect(indicator).toHaveClass("message-loading-dots");
    expect(indicator.querySelectorAll(".message-loading-dot")).toHaveLength(3);
    expect(screen.queryByText("\u200b")).not.toBeInTheDocument();
  });

  it("renders sanitized markdown links with safe external-link attributes", () => {
    render(<MessageContent content={'Open [docs](https://example.com/docs)<script>alert("xss")</script>'} />);

    const link = screen.getByRole("link", { name: "docs" });
    expect(link).toHaveAttribute("href", "https://example.com/docs");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(document.querySelector("script")).toBeNull();
  });

  it("automatically collapses long markdown messages and toggles them in place", async () => {
    const restoreLayout = mockLongMessageLayout();
    const user = userEvent.setup();
    const longText =
      "今天整理了一下整个Agent系统的设计，包含对话能力、工具调用、MCP支持、资源管理、消息流转、权限控制、任务编排、模型选择、运行时配置与会话状态保持。" +
      "为了确保这个消息在聊天区域中不会占据太多空间，我们需要默认折叠它，并允许用户按需展开查看完整内容。".repeat(4);

    const { container } = render(<MessageContent content={longText} enableLongMessageCollapse t={longMessageT} />);

    const expandButton = await screen.findByRole("button", { name: "展开全文" });
    expect(expandButton).toHaveAttribute("aria-expanded", "false");
    expect(container.querySelector(".long-message-collapse")).toHaveClass("is-collapsed");
    const content = container.querySelector(".long-message-content") as HTMLElement;
    const collapsedHeight = Number.parseInt(content.style.maxHeight, 10);
    expect(collapsedHeight).toBeGreaterThan(0);

    await user.click(expandButton);

    const collapseButton = screen.getByRole("button", { name: "收起" });
    expect(collapseButton).toHaveAttribute("aria-expanded", "true");
    expect(container.querySelector(".long-message-collapse")).toHaveClass("is-expanded");
    const expandedHeight = Number.parseInt(content.style.maxHeight, 10);
    expect(expandedHeight).toBeGreaterThan(collapsedHeight);

    await user.click(collapseButton);

    expect(screen.getByRole("button", { name: "展开全文" })).toHaveAttribute("aria-expanded", "false");
    restoreLayout();
  });

  it("does not collapse image-only markdown messages", async () => {
    const restoreLayout = mockLongMessageLayout();

    const { container } = render(
      <MessageContent content={"![](https://example.com/image.png)"} enableLongMessageCollapse t={longMessageT} />,
    );

    expect(screen.queryByRole("button", { name: "展开全文" })).not.toBeInTheDocument();
    expect(container.querySelector("img")).toHaveAttribute("src", "https://example.com/image.png");
    restoreLayout();
  });

  it("does not collapse markdown messages that include images", () => {
    const restoreLayout = mockLongMessageLayout();
    const content = `这是一条带图片的消息，图片消息不参与长文本折叠规则。\n\n![diagram](https://example.com/image.png)\n\n${"图片说明文字".repeat(40)}`;

    const { container } = render(<MessageContent content={content} enableLongMessageCollapse t={longMessageT} />);

    expect(screen.queryByRole("button", { name: "展开全文" })).not.toBeInTheDocument();
    expect(container.querySelector(".long-message-collapse")).toBeNull();
    expect(container.querySelector("img")).toHaveAttribute("src", "https://example.com/image.png");
    restoreLayout();
  });

  it("collapses long code blocks as part of the whole message", async () => {
    const restoreLayout = mockLongMessageLayout();
    const code = Array.from({ length: 24 }, (_, index) => `const item${index} = "line ${index}";`).join("\n");

    const { container } = render(
      <MessageContent
        content={`代码块需要参与整体折叠：\n\n\`\`\`ts\n${code}\n\`\`\``}
        enableLongMessageCollapse
        t={longMessageT}
      />,
    );

    expect(await screen.findByRole("button", { name: "展开全文" })).toBeInTheDocument();
    expect(container.querySelector(".long-message-collapse")).toHaveClass("is-collapsed");
    expect(container.querySelector("pre")).toBeInTheDocument();
    restoreLayout();
  });

  it("does not collapse long messages unless explicitly enabled", () => {
    const restoreLayout = mockLongMessageLayout();
    const longText = "收到的长消息默认保持完整展示，只有当前用户自己发送的长文本才需要折叠。".repeat(12);

    const { container } = render(<MessageContent content={longText} t={longMessageT} />);

    expect(screen.queryByRole("button", { name: "展开全文" })).not.toBeInTheDocument();
    expect(container.querySelector(".long-message-collapse")).toBeNull();
    restoreLayout();
  });

  it("supports externally controlled long message expansion state", async () => {
    const restoreLayout = mockLongMessageLayout();
    const user = userEvent.setup();
    const onExpandedChange = vi.fn();
    const longText = "用户自己发送的长消息展开状态需要在当前会话内保持。".repeat(12);

    const { container, rerender } = render(
      <MessageContent
        content={longText}
        enableLongMessageCollapse
        longMessageExpanded={false}
        onLongMessageExpandedChange={onExpandedChange}
        t={longMessageT}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "展开全文" }));

    expect(onExpandedChange).toHaveBeenCalledWith(true);

    rerender(
      <MessageContent
        content={longText}
        enableLongMessageCollapse
        longMessageExpanded
        onLongMessageExpandedChange={onExpandedChange}
        t={longMessageT}
      />,
    );

    expect(container.querySelector(".long-message-collapse")).toHaveClass("is-expanded");
    expect(screen.getByRole("button", { name: "收起" })).toHaveAttribute("aria-expanded", "true");
    restoreLayout();
  });

  it("renders canonical slash command prefixes with the prompt outside the XML element", () => {
    render(
      <MessageContent
        content={
          '<slash-command name="use-skill" arg="skill-creator"></slash-command> create <review> skill <img src=x onerror=alert(1)>'
        }
      />,
    );

    expect(screen.getByText("/skill-creator")).toHaveClass("message-slash-token");
    expect(screen.getByText("create <review> skill <img src=x onerror=alert(1)>")).toHaveClass("slash-command-body");
    expect(document.querySelector("slash-command")).toBeNull();
    expect(document.querySelector("img")).toBeNull();
  });

  it("renders canonical new conversation commands as slash text", () => {
    render(
      <MessageContent content={'<slash-command name="new" arg="conversation"></slash-command> reset before rebuild'} />,
    );

    expect(screen.getByText("/new")).toHaveClass("message-slash-token");
    expect(screen.getByText("reset before rebuild")).toHaveClass("slash-command-body");
    expect(document.querySelector("slash-command")).toBeNull();
  });

  it("renders canonical slash command mentions in history with mention and slash highlight classes", () => {
    render(
      <MessageContent
        content={'<slash-command name="use-skill" arg="basics"></slash-command> <at user_id="u-manager">manager</at>'}
      />,
    );

    expect(screen.getByText("/basics")).toHaveClass("message-slash-token");
    expect(screen.getByText("@manager")).toHaveClass("message-mention");
  });

  it("renders action cards and invokes the selected action with message context", async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    const message = { id: "message-1" };

    render(
      <MessageContent
        content={JSON.stringify({
          actions: [
            { id: ACTION_REBUILD_MANAGER, label: "Rebuild manager", style: "danger" },
            { id: "ignored-action", label: "Ignored" },
          ],
          badge: "Required",
          title: "Manager needs rebuild",
          type: CSGCLAW_ACTION_CARD_TYPE,
        })}
        message={message}
        onAction={onAction}
      />,
    );

    expect(screen.getByText("Manager needs rebuild")).toBeInTheDocument();
    expect(screen.getByText("Required")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ignored" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Rebuild manager" }));

    expect(onAction).toHaveBeenCalledTimes(1);
    expect(onAction).toHaveBeenCalledWith(
      expect.objectContaining({
        id: ACTION_REBUILD_MANAGER,
        label: "Rebuild manager",
        style: "danger",
      }),
      message,
    );
  });

  it("disables the busy action and shows action errors for the matching message", () => {
    render(
      <MessageContent
        actionBusy="message-1:rebuild-manager"
        actionError={{ key: "message-1:rebuild-manager", message: "Rebuild failed" }}
        content={JSON.stringify({
          actions: [{ id: ACTION_REBUILD_MANAGER, label: "Rebuild manager" }],
          title: "Manager needs rebuild",
          type: CSGCLAW_ACTION_CARD_TYPE,
        })}
        message={{ id: "message-1" }}
        onAction={vi.fn()}
      />,
    );

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
    expect(button).toHaveTextContent("...");
    expect(screen.getByText("Rebuild failed")).toBeInTheDocument();
  });

  it("renders notifier cards as structured content", () => {
    render(
      <MessageContent
        content={JSON.stringify({
          type: CSGCLAW_NOTIFY_CARD_TYPE,
          title: "GitHub · Pull request",
          badge: "opened",
          link: "https://github.com/acme/app/pull/1",
          meta: [{ label: "Branch", value: "feature -> main" }],
          raw: '{"action":"opened"}',
        })}
      />,
    );

    expect(screen.getByText("GitHub · Pull request")).toBeInTheDocument();
    expect(screen.getByText("opened")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open link" })).toHaveAttribute(
      "href",
      "https://github.com/acme/app/pull/1",
    );
    expect(screen.getByText("Branch")).toBeInTheDocument();
    expect(screen.getByText("feature -> main")).toBeInTheDocument();
  });

  it("keeps PicoClaw legacy tool feedback in markdown form", () => {
    render(
      <MessageContent
        content={`🔧 \`read_file\`
\`\`\`json
{"path":"README.md"}
\`\`\``}
      />,
    );

    expect(screen.getByText("read_file")).toBeInTheDocument();
    expect(screen.queryByText("查看原始 JSON · 1 个字段")).not.toBeInTheDocument();
  });

  it("renders Codex tool activity in the structured tool output style", () => {
    render(
      <MessageContent
        content={JSON.stringify({
          type: CSGCLAW_AGENT_ACTIVITY_TYPE,
          channel: "csgclaw",
          sender: "u-codex",
          content: {
            msgtype: AgentActivityMsgTypes.tool,
            body: "Running tool",
            tool: {
              id: "tool-1",
              input_summary: '{"cmd":"go test ./internal/runtime/codex"}',
              kind: "execute",
              status: "running",
              title: "Run shell command",
            },
          },
        })}
      />,
    );

    expect(screen.getByText("exec")).toBeInTheDocument();
    expect(screen.queryByText("Run shell command")).not.toBeInTheDocument();
    expect(screen.queryByText("Running")).not.toBeInTheDocument();
    expect(screen.getByText(/go test/)).toBeInTheDocument();
  });

  it("renders Codex permission buttons and posts decisions", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "perm-1", status: "allowed" }), {
        headers: { "Content-Type": "application/json" },
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(
      <MessageContent
        content={JSON.stringify({
          type: CSGCLAW_AGENT_ACTIVITY_TYPE,
          channel: "csgclaw",
          sender: "u-codex",
          content: {
            msgtype: AgentActivityMsgTypes.action,
            body: "Codex wants permission",
            action: {
              id: "perm-1",
              kind: "permission",
              options: [
                { id: "once", kind: "allow_once", label: "Allow once" },
                { id: "always", kind: "allow_always", label: "Allow always" },
                { id: "reject", kind: "reject_once", label: "Reject" },
              ],
              status: "pending",
              title: "Run shell command",
            },
          },
        })}
      />,
    );

    expect(screen.getByText("Permission request")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Allow always \(this agent\)/ })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Allow once/ }));

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/channels/csgclaw/activities/perm-1:decide",
      expect.objectContaining({
        body: JSON.stringify({ option_id: "once" }),
        method: "POST",
      }),
    );
    expect(await screen.findByText("Allowed")).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
});
