import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { AgentActivityPanel } from "@/pages/AgentPage/components/AgentDetailPane/AgentActivityPanel";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";
import type { IMServerEvent } from "@/models/conversations";

const subscribeIMEventsMock = vi.fn();

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: (handler: (payload: IMServerEvent) => void) => subscribeIMEventsMock(handler),
}));

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
  const result = render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
  return {
    ...result,
    queryClient,
    rerenderWithClient: (nextUI: ReactElement) =>
      result.rerender(<QueryClientProvider client={queryClient}>{nextUI}</QueryClientProvider>),
  };
}

describe("AgentActivityPanel", () => {
  afterEach(() => {
    subscribeIMEventsMock.mockReset();
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

  it("allows message rows to expand and show the full message", async () => {
    const user = userEvent.setup();
    const fullMessage = [
      "有进展了！项目目录里已经有之前的开发成果：已有文件 index.html、TEST_REPORT.md、tasks.json。",
      "dev 正在继续开发，qa 等待测试，manager 会持续同步项目状态。",
      "这段内容用于确认活动记录中的普通 message 也可以通过左侧展开按钮查看完整正文。",
      "补充上下文：开发任务、测试任务、房间同步、文件状态和后续安排都会被折叠到活动记录摘要里。",
      "补充上下文：这部分只是为了让摘要长度超过单行预览阈值，避免尾部内容直接出现在列表中。",
      "最后一段：需要打开完整消息才能看到。",
    ].join(" ");
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "reply-long",
            sender_id: "manager",
            created_at: "2026-07-01T10:00:00Z",
            content: fullMessage,
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
        locale="zh-CN"
        rooms={[
          {
            id: "room-1",
            title: "Dev group",
            members: ["manager", "u-admin"],
            messages: [{ id: "root", sender_id: "u-admin", content: "please inspect" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findByText("message")).toBeInTheDocument();
    expect(screen.queryByText(/最后一段/)).not.toBeInTheDocument();

    const row = screen.getByRole("listitem");
    const expandButton = within(row).getByRole("button", { name: /有进展了/ });
    expect(expandButton).toHaveAttribute("aria-expanded", "false");

    await user.click(expandButton);

    expect(expandButton).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(/最后一段：需要打开完整消息才能看到。/)).toBeInTheDocument();
  });

  it("does not refetch full room history when the latest room message changes", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify([]), { headers: { "Content-Type": "application/json" }, status: 200 }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const item = {
      id: "agent-manager",
      name: "manager",
      role: "manager",
      participants: [{ channel: "csgclaw", channel_user_ref: "manager", id: "manager" }],
    };
    const baseRoom = {
      id: "room-1",
      title: "Dev group",
      members: ["manager", "u-admin"],
      messages: [{ id: "root", sender_id: "u-admin", content: "please inspect" }],
    };
    const view = renderWithClient(<AgentActivityPanel item={item} locale="en" rooms={[baseRoom]} t={t} />);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    view.rerenderWithClient(
      <AgentActivityPanel
        item={item}
        locale="en"
        rooms={[
          {
            ...baseRoom,
            messages: [
              ...baseRoom.messages,
              {
                id: "user-latest",
                sender_id: "u-admin",
                created_at: "2026-07-01T10:00:01Z",
                content: "Any update?",
              },
            ],
          },
        ]}
        t={t}
      />,
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  });

  it("merges a new agent room message into the activity cache without refetching", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify([]), { headers: { "Content-Type": "application/json" }, status: 200 }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const item = {
      id: "agent-manager",
      name: "manager",
      role: "manager",
      participants: [{ channel: "csgclaw", channel_user_ref: "manager", id: "manager" }],
    };
    const baseRoom = {
      id: "room-1",
      title: "Dev group",
      members: ["manager", "u-admin"],
      messages: [{ id: "root", sender_id: "u-admin", content: "please inspect" }],
    };
    const view = renderWithClient(<AgentActivityPanel item={item} locale="en" rooms={[baseRoom]} t={t} />);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    view.rerenderWithClient(
      <AgentActivityPanel
        item={item}
        locale="en"
        rooms={[
          {
            ...baseRoom,
            messages: [
              ...baseRoom.messages,
              {
                id: "agent-live",
                sender_id: "manager",
                created_at: "2026-07-01T10:00:02Z",
                content: "Live activity arrived.",
              },
            ],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findByText("Live activity arrived.")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("deduplicates an agent message seen in both history and realtime room state", async () => {
    const historyMessage = {
      id: "reply-1",
      sender_id: "manager",
      created_at: "2026-07-01T10:00:00Z",
      content: "Already loaded from history.",
    };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([historyMessage]), {
        headers: { "Content-Type": "application/json" },
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const item = {
      id: "agent-manager",
      name: "manager",
      role: "manager",
      participants: [{ channel: "csgclaw", channel_user_ref: "manager", id: "manager" }],
    };
    const baseRoom = {
      id: "room-1",
      title: "Dev group",
      members: ["manager", "u-admin"],
      messages: [{ id: "root", sender_id: "u-admin", content: "please inspect" }],
    };
    const view = renderWithClient(<AgentActivityPanel item={item} locale="en" rooms={[baseRoom]} t={t} />);

    expect(await screen.findByText("Already loaded from history.")).toBeInTheDocument();

    view.rerenderWithClient(
      <AgentActivityPanel
        item={item}
        locale="en"
        rooms={[{ ...baseRoom, messages: [...baseRoom.messages, historyMessage] }]}
        t={t}
      />,
    );

    await waitFor(() => expect(screen.getAllByRole("listitem")).toHaveLength(1));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("merges thread reply SSE messages without refetching full room history", async () => {
    let eventHandler: ((payload: IMServerEvent) => void) | null = null;
    subscribeIMEventsMock.mockImplementation((handler: (payload: IMServerEvent) => void) => {
      eventHandler = handler;
      return () => {
        eventHandler = null;
      };
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify([]), { headers: { "Content-Type": "application/json" }, status: 200 }),
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

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(eventHandler).toBeTruthy();

    act(() => {
      eventHandler?.({
        type: "message.created",
        room_id: "room-1",
        message: {
          id: "thread-reply-1",
          sender_id: "manager",
          created_at: "2026-07-01T10:00:03Z",
          content: "Thread reply activity.",
          relates_to: { event_id: "root", rel_type: "m.thread" },
        },
      });
    });

    expect(await screen.findByText("Thread reply activity.")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
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
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "search-result",
            sender_id: "openclaw-weather",
            created_at: "2026-07-01T10:00:01Z",
            content: '🔎 Web Search: for "上海天气" (top 5) {"query":"上海天气","count":5,"status":"ok"}',
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "reply-1",
            sender_id: "openclaw-weather",
            created_at: "2026-07-01T10:00:02Z",
            content: "上海这周多云。",
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

  it("coalesces OpenClaw tool item, command item, and command output for one exec call", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "tool-start",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:00Z",
            content: JSON.stringify({
              type: CSGCLAW_AGENT_ACTIVITY_TYPE,
              channel: "openclaw",
              sender: "openclaw",
              content: {
                msgtype: AgentActivityMsgTypes.tool,
                body: "exec csgclaw-cli --version · running",
                tool: {
                  id: "tool:call-version",
                  item_id: "tool:call-version",
                  input_summary: "csgclaw-cli --version",
                  kind: "exec",
                  phase: "start",
                  status: "running",
                  title: "exec csgclaw-cli --version",
                },
              },
            }),
            metadata: { openclaw: { activity_kind: "item", delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "plain-command",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:00.500Z",
            content: "🛠️ csgclaw-cli --version",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "command-end",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:01Z",
            content: JSON.stringify({
              type: CSGCLAW_AGENT_ACTIVITY_TYPE,
              channel: "openclaw",
              sender: "openclaw",
              content: {
                msgtype: AgentActivityMsgTypes.tool,
                body: "csgclaw-cli available, version v0.3.16.",
                tool: {
                  id: "command:call-version",
                  command: "command csgclaw-cli --version",
                  item_id: "command:call-version",
                  input_summary: "csgclaw-cli --version",
                  kind: "exec",
                  output_summary: "csgclaw-cli available, version v0.3.16.",
                  phase: "end",
                  status: "completed",
                  title: "command csgclaw-cli --version",
                },
              },
            }),
            metadata: { openclaw: { activity_kind: "item", delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "command-output",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:02Z",
            content: JSON.stringify({
              type: CSGCLAW_AGENT_ACTIVITY_TYPE,
              channel: "openclaw",
              sender: "openclaw",
              content: {
                msgtype: AgentActivityMsgTypes.tool,
                body: "csgclaw-cli available, version v0.3.16.",
                tool: {
                  id: "command:call-version",
                  command: "command csgclaw-cli --version",
                  item_id: "command:call-version",
                  input_summary: "csgclaw-cli --version",
                  tool_call_id: "call-version",
                  kind: "exec",
                  output: "csgclaw-cli available, version v0.3.16.",
                  phase: "end",
                  status: "completed",
                  title: "command csgclaw-cli --version",
                },
              },
            }),
            metadata: { openclaw: { activity_kind: "command_output", delivery_kind: "tool", request_id: "msg-user" } },
          },
        ]),
        { headers: { "Content-Type": "application/json" }, status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(
      <AgentActivityPanel
        item={{
          id: "agent-dev-openclaw",
          name: "dev-openclaw",
          role: "worker",
          participants: [{ channel: "csgclaw", channel_user_ref: "dev-openclaw", id: "dev-openclaw" }],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-dev-openclaw",
            title: "dev-openclaw",
            members: ["dev-openclaw", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "运行一下 csgclaw cli" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(1);
    expect(screen.getByText("1 events")).toBeInTheDocument();
    expect(screen.getByText("csgclaw-cli --version")).toBeInTheDocument();
    expect(screen.queryByText("command csgclaw-cli --version")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /csgclaw-cli --version/ }));

    expect(screen.getByText("Result")).toBeInTheDocument();
    expect(screen.getByText("csgclaw-cli available, version v0.3.16.")).toBeInTheDocument();
  });

  it("deduplicates equivalent OpenClaw command wrapper lines for the same command", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "help-wrapper",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:00Z",
            content: "command csgclaw-cli --help",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "help-command",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:00Z",
            content: "csgclaw-cli --help",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "version-wrapper",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:05Z",
            content: "command csgclaw-cli --version",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "version-command",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:05Z",
            content: "csgclaw-cli --version",
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
          id: "agent-dev-openclaw",
          name: "dev-openclaw",
          role: "worker",
          participants: [{ channel: "csgclaw", channel_user_ref: "dev-openclaw", id: "dev-openclaw" }],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-dev-openclaw",
            title: "dev-openclaw",
            members: ["dev-openclaw", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "运行一下 csgclaw cli" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(2);
    expect(screen.getByText("2 events")).toBeInTheDocument();
    expect(screen.getByText("csgclaw-cli --help")).toBeInTheDocument();
    expect(screen.getByText("csgclaw-cli --version")).toBeInTheDocument();
    expect(screen.queryByText("command csgclaw-cli --help")).not.toBeInTheDocument();
    expect(screen.queryByText("command csgclaw-cli --version")).not.toBeInTheDocument();
  });

  it("deduplicates repeated OpenClaw command deliveries for the same request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "version-start",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:00Z",
            content: "csgclaw-cli --version",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "version-repeat",
            sender_id: "dev-openclaw",
            created_at: "2026-07-01T10:00:04Z",
            content: "csgclaw-cli --version",
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
          id: "agent-dev-openclaw",
          name: "dev-openclaw",
          role: "worker",
          participants: [{ channel: "csgclaw", channel_user_ref: "dev-openclaw", id: "dev-openclaw" }],
        }}
        locale="zh-CN"
        rooms={[
          {
            id: "room-dev-openclaw",
            title: "dev-openclaw",
            members: ["dev-openclaw", "admin"],
            messages: [{ id: "root", sender_id: "admin", content: "运行一下 csgclaw cli" }],
          },
        ]}
        t={t}
      />,
    );

    expect(await screen.findAllByText("exec_command")).toHaveLength(1);
    expect(screen.getByText("1 events")).toBeInTheDocument();
    expect(screen.getByText("csgclaw-cli --version")).toBeInTheDocument();
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
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "memory-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:01Z",
            content: '🧠 Memory Search: GitLab issues user context {"results":[]}',
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
          },
          {
            id: "run-1",
            sender_id: "glab-opencsg",
            created_at: "2026-07-01T10:00:02Z",
            content: "🛠️ run cd → run python3 scripts/ensure_gitlab_auth.py",
            metadata: { openclaw: { delivery_kind: "tool", request_id: "msg-user" } },
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
