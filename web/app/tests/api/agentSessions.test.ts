import { afterEach, describe, expect, it, vi } from "vitest";
import { createAgentSessionResponse, fetchSessionAgents } from "@/api/agentSessions";

describe("agent session API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns only agents with an active local participant", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify([
              {
                id: "agent-ready",
                name: "Ready",
                participants: [
                  {
                    channel: "csgclaw",
                    type: "agent",
                    lifecycle_status: "active",
                    channel_user_kind: "local_user_id",
                    channel_user_ref: "user-ready",
                  },
                ],
              },
              { id: "agent-remote", name: "Remote", participants: [] },
              {
                id: "agent-ambiguous",
                name: "Ambiguous",
                participants: [
                  {
                    channel: "csgclaw",
                    type: "agent",
                    lifecycle_status: "active",
                    channel_user_kind: "local_user_id",
                    channel_user_ref: "user-ambiguous-a",
                  },
                  {
                    channel: "csgclaw",
                    type: "agent",
                    lifecycle_status: "active",
                    channel_user_kind: "local_user_id",
                    channel_user_ref: "user-ambiguous-b",
                  },
                ],
              },
            ]),
            { status: 200 },
          ),
      ),
    );

    await expect(fetchSessionAgents()).resolves.toEqual([{ id: "agent-ready", name: "Ready", status: "" }]);
  });

  it("posts text input and exposes the nested API error message", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "resp-1",
            object: "response",
            status: "completed",
            output: [],
            metadata: {},
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: { code: "session_busy", message: "Session is busy" } }), {
          status: 409,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await createAgentSessionResponse({ agentName: "Ready Agent", sessionId: "session-1", input: "Hello" });
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "api/v1/agents/Ready%20Agent/sessions/session-1/responses",
      expect.objectContaining({ body: JSON.stringify({ input: "Hello" }), method: "POST" }),
    );
    await expect(
      createAgentSessionResponse({ agentName: "Ready", sessionId: "session-1", input: "Again" }),
    ).rejects.toMatchObject({ status: 409, code: "session_busy", message: "Session is busy" });
  });
});
