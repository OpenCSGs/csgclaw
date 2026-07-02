import {
  isOpenClawToolDeliveryMessage,
  isToolActivityMessage,
  openClawDeliveryKind,
  openClawToolCallID,
  parseAgentActivity,
  parseOpenClawDeliveryCommand,
  parsePlainAgentCommand,
} from "@/models/agentActivity";
import { AgentActivityKinds, AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";

describe("agent activity model", () => {
  it("parses tool activity payloads", () => {
    const activity = parseAgentActivity(
      JSON.stringify({
        type: CSGCLAW_AGENT_ACTIVITY_TYPE,
        version: 1,
        channel: "csgclaw",
        event_id: "act-1",
        room_id: "room-1",
        sender: "u-codex",
        origin_server_ts: 1779259200000,
        content: {
          msgtype: AgentActivityMsgTypes.tool,
          body: "Running tool",
          tool: {
            id: "tool-1",
            kind: "execute",
            status: "running",
            title: "Run shell command",
          },
        },
      }),
    );

    expect(activity?.content.tool).toMatchObject({
      id: "tool-1",
      kind: "execute",
      status: "running",
      title: "Run shell command",
    });
    expect(activity?.channel).toBe("csgclaw");
    expect(activity?.version).toBe(1);
  });

  it("parses OpenClaw structured command output fields", () => {
    const activity = parseAgentActivity(
      JSON.stringify({
        type: CSGCLAW_AGENT_ACTIVITY_TYPE,
        content: {
          msgtype: AgentActivityMsgTypes.tool,
          body: "ok",
          tool: {
            id: "command-tool-1",
            command: "run python3 inline script",
            cwd: "/workspace",
            duration_ms: 42,
            exit_code: 0,
            item_id: "command-tool-1",
            output: "ok",
            phase: "end",
            status: "completed",
            title: "run python3 inline script",
            tool_call_id: "tool-call-1",
          },
        },
      }),
    );

    expect(activity?.content.tool).toMatchObject({
      command: "run python3 inline script",
      cwd: "/workspace",
      duration_ms: 42,
      exit_code: 0,
      item_id: "command-tool-1",
      output: "ok",
      phase: "end",
      status: "completed",
      tool_call_id: "tool-call-1",
    });
  });

  it("defaults legacy activity payloads to version 1", () => {
    const activity = parseAgentActivity(
      JSON.stringify({
        type: CSGCLAW_AGENT_ACTIVITY_TYPE,
        content: {
          msgtype: AgentActivityMsgTypes.tool,
          body: "Running tool",
          tool: { id: "tool-1", status: "running", title: "Run shell command" },
        },
      }),
    );

    expect(activity?.version).toBe(1);
    expect(activity?.channel).toBe("");
  });

  it("classifies tool messages", () => {
    const tool = {
      content: JSON.stringify({
        type: CSGCLAW_AGENT_ACTIVITY_TYPE,
        content: {
          msgtype: AgentActivityMsgTypes.tool,
          body: "Running tool",
          tool: { id: "tool-1", status: "running", title: "Run shell command" },
        },
      }),
    };

    expect(isToolActivityMessage(tool)).toBe(true);
  });

  it("parses OpenClaw plain text command messages with structured output", () => {
    const command = parsePlainAgentCommand(
      '🔎 Web Search: for "上海天气" (top 5) {"query":"上海天气","provider":"duckduckgo","count":5}',
    );

    expect(command).toMatchObject({
      command: 'Web Search: for "上海天气" (top 5)',
      kind: AgentActivityKinds.execCommand,
      signature: 'web search: for "上海天气" (top 5)',
      title: "Web Search",
    });
    expect(command?.output).toContain('"provider": "duckduckgo"');
  });

  it("classifies csgclaw-cli command messages as exec command activity", () => {
    expect(parsePlainAgentCommand("🛠 csgclaw-cli task claim --task task-4 --participant-id pt-worker")).toMatchObject({
      command: "csgclaw-cli task claim --task task-4 --participant-id pt-worker",
      kind: AgentActivityKinds.execCommand,
      title: "csgclaw-cli task",
    });
  });

  it("classifies OpenClaw progress lines as exec command activity", () => {
    expect(parsePlainAgentCommand("📖 Read: from ~/.openclaw/workspace/skills/gitlab/SKILL.md")).toMatchObject({
      command: "Read: from ~/.openclaw/workspace/skills/gitlab/SKILL.md",
      kind: AgentActivityKinds.execCommand,
      title: "Read",
    });
    expect(parsePlainAgentCommand('🧠 Memory Search: GitLab issues user context {"results":[]}')).toMatchObject({
      command: "Memory Search: GitLab issues user context",
      kind: AgentActivityKinds.execCommand,
      title: "Memory Search",
    });
    expect(parsePlainAgentCommand("🛠️ run cd → run python3 scripts/ensure_gitlab_auth.py")).toMatchObject({
      command: "run cd → run python3 scripts/ensure_gitlab_auth.py",
      kind: AgentActivityKinds.execCommand,
      title: "run cd",
    });
  });

  it("splits OpenClaw inline script result text from the command", () => {
    expect(parsePlainAgentCommand("run python3 inline script\nok")).toMatchObject({
      command: "run python3 inline script",
      kind: AgentActivityKinds.execCommand,
      output: "ok",
      signature: "run python3 inline script",
    });
    expect(
      parseOpenClawDeliveryCommand({
        content: "run python3 inline script === v0.6.4 迭代内你的 Open Issues: 13 ===",
        metadata: { openclaw: { delivery_kind: "tool" } },
      }),
    ).toMatchObject({
      command: "run python3 inline script",
      kind: AgentActivityKinds.execCommand,
      output: "=== v0.6.4 迭代内你的 Open Issues: 13 ===",
      signature: "run python3 inline script",
    });
  });

  it("splits OpenClaw first-line command output from tool progress text", () => {
    expect(
      parsePlainAgentCommand(
        "📖 Read: from ~/.openclaw/workspace/skills/gitlab-csgclaw/scripts/create_issue.py\n#!/usr/bin/env python3",
      ),
    ).toMatchObject({
      command: "Read: from ~/.openclaw/workspace/skills/gitlab-csgclaw/scripts/create_issue.py",
      kind: AgentActivityKinds.execCommand,
      output: "#!/usr/bin/env python3",
    });
    expect(
      parsePlainAgentCommand(
        "🛠️ run python3 $SKILLS_BASE/gitlab-csgclaw/scripts/get_issue_template.py\nTraceback (most recent call last):",
      ),
    ).toMatchObject({
      command: "run python3 $SKILLS_BASE/gitlab-csgclaw/scripts/get_issue_template.py",
      kind: AgentActivityKinds.execCommand,
      output: "Traceback (most recent call last):",
    });
  });

  it("classifies OpenClaw message receipts as other activity", () => {
    expect(
      parsePlainAgentCommand(`📩 Message
{"channel":"csgclaw","to":"csgclaw:room:room-1","result":{"ok":true}}`),
    ).toMatchObject({
      command: "Message",
      kind: AgentActivityKinds.other,
      title: "Message",
    });
  });

  it("uses OpenClaw delivery metadata for tool activity classification", () => {
    const message = {
      content: "Inspect workspace state",
      metadata: {
        openclaw: {
          delivery_info: {
            futureField: { nested: true },
            kind: "tool",
            toolCallId: "tool-call-1",
            toolStatus: "completed",
          },
          request_id: "msg-user",
        },
      },
    };

    expect(openClawDeliveryKind(message)).toBe("tool");
    expect(openClawToolCallID(message)).toBe("tool-call-1");
    expect(isOpenClawToolDeliveryMessage(message)).toBe(true);
    expect(parseOpenClawDeliveryCommand(message)).toMatchObject({
      command: "Inspect workspace state",
      kind: AgentActivityKinds.execCommand,
      signature: "openclaw-tool:tool-call-1",
      title: "Inspect workspace",
    });
    expect(
      parseOpenClawDeliveryCommand({
        content: "Final answer",
        metadata: { openclaw: { delivery_kind: "final" } },
      }),
    ).toBeNull();
    expect(
      parseOpenClawDeliveryCommand({
        content: "Tool progress without marker",
        metadata: { delivery_info: { kind: "tool", toolCallId: "flat-tool-1" }, delivery_kind: "tool" },
      }),
    ).toMatchObject({
      command: "Tool progress without marker",
      signature: "openclaw-tool:flat-tool-1",
    });
  });

  it("parses legacy fenced command messages", () => {
    const command = parsePlainAgentCommand(`🔧 \`exec\`
\`\`\`json
{"command":"ls web/app","output":"src\\ntests"}
\`\`\``);

    expect(command).toMatchObject({
      command: "ls web/app",
      kind: AgentActivityKinds.execCommand,
      output: "src\ntests",
    });
  });

  it("rejects invalid activity shapes", () => {
    expect(parseAgentActivity("plain text")).toBeNull();
    expect(parseAgentActivity(JSON.stringify({ type: CSGCLAW_AGENT_ACTIVITY_TYPE, content: {} }))).toBeNull();
  });
});
