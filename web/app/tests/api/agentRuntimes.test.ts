import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { fetchAgentRuntimes } from "@/api/agentRuntimes";

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

  it("loads bundled runtime status without browser caching", async () => {
    const fetchMock = mockFetch([]);

    await fetchAgentRuntimes();

    expect(fetchMock).toHaveBeenCalledWith("api/v1/agent-runtimes", expect.objectContaining({ cache: "no-store" }));
  });
});
