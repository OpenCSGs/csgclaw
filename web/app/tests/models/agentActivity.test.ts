import { isToolActivityMessage, parseAgentActivity } from "@/models/agentActivity";
import { AgentActivityMsgTypes, CSGCLAW_AGENT_ACTIVITY_TYPE } from "@/shared/constants/messages";

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

  it("rejects invalid activity shapes", () => {
    expect(parseAgentActivity("plain text")).toBeNull();
    expect(parseAgentActivity(JSON.stringify({ type: CSGCLAW_AGENT_ACTIVITY_TYPE, content: {} }))).toBeNull();
  });
});
