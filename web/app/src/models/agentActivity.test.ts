import { describe, expect, it } from "vitest";
import { isToolCallMessage, type IMMessage } from "@/models/conversations";
import { parseMessageActivityCommand, parsePlainAgentCommand } from "./agentActivity";

describe("parsePlainAgentCommand", () => {
  it("does not treat CSGClaw CLI text as a command without structured metadata", () => {
    expect(
      parsePlainAgentCommand(
        [
          "CSGClaw CLI 已经成功调用，可用命令如下：",
          "",
          "| 命令 | 功能 |",
          "|------|------|",
          "| `participant` / `pt` | 管理频道参与者 |",
        ].join("\n"),
      ),
    ).toBeNull();

    expect(parsePlainAgentCommand("csgclaw-cli is a lite CSGClaw CLI for participants.")).toBeNull();
    expect(parsePlainAgentCommand("csgclaw-cli room list\nroom-1  dev3-openclaw")).toBeNull();
  });
});

describe("parseMessageActivityCommand", () => {
  it("respects explicit OpenClaw final deliveries", () => {
    const message: IMMessage = {
      content: "csgclaw-cli room list\nroom-1  dev3-openclaw",
      metadata: {
        openclaw: {
          delivery_info: { kind: "final" },
          delivery_kind: "final",
        },
      },
    };

    expect(parseMessageActivityCommand(message)).toBeNull();
    expect(isToolCallMessage(message)).toBe(false);
  });

  it("keeps explicit OpenClaw final deliveries visible even when content looks like a legacy tool", () => {
    const message: IMMessage = {
      content: '🔧 `exec`\n```\n{"command":"csgclaw-cli participant list","timeout":15}\n```',
      metadata: {
        openclaw: {
          delivery_info: { kind: "final" },
          delivery_kind: "final",
        },
      },
    };

    expect(parseMessageActivityCommand(message)).toBeNull();
    expect(isToolCallMessage(message)).toBe(false);
  });

  it("keeps plain CSGClaw CLI text visible in chat", () => {
    const message: IMMessage = {
      content: "csgclaw-cli room list\nroom-1  dev3-openclaw",
    };

    expect(parseMessageActivityCommand(message)).toBeNull();
    expect(isToolCallMessage(message)).toBe(false);
  });

  it("keeps explicit OpenClaw tool deliveries hidden as tool calls", () => {
    const message: IMMessage = {
      content: "csgclaw-cli room list\nroom-1  dev3-openclaw",
      metadata: {
        openclaw: {
          delivery_info: { kind: "tool" },
          delivery_kind: "tool",
        },
      },
    };

    const command = parseMessageActivityCommand(message);

    expect(command?.kind).toBe("exec_command");
    expect(command?.command).toBe("csgclaw-cli room list");
    expect(command?.output).toBe("room-1  dev3-openclaw");
    expect(isToolCallMessage(message)).toBe(true);
  });

  it("parses PicoClaw legacy fenced tools as activity commands", () => {
    const message: IMMessage = {
      content: [
        "🔧 `exec`",
        "```",
        '{"command":"csgclaw-cli participant list --channel csgclaw --type agent 2>&1","timeout":15}',
        "```",
      ].join("\n"),
    };

    const command = parseMessageActivityCommand(message);

    expect(command?.kind).toBe("exec_command");
    expect(command?.command).toBe("csgclaw-cli participant list --channel csgclaw --type agent 2>&1");
    expect(command?.title).toBe("exec");
    expect(isToolCallMessage(message)).toBe(true);
  });

  it("does not hide plain wrench text without a structured legacy payload", () => {
    const message: IMMessage = {
      content: "🔧 exec\n我只是普通说明，不是工具调用 payload。",
    };

    expect(parseMessageActivityCommand(message)).toBeNull();
    expect(isToolCallMessage(message)).toBe(false);
  });

  it("does not hide fenced wrench text unless the payload is JSON-like", () => {
    const message: IMMessage = {
      content: ["🔧 `exec`", "```", "not a tool payload", "```"].join("\n"),
    };

    expect(parseMessageActivityCommand(message)).toBeNull();
    expect(isToolCallMessage(message)).toBe(false);
  });
});
