import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { AgentActivityPanel } from "@/pages/AgentPage/components/AgentDetailPane/AgentActivityPanel";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";

const labels: Record<string, string> = {
  agentActivityAction: "Action",
  agentActivityDescription: "Activity description",
  agentActivityEmpty: "No activity",
  agentActivityLoadFailed: "Activity failed",
  agentActivityLoading: "Loading activity",
  agentActivityNoRooms: "No rooms",
  agentActivityNotice: "Notice",
  agentActivityRefresh: "Refresh",
  agentActivityReply: "Reply",
  agentActivityTitle: "Activity",
  agentActivityTool: "Tool",
  agentActivityCommand: "Command",
  agentActivityChronological: "Chronological",
  agentActivityClearFilters: "Clear filters",
  agentActivityResult: "Result",
  agentActivityEventsCount: "{count} events",
  agentActivityFilter: "Filter",
  agentActivityFilterActiveCount: "{count} filters",
  agentActivityFilteredEventsCount: "{shown}/{total} events",
  agentActivityInput: "Input",
  agentActivityMessageFilter: "Messages",
  agentActivityNewestFirst: "Newest first",
  agentActivityNoFilteredResults: "No matching activity",
  agentActivityOtherFilter: "Other",
  agentActivityRemoveFilter: "Remove filter {label}",
  agentActivitySelectedFiltersLabel: "Selected filters",
  agentActivitySortLabel: "Activity sort",
  agentActivitySummaryLabel: "Activity summary",
  agentActivityTimelineLabel: "Activity timeline",
  agentActivityTimelineSegment: "{label} #{index} {time}",
  agentActivityToolCallsCount: "{count} tool calls",
};

function t(key: string, params: Record<string, string | number> = {}): string {
  return (labels[key] ?? key).replace(/\{(\w+)}/g, (_, name) => String(params[name] ?? ""));
}

function renderWithClient(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

describe("AgentActivityPanel", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads thread-inclusive room messages and shows tool activity plus text replies", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "tool-1",
            sender_id: "manager",
            created_at: "2026-07-01T10:00:00Z",
            content: JSON.stringify({
              type: CSGCLAW_AGENT_ACTIVITY_TYPE,
              channel: "csgclaw",
              sender: "manager",
              content: {
                msgtype: AgentActivityMsgTypes.tool,
                body: "Tool running",
                tool: {
                  id: "exec-1",
                  input_summary: '{"cmd":"pwd"}',
                  output_summary: '{"output":"/workspace"}',
                  kind: "execute",
                  status: "completed",
                  title: "Run shell command",
                },
              },
            }),
          },
          {
            id: "reply-1",
            sender_id: "manager",
            created_at: "2026-07-01T10:00:02Z",
            content: "Done with the workspace check.",
          },
          {
            id: "legacy-tool-1",
            sender_id: "manager",
            created_at: "2026-07-01T10:00:03Z",
            content: `🔧 \`exec\`
\`\`\`json
{"command":"ls web/app","output":"src\\ntests"}
\`\`\``,
          },
        ]),
        { headers: { "Content-Type": "application/json" }, status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(
      <AgentActivityPanel
        item={{
          id: "agent-manager",
          name: "manager",
          role: "manager",
          participants: [{ channel: "csgclaw", channel_user_ref: "manager", id: "manager" }],
        }}
        locale="en"
        rooms={[
          {
            id: "room-1",
            title: "Dev group",
            members: ["manager", "u-admin"],
            messages: [{ id: "root", sender_id: "u-admin", content: "please inspect" }],
            threads: [{ root_message_id: "root" }],
          },
        ]}
        t={t}
      />,
    );

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "api/v1/messages?room_id=room-1&include_thread_replies=true",
        expect.anything(),
      ),
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(2);
    expect(screen.getByText(/pwd/)).toBeInTheDocument();
    expect(screen.queryByText("/workspace")).not.toBeInTheDocument();
    expect(screen.getByText("Done with the workspace check.")).toBeInTheDocument();
    expect(screen.getAllByText("Dev group")).toHaveLength(3);
    expect(screen.getByText("3s")).toBeInTheDocument();
    expect(screen.getByText("2 tool calls")).toBeInTheDocument();
    expect(screen.getByText("3 events")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Newest first/ })).toHaveAttribute("aria-pressed", "true");
    let rows = await screen.findAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(within(rows[0]).getByText(/ls web\/app/)).toBeInTheDocument();
    expect(within(rows[0]).getByText("#3")).toBeInTheDocument();
    expect(within(rows[1]).getByText("Done with the workspace check.")).toBeInTheDocument();
    expect(within(rows[1]).getByText("#2")).toBeInTheDocument();
    expect(within(rows[2]).getByText(/pwd/)).toBeInTheDocument();
    expect(within(rows[2]).getByText("#1")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Chronological/ }));
    expect(screen.getByRole("button", { name: /Chronological/ })).toHaveAttribute("aria-pressed", "true");
    rows = await screen.findAllByRole("listitem");
    expect(within(rows[0]).getByText(/pwd/)).toBeInTheDocument();
    expect(within(rows[0]).getByText("#1")).toBeInTheDocument();
    expect(within(rows[2]).getByText(/ls web\/app/)).toBeInTheDocument();
    expect(within(rows[2]).getByText("#3")).toBeInTheDocument();

    const timeline = screen.getByRole("navigation", { name: "Activity timeline" });
    const segments = within(timeline).getAllByRole("button");
    await user.click(segments[0]);
    expect(rows[0]).toHaveClass("selected");

    await user.click(screen.getByRole("button", { name: /pwd/ }));
    expect(screen.getByText("Command")).toBeInTheDocument();
    expect(screen.getByText("Result")).toBeInTheDocument();
    expect(screen.getByText("/workspace")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Filter/ }));
    await user.click(screen.getByRole("menuitemcheckbox", { name: "Tool:exec (1)" }));
    await waitFor(() => expect(screen.getAllByRole("listitem")).toHaveLength(1));
    expect(screen.getByText("1/3 events")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Remove filter Tool:exec/ })).toBeInTheDocument();
    expect(screen.queryByText("Done with the workspace check.")).not.toBeInTheDocument();

    expect(screen.queryByText(/src\\ntests/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /ls web\/app/ }));
    expect(screen.getByText(/src/)).toBeInTheDocument();
    expect(screen.getByText(/tests/)).toBeInTheDocument();
  });

  it("folds OpenClaw plain command output under the command row", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "search-start",
            sender_id: "openclaw-weather",
            created_at: "2026-07-01T10:00:00Z",
            content: '🔎 Web Search: for "上海天气" (top 5)',
          },
          {
            id: "search-result",
            sender_id: "openclaw-weather",
            created_at: "2026-07-01T10:00:01Z",
            content: '🔎 Web Search: for "上海天气" (top 5) {"query":"上海天气","count":5,"status":"ok"}',
          },
          {
            id: "reply-1",
            sender_id: "openclaw-weather",
            created_at: "2026-07-01T10:00:02Z",
            content: "上海这周多云。",
          },
        ]),
        { headers: { "Content-Type": "application/json" }, status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(
      <AgentActivityPanel
        item={{
          id: "agent-openclaw-weather",
          name: "openclaw-weather",
          role: "worker",
          participants: [
            {
              channel: "csgclaw",
              channel_user_ref: "openclaw-weather",
              id: "openclaw-weather",
            },
          ],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-weather",
            title: "天气查询小队",
            members: ["openclaw-weather", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "查天气" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(1);
    expect(screen.getByText("message")).toBeInTheDocument();
    expect(screen.getByText('Web Search: for "上海天气" (top 5)')).toBeInTheDocument();
    expect(screen.queryByText(/"status": "ok"/)).not.toBeInTheDocument();

    const weatherCommandRow = screen
      .getAllByRole("listitem")
      .find((row) => within(row).queryByText('Web Search: for "上海天气" (top 5)'));
    expect(weatherCommandRow).toBeTruthy();
    await user.click(within(weatherCommandRow as HTMLElement).getByRole("button", { name: /Web Search/ }));

    expect(screen.getByText("Result")).toBeInTheDocument();
    expect(screen.getByText(/"status": "ok"/)).toBeInTheDocument();
    expect(screen.getByText("上海这周多云。")).toBeInTheDocument();
  });

  it("folds OpenClaw metadata tool result text under the command row", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "script-start",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:00Z",
            content: "run python3 inline script",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "script-result",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:01Z",
            content: "run python3 inline script\nok",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "reply-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:02Z",
            content: "已完成。",
            metadata: { openclaw: { delivery_kind: "final", request_id: "msg-user" } },
          },
        ]),
        { headers: { "Content-Type": "application/json" }, status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(
      <AgentActivityPanel
        item={{
          id: "agent-glab-opencsg",
          name: "glab-opencsg",
          role: "worker",
          participants: [{ channel: "csgclaw", channel_user_ref: "glab-opencsg", id: "glab-opencsg" }],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-glab",
            title: "glab-opencsg",
            members: ["glab-opencsg", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "查一下 issue" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(1);
    expect(screen.getByText("2 events")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /run python3 inline script/ }));
    expect(screen.getByText("Result")).toBeInTheDocument();
    expect(screen.getByText("ok")).toBeInTheDocument();
    expect(screen.getByText("已完成。")).toBeInTheDocument();
  });

  it("allows OpenClaw command-only rows to expand and show the full command", async () => {
    const user = userEvent.setup();
    const longCommand =
      'export GLAB_TELEMETRY_DISABLED=1 REPO="product/agentichub/requirements" IID=2768 ACTION=issue_update ASSIGNEE=jared';
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "command-only",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:00Z",
            content: longCommand,
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
        ]),
        { headers: { "Content-Type": "application/json" }, status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(
      <AgentActivityPanel
        item={{
          id: "agent-glab-opencsg",
          name: "glab-opencsg",
          role: "worker",
          participants: [{ channel: "csgclaw", channel_user_ref: "glab-opencsg", id: "glab-opencsg" }],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-glab",
            title: "glab-opencsg",
            members: ["glab-opencsg", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "更新 issue" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(1);
    const row = screen.getByRole("listitem");
    const rowButton = within(row).getByRole("button", { name: /export GLAB_TELEMETRY_DISABLED/ });
    expect(rowButton).toHaveAttribute("aria-expanded", "false");

    await user.click(rowButton);

    expect(rowButton).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Command")).toBeInTheDocument();
    expect(screen.getAllByText(longCommand).length).toBeGreaterThanOrEqual(1);
  });

  it("merges OpenClaw structured command output activity into one expandable command row", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "command-start",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:00Z",
            content: JSON.stringify({
              type: CSGCLAW_AGENT_ACTIVITY_TYPE,
              channel: "openclaw",
              sender: "openclaw",
              content: {
                msgtype: AgentActivityMsgTypes.tool,
                body: "run python3 inline script · running",
                tool: {
                  id: "command-tool-1",
                  command: "run python3 inline script",
                  kind: "exec",
                  phase: "start",
                  status: "running",
                  title: "run python3 inline script",
                },
              },
            }),
            metadata: { openclaw: { activity_kind: "item", delivery_kind: "tool" } },
          },
          {
            id: "command-end",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:01Z",
            content: JSON.stringify({
              type: CSGCLAW_AGENT_ACTIVITY_TYPE,
              channel: "openclaw",
              sender: "openclaw",
              content: {
                msgtype: AgentActivityMsgTypes.tool,
                body: "created issue",
                tool: {
                  id: "command-tool-1",
                  command: "run python3 inline script",
                  cwd: "/workspace",
                  duration_ms: 99,
                  exit_code: 0,
                  kind: "exec",
                  output: "created issue",
                  phase: "end",
                  status: "completed",
                  title: "run python3 inline script",
                  tool_call_id: "tool-call-1",
                },
              },
            }),
            metadata: { openclaw: { activity_kind: "command_output", delivery_kind: "tool" } },
          },
        ]),
        { headers: { "Content-Type": "application/json" }, status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(
      <AgentActivityPanel
        item={{
          id: "agent-glab-opencsg",
          name: "glab-opencsg",
          role: "worker",
          participants: [{ channel: "csgclaw", channel_user_ref: "glab-opencsg", id: "glab-opencsg" }],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-glab",
            title: "glab-opencsg",
            members: ["glab-opencsg", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "创建 issue" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(1);
    expect(screen.getByText("1 events")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /run python3 inline script/ }));
    expect(screen.getByText("Result")).toBeInTheDocument();
    expect(screen.getByText("created issue")).toBeInTheDocument();
  });

  it("shows OpenClaw progress lines as command activity instead of messages", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "read-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:00Z",
            content: "📖 Read: from ~/.openclaw/workspace/skills/gitlab/SKILL.md",
          },
          {
            id: "memory-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:01Z",
            content: '🧠 Memory Search: GitLab issues user context {"results":[]}',
          },
          {
            id: "run-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:02Z",
            content: "🛠️ run cd → run python3 scripts/ensure_gitlab_auth.py",
          },
          {
            id: "metadata-tool-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:03Z",
            content: "Tool progress without a legacy marker",
            metadata: {
              openclaw: {
                delivery_kind: "tool",
                request_id: "msg-user",
              },
            },
          },
          {
            id: "reply-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:04Z",
            content: "当前 GitLab 环境缺少有效凭证，无法查询。",
            metadata: {
              openclaw: {
                delivery_kind: "final",
                request_id: "msg-user",
              },
            },
          },
        ]),
        { headers: { "Content-Type": "application/json" }, status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(
      <AgentActivityPanel
        item={{
          id: "agent-glab-opencsg",
          name: "glab-opencsg",
          role: "worker",
          participants: [
            {
              channel: "csgclaw",
              channel_user_ref: "glab-opencsg",
              id: "glab-opencsg",
            },
          ],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-glab",
            title: "glab-opencsg",
            members: ["glab-opencsg", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "查一下 issue" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(4);
    expect(screen.getByText("message")).toBeInTheDocument();
    expect(screen.getByText("4 tool calls")).toBeInTheDocument();
    expect(screen.getByText("5 events")).toBeInTheDocument();

    const rows = await screen.findAllByRole("listitem");
    const readRow = rows.find((row) => within(row).queryByText(/Read: from/));
    expect(readRow).toBeTruthy();
    expect(within(readRow as HTMLElement).getByText("exec_command")).toBeInTheDocument();
    expect(within(readRow as HTMLElement).queryByText("message")).not.toBeInTheDocument();
    const metadataRow = rows.find((row) => within(row).queryByText("Tool progress without a legacy marker"));
    expect(metadataRow).toBeTruthy();
    expect(within(metadataRow as HTMLElement).getByText("exec_command")).toBeInTheDocument();
  });
});
