import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchRemoteKnowledgeBaseMCPConfig, fetchRemoteKnowledgeBases } from "@/api/knowledgeBases";

describe("knowledge bases API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("loads the current user's AgenticHub knowledge bases through CSGClaw", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            items: [
              {
                availability: "available",
                csghub_response: {
                  id: 42,
                  metadata: {
                    mcp_endpoint_url: "https://gateway.example.test/v1/llmwikis/content-42/mcp",
                  },
                  remote_only: { status: "kept" },
                },
                configured_mcp_name: "agentichub-kb-42",
                content_id: "content-42",
                description: "Engineering runbooks",
                id: 42,
                name: "Engineering handbook",
              },
            ],
            page: 1,
            per: 50,
            total: 1,
          }),
          { status: 200 },
        ),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRemoteKnowledgeBases(" handbook ")).resolves.toEqual({
      items: [
        {
          availability: "available",
          csgHubResponse: {
            id: 42,
            metadata: {
              mcp_endpoint_url: "https://gateway.example.test/v1/llmwikis/content-42/mcp",
            },
            remote_only: { status: "kept" },
          },
          configuredMCPName: "agentichub-kb-42",
          contentID: "content-42",
          description: "Engineering runbooks",
          id: "42",
          name: "Engineering handbook",
          unavailableReason: undefined,
        },
      ],
      nextPage: undefined,
      page: 1,
      per: 50,
      total: 1,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/knowledge-bases/remote?page=1&per=50&search=handbook",
      expect.any(Object),
    );
  });

  it("loads an MCP document without receiving a user access token", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            name: "agentichub-kb-42",
            config: {
              type: "remote",
              url: "https://gateway.example.test/v1/llmwikis/content-42/mcp",
              csgclaw: {
                kind: "agentichub_knowledge_base",
                knowledge_base_id: "42",
                content_id: "content-42",
              },
            },
          }),
          { status: 200 },
        ),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchRemoteKnowledgeBaseMCPConfig("42");
    expect(result.name).toBe("agentichub-kb-42");
    expect(JSON.stringify(result)).not.toContain("OPENCSG_TOKEN");
    expect(result.config).not.toHaveProperty("headers");
    expect(JSON.stringify(result)).not.toContain("current-user-token");
  });

  it("requests later knowledge-base pages and exposes the next page", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      Promise.resolve(
        new Response(JSON.stringify({ items: [], page: 2, per: 50, total: 120 }), {
          status: 200,
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRemoteKnowledgeBases(" tourism ", 2)).resolves.toEqual({
      items: [],
      nextPage: 3,
      page: 2,
      per: 50,
      total: 120,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/knowledge-bases/remote?page=2&per=50&search=tourism",
      expect.any(Object),
    );
  });
});
