import { describe, expect, it } from "vitest";
import {
  createdMCPServerName,
  formatMCPServerDocument,
  hasMCPServerName,
  mcpServerDisplayName,
  mcpServerDetailConfig,
  mcpServerDescription,
  mcpServerOrigin,
  isRemoteMCPInstalled,
  managedMCPServerSnapshotDiffers,
  mcpManagedKnowledgeBaseSource,
  mcpProbeResultFromResponse,
  mcpServersFromCatalogResponse,
  mcpServersFromMap,
  mcpServersFromTemplateDocument,
  mcpServerPayloadFromDocument,
  mcpToolParameters,
} from "@/models/mcp";

describe("MCP catalog helpers", () => {
  it("preserves presentation and managed identity while redacting detail credentials", () => {
    const config = {
      name: "Handbook",
      description: "Team knowledge",
      source: "agentichub",
      type: "remote",
      enabled: false,
      url: "https://user:password@example.test/mcp?token=secret#private",
      headers: { Authorization: "Bearer secret" },
      env: { TOKEN: "secret" },
      args: ["--token", "secret"],
      _meta: {
        "com.opencsg/mcp": {
          type: "llm_wiki",
          resource_id: "42",
          content_id: "content-42",
          auth_type: "csghub_access_token",
          access_token: "secret",
        },
        private: { token: "secret" },
      },
    };
    const detail = mcpServerDetailConfig(config);
    expect(detail).toEqual({
      name: "Handbook",
      description: "Team knowledge",
      source: "agentichub",
      type: "remote",
      enabled: false,
      url: "https://example.test/mcp",
      headers: { Authorization: "••••••" },
      env: { TOKEN: "••••••" },
      argument_count: 2,
      _meta: {
        "com.opencsg/mcp": {
          type: "llm_wiki",
          resource_id: "42",
          content_id: "content-42",
          auth_type: "csghub_access_token",
        },
      },
    });
    expect(JSON.stringify(detail)).not.toContain("secret");
    expect(config.headers.Authorization).toBe("Bearer secret");
    expect(config._meta["com.opencsg/mcp"].access_token).toBe("secret");
  });

  it.each(["local-tools", "Team tools", "团队工具"])("resolves the created key for display name %s", (name) => {
    expect(
      createdMCPServerName(
        {
          mcpServers: {
            [name]: { display_name: "Renamed server", command: "old" },
            mcp_created: { display_name: name, command: "node" },
          },
        },
        { name: ` ${name} `, config: { command: "node" } },
      ),
    ).toBe("mcp_created");
  });

  it.each([
    ["kb-1", "wiki-1", "mcp_knowledge"],
    ["kb-2", "wiki-1", ""],
    ["kb-1", "wiki-2", ""],
  ])("resolves created knowledge identity %s/%s", (resourceID, contentID, expected) => {
    const config = {
      _meta: {
        "com.opencsg/mcp": {
          type: "llm_wiki",
          resource_id: "kb-1",
          content_id: "wiki-1",
          auth_type: "csghub_access_token",
        },
      },
    };
    expect(
      createdMCPServerName(
        {
          mcpServers: {
            Engineering: { command: "old" },
            mcp_knowledge: {
              ...config,
              display_name: "Knowledge",
              _meta: {
                "com.opencsg/mcp": {
                  ...config._meta["com.opencsg/mcp"],
                  resource_id: resourceID,
                  content_id: contentID,
                },
              },
            },
          },
        },
        { name: "Engineering", config },
      ),
    ).toBe(expected);
  });

  it.each([
    null,
    {},
    { mcpServers: { tools: { display_name: "Renamed", command: "old" } } },
    { mcpServers: { first: { display_name: "tools" }, second: { display_name: "tools" } } },
  ])("rejects missing or ambiguous created MCP identities", (response) => {
    expect(createdMCPServerName(response, { name: "tools", config: { command: "node" } })).toBe("");
  });

  it("splits state mcpServers into individual sorted server entries", () => {
    expect(
      mcpServersFromCatalogResponse({
        mcpServers: {
          github: { url: "https://github.example/mcp" },
          filesystem: {
            command: "npx",
            args: ["-y", "@modelcontextprotocol/server-filesystem"],
            startup_timeout_sec: 60,
          },
        },
      }),
    ).toEqual([
      {
        name: "filesystem",
        config: { command: "npx", args: ["-y", "@modelcontextprotocol/server-filesystem"], startup_timeout_sec: 60 },
        description: "",
      },
      {
        name: "github",
        config: { url: "https://github.example/mcp" },
        description: "",
      },
    ]);
  });

  it("formats a single MCP server document", () => {
    const formatted = formatMCPServerDocument("filesystem", {
      command: "npx",
      args: ["-y"],
      startup_timeout_sec: 60,
    });

    expect(JSON.parse(formatted)).toEqual({
      mcpServers: {
        filesystem: { command: "npx", args: ["-y"], startup_timeout_sec: 60 },
      },
    });
  });

  it("builds a single MCP server payload from an already parsed document", () => {
    expect(
      mcpServerPayloadFromDocument({
        mcpServers: {
          filesystem: { command: "npx", args: ["-y"] },
        },
      }),
    ).toEqual({
      name: "filesystem",
      config: { command: "npx", args: ["-y"] },
    });
  });

  it("reads agent MCP servers from a direct map", () => {
    expect(
      mcpServersFromMap({
        context7: { command: "npx", args: ["-y", "context7-mcp"] },
      }),
    ).toEqual([
      {
        name: "context7",
        config: { command: "npx", args: ["-y", "context7-mcp"] },
        description: "",
      },
    ]);
  });

  it("uses friendly descriptions for bundled desktop MCPs instead of their executable paths", () => {
    expect(mcpServerDescription({ display_name: "computer-use", command: "/path/to/client" })).toBe(
      "Control desktop apps and browsers.",
    );
    expect(mcpServerDescription({ display_name: "node_repl", command: "/path/to/node_repl" })).toBe(
      "Run JavaScript for browser and desktop automation.",
    );
  });

  it("reads published template MCP files in direct and legacy wrapped formats", () => {
    const direct = {
      context7: { command: "npx", args: ["-y", "context7-mcp"] },
    };
    expect(mcpServersFromTemplateDocument(direct)).toHaveLength(1);
    expect(mcpServersFromTemplateDocument({ mcpServers: direct })).toEqual(mcpServersFromTemplateDocument(direct));
  });

  it("normalizes MCP probe results and exposes tool parameters", () => {
    const result = mcpProbeResultFromResponse({
      connected: true,
      duration_ms: 42,
      protocol_version: "2025-11-25",
      server_info: { name: "docs", title: "Docs", version: "1.0.0" },
      tools_supported: true,
      tools: [
        {
          name: "search",
          title: "Search docs",
          description: "Search documentation.",
          input_schema: {
            type: "object",
            properties: {
              limit: { type: "integer" },
              query: { type: "string", description: "Search query" },
            },
            required: ["query"],
          },
        },
      ],
    });

    expect(result).toMatchObject({
      connected: true,
      durationMs: 42,
      protocolVersion: "2025-11-25",
      serverInfo: { name: "docs", title: "Docs", version: "1.0.0" },
      toolsSupported: true,
    });
    expect(mcpToolParameters(result!.tools[0]!)).toEqual([
      { name: "query", type: "string", description: "Search query", required: true },
      { name: "limit", type: "integer", description: undefined, required: false },
    ]);
  });

  it("detects whether a remote MCP install replaces a local catalog entry", () => {
    const servers = [{ name: "calendar", config: { url: "https://mcp.example.test/calendar" } }];

    expect(hasMCPServerName(servers, "calendar")).toBe(true);
    expect(hasMCPServerName(servers, "calendar ")).toBe(true);
    expect(hasMCPServerName(servers, "github")).toBe(false);
  });

  it("reads managed knowledge base identity without treating it as a live reference", () => {
    const config = {
      url: "https://gateway.example.test/v1/llmwikis/kb-investment/mcp",
      _meta: {
        "com.opencsg/mcp": {
          auth_type: "csghub_access_token",
          content_id: "kb-investment",
          resource_id: "143",
          type: "llm_wiki",
        },
      },
    };

    expect(mcpManagedKnowledgeBaseSource(config)).toEqual({
      contentID: "kb-investment",
      resourceID: "143",
    });
  });

  it("detects a newer global snapshot only for the same managed knowledge base", () => {
    const managed = {
      _meta: {
        "com.opencsg/mcp": {
          auth_type: "csghub_access_token",
          content_id: "kb-investment",
          resource_id: "143",
          type: "llm_wiki",
        },
      },
      headers: { Authorization: "Bearer saved-token" },
      transport: "streamable_http",
      type: "http",
      url: "https://old.example.test/mcp",
    };

    expect(
      managedMCPServerSnapshotDiffers(managed, {
        ...managed,
        headers: { Authorization: "Bearer current-token" },
        url: "https://current.example.test/mcp",
      }),
    ).toBe(true);
    expect(managedMCPServerSnapshotDiffers(managed, { ...managed })).toBe(false);
    expect(
      managedMCPServerSnapshotDiffers(managed, {
        ...managed,
        _meta: {
          "com.opencsg/mcp": {
            auth_type: "csghub_access_token",
            content_id: "kb-tourism",
            resource_id: "144",
            type: "llm_wiki",
          },
        },
        url: "https://current.example.test/mcp",
      }),
    ).toBe(false);
  });
});

describe("MCP identities", () => {
  it.each([
    [{ command: "npx", args: ["local-server"] }, "manual"],
    [{ source: "system" }, "builtin"],
    [{ source: "official" }, "market"],
    [{ _meta: { "com.opencsg/marketplace": { server_id: "42" } } }, "market"],
    [{ _meta: { "com.opencsg/mcp": { type: "agentichub_mcp", resource_id: "42" } } }, "market"],
    [{ _meta: { "com.opencsg/mcp": { type: "llm_wiki", resource_id: "142" } } }, "knowledge"],
  ])("recognizes persisted source metadata %j as %s", (config, origin) => {
    expect(mcpServerOrigin({ name: "mcp_fixed", config })).toBe(origin);
  });
  it("keeps keys for actions and uses Unicode display names in the UI and rename payload", () => {
    const [server] = mcpServersFromMap({ mcp_fixed: { display_name: "必应 搜索", url: "https://example.test" } });
    expect(server.name).toBe("mcp_fixed");
    expect(mcpServerDisplayName(server)).toBe("必应 搜索");
    const document = JSON.parse(formatMCPServerDocument(server.name, server.config));
    document.mcpServers.mcp_fixed.display_name = "改名之后";
    expect(mcpServerPayloadFromDocument(document)?.name).toBe("改名之后");
    expect(Object.keys(document.mcpServers)).toEqual(["mcp_fixed"]);
  });
  it("recognizes a renamed marketplace installation by its source ID", () => {
    const servers = mcpServersFromMap({
      mcp_fixed: {
        display_name: "自定义名称",
        url: "https://example.test",
        _meta: { "com.opencsg/marketplace": { server_id: "42", hub_url: "https://hub.test" } },
      },
    });
    expect(isRemoteMCPInstalled(servers, { id: "42", name: "市场新名字" })).toBe(true);
    expect(isRemoteMCPInstalled(servers, { id: "43", name: "自定义名称" })).toBe(false);
  });
});
