import { describe, expect, it } from "vitest";
import { agentSessionResponseText, agentSessionRoomTitle, isValidAgentSessionID } from "@/models/agentSessions";
import {
  emptySessionDemoStorageState,
  findSessionRecord,
  loadSessionDemoStorage,
  saveSessionDemoStorage,
  SESSION_DEMO_MAX_MESSAGES,
  SESSION_DEMO_MAX_SESSIONS,
  upsertSessionRecord,
  type AgentSessionRecord,
} from "@/pages/SessionDemoPage/sessionDemoStorage";

describe("agent session models", () => {
  it("validates path-safe session identifiers and formats searchable room titles", () => {
    expect(isValidAgentSessionID("session_01.test~dev")).toBe(true);
    expect(isValidAgentSessionID("bad/session")).toBe(false);
    expect(agentSessionRoomTitle("session-01", "Reviewer", "agent-reviewer")).toBe(
      "Anonymous Session: session-01 | Agent: Reviewer (agent-reviewer)",
    );
  });

  it("persists bounded records and restores only valid data", () => {
    const state = emptySessionDemoStorageState();
    const sessions: AgentSessionRecord[] = Array.from({ length: SESSION_DEMO_MAX_SESSIONS + 3 }, (_, index) => ({
      agentId: "agent-reviewer",
      agentName: "Reviewer",
      sessionId: `session-${index}`,
      messages: Array.from({ length: SESSION_DEMO_MAX_MESSAGES + 4 }, (_, messageIndex) => ({
        id: `${index}-${messageIndex}`,
        role: messageIndex % 2 === 0 ? ("user" as const) : ("assistant" as const),
        content: `Message ${messageIndex}`,
        createdAt: new Date(messageIndex).toISOString(),
      })),
      updatedAt: new Date(index).toISOString(),
    }));
    const next = sessions.reduce(upsertSessionRecord, state);

    saveSessionDemoStorage(localStorage, "test.session-demo", next);
    const restored = loadSessionDemoStorage(localStorage, "test.session-demo");

    expect(restored.sessions).toHaveLength(SESSION_DEMO_MAX_SESSIONS);
    expect(restored.sessions[0].messages).toHaveLength(SESSION_DEMO_MAX_MESSAGES);
    expect(findSessionRecord(restored, restored.sessions[0].sessionId, "agent-reviewer")).not.toBeNull();
  });

  it("extracts final text from a Responses-style payload", () => {
    expect(
      agentSessionResponseText({
        id: "resp-1",
        object: "response",
        created_at: 1,
        completed_at: 2,
        status: "completed",
        model: "agent-reviewer",
        output: [
          {
            id: "message-1",
            type: "message",
            status: "completed",
            role: "assistant",
            content: [{ type: "output_text", text: "Reviewed", annotations: [] }],
          },
        ],
        metadata: { agent_id: "agent-reviewer", room_id: "room-1", session_id: "session-1" },
      }),
    ).toBe("Reviewed");
  });
});
