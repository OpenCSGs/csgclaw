import { afterEach, describe, expect, it, vi } from "vitest";
import { cancelAgentSessionResponse, fetchSessionAgents, streamAgentSessionResponse } from "@/api/agentSessions";

describe("agent session API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns only running Codex agents", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify([
              {
                id: "agent-ready",
                name: "Ready",
                runtime: { name: "codex", state: "running" },
              },
              {
                id: "agent-stopped",
                name: "Stopped",
                runtime: { name: "codex", state: "stopped" },
              },
              { id: "agent-openclaw", name: "OpenClaw", runtime: { name: "openclaw", state: "running" } },
            ]),
            { status: 200 },
          ),
      ),
    );

    await expect(fetchSessionAgents()).resolves.toEqual([{ id: "agent-ready", name: "Ready", status: "running" }]);
    expect(fetch).toHaveBeenCalledWith("api/v1/agents", expect.anything());
  });

  it("streams text deltas and exposes the nested API error message", async () => {
    const deltas: string[] = [];
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode('event: message_start\r\ndata: {"type":"message_start","message":{"id":"resp-1"}}\r'),
        );
        controller.enqueue(
          encoder.encode(
            '\n\r\n: heartbeat\n\nevent: content_block_delta\ndata: {"type":"content_block_delta","delta":{"type":"te',
          ),
        );
        controller.enqueue(
          encoder.encode(
            'xt_delta","text":"Hello"}}\n\nevent: content_block_delta\ndata: {"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}\n\nevent: message_stop\ndata: {"type":"message_stop"}\n\n',
          ),
        );
        controller.close();
      },
    });
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(stream, { status: 200, headers: { "Content-Type": "text/event-stream" } }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: { code: "session_busy", message: "Session is busy" } }), {
          status: 409,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      streamAgentSessionResponse({ agentName: "Ready Agent", sessionId: "session-1", input: "Hello" }, (delta) =>
        deltas.push(delta),
      ),
    ).resolves.toEqual({ id: "resp-1", text: "Hello world" });
    expect(deltas).toEqual(["Hello", " world"]);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "api/v1/agents/Ready%20Agent/sessions/session-1/responses",
      expect.objectContaining({ body: JSON.stringify({ input: "Hello", stream: true }), method: "POST" }),
    );
    await expect(
      streamAgentSessionResponse({ agentName: "Ready", sessionId: "session-1", input: "Again" }),
    ).rejects.toMatchObject({ status: 409, code: "session_busy", message: "Session is busy" });
  });

  it("cancels the active session response explicitly", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      cancelAgentSessionResponse({ agentName: "Ready Agent", sessionId: "session-1" }),
    ).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/agents/Ready%20Agent/sessions/session-1/responses/cancel",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
