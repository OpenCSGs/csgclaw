import { describe, expect, it } from "vitest";
import {
  formatMCPServerDocument,
  hasMCPServerName,
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
        description: "npx -y @modelcontextprotocol/server-filesystem",
      },
      {
        name: "github",
        config: { url: "https://github.example/mcp" },
        description: "https://github.example/mcp",
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
        description: "npx -y context7-mcp",
      },
    ]);
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
