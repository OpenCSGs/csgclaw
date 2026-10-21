import { afterEach, describe, expect, it, vi } from "vitest";
import { createBotRequest, deleteAgentLikeRequest } from "./agents";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createBotRequest", () => {
  it.each([{ skills: ["reviewer"] }, { skills: [] }])(
    "preserves instructions and explicit skills ($skills)",
    async ({ skills }) => {
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(new Response(JSON.stringify({ id: "pt-created", agent_id: "created" })))
        .mockResolvedValueOnce(new Response(JSON.stringify({ id: "created", instructions: "Review carefully." })));
      vi.stubGlobal("fetch", fetchMock);

      await createBotRequest({
        name: "Reviewer",
        runtime_name: "codex",
        instructions: "Review carefully.",
        skills,
      });

      const body = JSON.parse(fetchMock.mock.calls[0][1].body);
      expect(body.agent_binding.agent).toMatchObject({
        instructions: "Review carefully.",
        skills,
      });
    },
  );
});

describe("deleteAgentLikeRequest", () => {
  it("deletes notification participants through the channel participant endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await deleteAgentLikeRequest({
      id: "notification-1",
      name: "Notification",
      type: "notification",
      bot_type: "notification",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/channels/csgclaw/participants/notification-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("keeps regular agents on the agent endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await deleteAgentLikeRequest({
      id: "agent-1",
      name: "Agent",
      type: "agent",
    });

    expect(fetchMock).toHaveBeenCalledWith("api/v1/agents/agent-1", expect.objectContaining({ method: "DELETE" }));
  });
});
