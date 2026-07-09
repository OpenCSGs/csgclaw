import { describe, expect, it } from "vitest";
import { buildUsersById, type IMConversation, type IMMessage } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";
import { activityWorkingParticipantsForConversation } from "./useConversationController";

function toolActivityMessage(status: string): IMMessage {
  return {
    id: `tool-${status}`,
    sender_id: "pt-dev",
    content: JSON.stringify({
      type: "com.opencsg.csgclaw.agent.activity",
      version: 1,
      event_id: `tool-${status}`,
      sender: "pt-dev",
      content: {
        msgtype: "com.opencsg.csgclaw.agent.tool",
        body: `Tool ${status}: exec`,
        tool: {
          id: "opaque-tool-id",
          kind: "execute",
          status,
          title: "Run command",
        },
      },
    }),
  };
}

function legacyToolMessage(payload: Record<string, unknown>, id = "legacy-tool"): IMMessage {
  return {
    id,
    sender_id: "pt-dev",
    content: ["🔧 `exec`", "```", JSON.stringify(payload), "```"].join("\n"),
  };
}

function agentTextMessage(content: string): IMMessage {
  return {
    id: "agent-text",
    sender_id: "pt-dev",
    content,
  };
}

function agentPlaceholderMessage(): IMMessage {
  return {
    id: "agent-placeholder",
    sender_id: "pt-dev",
    content: "\u200b",
  };
}

function conversationWithMessages(messages: IMMessage[]): IMConversation {
  return {
    id: "room-1",
    is_direct: true,
    members: ["user-admin", "pt-dev"],
    messages,
  };
}

const usersById = buildUsersById([
  { id: "user-admin", name: "Admin" },
  { id: "pt-dev", name: "dev" },
]);

const agents: AgentLike[] = [{ id: "dev", user_id: "pt-dev", name: "dev" }];

describe("activityWorkingParticipantsForConversation", () => {
  it("keeps an agent working while a structured tool activity is running", () => {
    const participants = activityWorkingParticipantsForConversation(
      conversationWithMessages([toolActivityMessage("running")]),
      "user-admin",
      agents,
      usersById,
    );

    expect(participants).toEqual([{ id: "pt-dev", name: "dev" }]);
  });

  it("clears the working participant after the matching tool reaches a terminal status", () => {
    const participants = activityWorkingParticipantsForConversation(
      conversationWithMessages([toolActivityMessage("running"), toolActivityMessage("completed")]),
      "user-admin",
      agents,
      usersById,
    );

    expect(participants).toEqual([]);
  });

  it("clears a structured tool when the agent sends a runtime error reply", () => {
    const participants = activityWorkingParticipantsForConversation(
      conversationWithMessages([
        toolActivityMessage("running"),
        agentTextMessage(
          "Runtime error: unsupported_feature:input.web_search_call (type=invalid_request_error, code=unsupported_feature)",
        ),
      ]),
      "user-admin",
      agents,
      usersById,
    );

    expect(participants).toEqual([]);
  });

  it("keeps a structured tool active after the agent turn placeholder arrives", () => {
    const participants = activityWorkingParticipantsForConversation(
      conversationWithMessages([toolActivityMessage("running"), agentPlaceholderMessage()]),
      "user-admin",
      agents,
      usersById,
    );

    expect(participants).toEqual([{ id: "pt-dev", name: "dev" }]);
  });

  it("keeps an agent working after a PicoClaw legacy tool message without status", () => {
    const participants = activityWorkingParticipantsForConversation(
      conversationWithMessages([legacyToolMessage({ command: "run node inline script" })]),
      "user-admin",
      agents,
      usersById,
    );

    expect(participants).toEqual([{ id: "pt-dev", name: "dev" }]);
  });

  it("clears a PicoClaw legacy tool after matching output arrives", () => {
    const participants = activityWorkingParticipantsForConversation(
      conversationWithMessages([
        legacyToolMessage({ command: "run node inline script" }, "legacy-start"),
        legacyToolMessage({ command: "run node inline script", output: "done" }, "legacy-output"),
      ]),
      "user-admin",
      agents,
      usersById,
    );

    expect(participants).toEqual([]);
  });

  it("clears a PicoClaw legacy tool when the agent sends a normal reply", () => {
    const participants = activityWorkingParticipantsForConversation(
      conversationWithMessages([
        legacyToolMessage({ command: "run node inline script" }),
        agentTextMessage("The command finished."),
      ]),
      "user-admin",
      agents,
      usersById,
    );

    expect(participants).toEqual([]);
  });
});
