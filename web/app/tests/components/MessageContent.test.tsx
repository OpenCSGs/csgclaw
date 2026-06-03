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

describe("MessageContent", () => {
  it("renders sanitized markdown links with safe external-link attributes", () => {
    render(<MessageContent content={'Open [docs](https://example.com/docs)<script>alert("xss")</script>'} />);

    const link = screen.getByRole("link", { name: "docs" });
    expect(link).toHaveAttribute("href", "https://example.com/docs");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(document.querySelector("script")).toBeNull();
  });

  it("renders canonical slash command prefixes with the prompt outside the XML element", () => {
    render(
      <MessageContent
        content={
          '<slash-command name="use-skill" arg="skill-creator"></slash-command> create <review> skill <img src=x onerror=alert(1)>'
        }
      />,
    );

    expect(screen.getByText("/skill-creator create <review> skill <img src=x onerror=alert(1)>")).toBeInTheDocument();
    expect(document.querySelector("slash-command")).toBeNull();
    expect(document.querySelector("img")).toBeNull();
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
