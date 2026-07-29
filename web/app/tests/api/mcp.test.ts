import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { fetchRemoteMCPServersPage, installRemoteMCPServerRequest } from "@/api/mcp";

function mockFetch(handler: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) {
  const fetchMock = vi.fn<typeof fetch>(handler);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock as Mock<typeof fetch>;
}

describe("MCP API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("loads and normalizes remote MCP servers through the CSGClaw API", async () => {
    const fetchMock = mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            items: [
              {
                description: "Calendar tools",
                id: "builtin:calendar",
                name: "calendar",
                protocol: "streamable-http",
                url: "https://mcp.example.test/calendar",
              },
            ],
            next_page: 2,
            page: 1,
            per: 12,
            total: 13,
          }),
          { status: 200 },
        ),
    );

    await expect(fetchRemoteMCPServersPage()).resolves.toEqual({
      hasMore: true,
      items: [
        {
          description: "Calendar tools",
          id: "builtin:calendar",
          name: "calendar",
          protocol: "streamable-http",
          url: "https://mcp.example.test/calendar",
        },
      ],
      nextPage: 2,
      page: 1,
      per: 12,
      total: 13,
    });
    expect(fetchMock).toHaveBeenCalledWith("api/v1/mcp-servers/remote?page=1&per=12&search=", expect.any(Object));
  });

  it("passes pagination and search to the CSGClaw remote MCP endpoint", async () => {
    const fetchMock = mockFetch(
      async () => new Response(JSON.stringify({ items: [], page: 2, per: 12, total: 12 }), { status: 200 }),
    );

    await expect(fetchRemoteMCPServersPage(2, "calendar")).resolves.toMatchObject({
      hasMore: false,
      items: [],
      nextPage: null,
      page: 2,
      per: 12,
      total: 12,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/mcp-servers/remote?page=2&per=12&search=calendar",
      expect.any(Object),
    );
  });

  it("installs a remote MCP through CSGClaw using its Hub item id", async () => {
    const fetchMock = mockFetch(async () => new Response(JSON.stringify({ name: "calendar" }), { status: 200 }));

    await expect(installRemoteMCPServerRequest("builtin:calendar")).resolves.toBe("calendar");
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/mcp-servers/remote/builtin%3Acalendar/install",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
