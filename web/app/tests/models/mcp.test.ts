import { describe, expect, it } from "vitest";
import {
  formatMCPServerDocument,
  mcpServersFromCatalogResponse,
  mcpServersFromMap,
  mcpServerPayloadFromDocument,
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
});
