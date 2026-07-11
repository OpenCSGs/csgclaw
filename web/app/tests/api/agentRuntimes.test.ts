import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { fetchAgentRuntimes, installAgentRuntimeRequest } from "@/api/agentRuntimes";

function mockFetch(payload: unknown): Mock<typeof fetch> {
  const fetchMock = vi.fn<typeof fetch>(
    async () =>
      new Response(JSON.stringify(payload), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("agent runtimes API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads agent runtimes without browser caching", async () => {
    const fetchMock = mockFetch([]);

    await fetchAgentRuntimes();

    expect(fetchMock).toHaveBeenCalledWith("api/v1/agent-runtimes", expect.objectContaining({ cache: "no-store" }));
  });

  it("posts install requests to the encoded runtime resource", async () => {
    const fetchMock = mockFetch({ name: "codex", status: "installed" });

    await installAgentRuntimeRequest("codex");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/agent-runtimes/codex/install",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
