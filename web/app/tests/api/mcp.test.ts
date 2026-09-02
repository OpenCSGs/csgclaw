import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import {
  fetchAgentMCPServerSourceStatus,
  fetchMCPServerSourceStatus,
  fetchRemoteMCPServersPage,
  installRemoteMCPServerRequest,
  probeMCPServerRequest,
  syncMCPServerSource,
  syncAgentMCPServerSource,
} from "@/api/mcp";

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

  it("probes the current MCP draft and normalizes its tools", async () => {
    const fetchMock = mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            connected: true,
            duration_ms: 18,
            protocol_version: "2025-11-25",
            tools_supported: true,
            tools: [{ name: "search", description: "Search docs", input_schema: { type: "object" } }],
          }),
          { status: 200 },
        ),
    );
    const payload = { name: "docs", config: { url: "https://mcp.example.test" } };

    await expect(probeMCPServerRequest(payload)).resolves.toMatchObject({
      connected: true,
      durationMs: 18,
      protocolVersion: "2025-11-25",
      tools: [{ name: "search", description: "Search docs" }],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/mcp-servers:probe",
      expect.objectContaining({ method: "POST", body: JSON.stringify(payload) }),
    );
  });

  it("checks a managed MCP source without mutating its saved snapshot", async () => {
    const fetchMock = mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            auth_type: "csghub_access_token",
            configured_endpoint_url: "https://old.example.test/mcp",
            content_id: "kb-investment",
            kind: "llm_wiki",
            latest_endpoint_url: "https://current.example.test/mcp",
            resource_id: "143",
            source_description: "Investment knowledge base",
            source_name: "Investment",
            update_available: true,
          }),
          { status: 200 },
        ),
    );

    await expect(fetchMCPServerSourceStatus("kb-investment")).resolves.toEqual({
      agentUpdateAvailable: false,
      authType: "csghub_access_token",
      configuredEndpointURL: "https://old.example.test/mcp",
      contentID: "kb-investment",
      globalServerName: undefined,
      kind: "llm_wiki",
      latestEndpointURL: "https://current.example.test/mcp",
      resourceID: "143",
      sourceAvailable: true,
      sourceDescription: "Investment knowledge base",
      sourceName: "Investment",
      updateAvailable: true,
    });
    expect(fetchMock).toHaveBeenCalledWith("api/v1/mcp-servers/kb-investment/source", expect.any(Object));
  });

  it("reports a deleted knowledge base source as unavailable", async () => {
    mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            auth_type: "csghub_access_token",
            configured_endpoint_url: "https://old.example.test/mcp",
            content_id: "kb-investment",
            kind: "agentichub_knowledge_base",
            latest_endpoint_url: "",
            resource_id: "143",
            source_available: false,
            update_available: false,
          }),
          { status: 200 },
        ),
    );

    await expect(fetchAgentMCPServerSourceStatus("agent-1", "kb-investment")).resolves.toMatchObject({
      contentID: "kb-investment",
      sourceAvailable: false,
      updateAvailable: false,
    });
  });

  it("manually syncs a managed MCP source and returns the persisted snapshot", async () => {
    const fetchMock = mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            source: {
              auth_type: "csghub_access_token",
              configured_endpoint_url: "https://current.example.test/mcp",
              content_id: "kb-investment",
              kind: "llm_wiki",
              latest_endpoint_url: "https://current.example.test/mcp",
              resource_id: "143",
              update_available: false,
            },
            state: {
              mcpServers: {
                "kb-investment": {
                  headers: { Authorization: "Bearer current-token" },
                  url: "https://current.example.test/mcp",
                },
              },
            },
          }),
          { status: 200 },
        ),
    );

    await expect(syncMCPServerSource("kb-investment")).resolves.toMatchObject({
      source: { contentID: "kb-investment", resourceID: "143", updateAvailable: false },
      state: {
        mcpServers: {
          "kb-investment": { url: "https://current.example.test/mcp" },
        },
      },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/mcp-servers/kb-investment/source:sync",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("checks and synchronizes AgenticHub through the current Agent MCP", async () => {
    let synced = false;
    const fetchMock = mockFetch(async (_input, init) => {
      if (init?.method === "POST") {
        synced = true;
        return new Response(
          JSON.stringify({
            agent: {
              agent_id: "agent-1",
              servers: {
                kb_investment: { url: "https://current.example.test/mcp" },
              },
            },
            source: {
              auth_type: "csghub_access_token",
              configured_endpoint_url: "https://current.example.test/mcp",
              content_id: "kb_investment",
              kind: "agentichub_knowledge_base",
              latest_endpoint_url: "https://current.example.test/mcp",
              resource_id: "143",
              source_available: true,
              update_available: false,
            },
          }),
          { status: 200 },
        );
      }
      return new Response(
        JSON.stringify({
          agent_update_available: true,
          auth_type: "csghub_access_token",
          configured_endpoint_url: "https://old.example.test/mcp",
          content_id: "kb_investment",
          kind: "agentichub_knowledge_base",
          latest_endpoint_url: "https://current.example.test/mcp",
          resource_id: "143",
          source_available: true,
          update_available: true,
        }),
        { status: 200 },
      );
    });

    await expect(fetchAgentMCPServerSourceStatus("agent-1", "kb_investment")).resolves.toMatchObject({
      agentUpdateAvailable: true,
      contentID: "kb_investment",
      sourceAvailable: true,
      updateAvailable: true,
    });
    await expect(syncAgentMCPServerSource("agent-1", "kb_investment")).resolves.toMatchObject({
      agent: { agent_id: "agent-1", servers: { kb_investment: { url: "https://current.example.test/mcp" } } },
      source: { sourceAvailable: true, updateAvailable: false },
    });
    expect(synced).toBe(true);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "api/v1/agents/agent-1/mcp-servers/kb_investment/source",
      expect.any(Object),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "api/v1/agents/agent-1/mcp-servers/kb_investment/source:sync",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
