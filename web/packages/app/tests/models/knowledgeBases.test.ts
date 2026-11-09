import { describe, expect, it } from "vitest";
import {
  configuredKnowledgeBases,
  mergeRemoteKnowledgeBasePages,
  knowledgeBaseMCPServer,
} from "@/models/knowledgeBases";
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";

const REMOTE_KNOWLEDGE_BASES: RemoteKnowledgeBase[] = [
  {
    availability: "available",
    contentID: "content-remote",
    id: "remote",
    name: "Remote only",
  },
  {
    availability: "available",
    configuredMCPName: "content-local",
    contentID: "content-local",
    id: "local",
    name: "Added locally",
  },
];

it("resolves an installed connection by stable metadata after its local name changes", () => {
  const server = {
    name: "renamed",
    config: {
      _meta: {
        "com.opencsg/mcp": {
          type: "llm_wiki",
          auth_type: "csghub_access_token",
          resource_id: "local",
          content_id: "content-local",
        },
      },
    },
  };
  expect(knowledgeBaseMCPServer(REMOTE_KNOWLEDGE_BASES[1], [server])).toBe(server);
  expect(knowledgeBaseMCPServer(REMOTE_KNOWLEDGE_BASES[0], [server])).toBeNull();
});

describe("configuredKnowledgeBases", () => {
  it("keeps only remote knowledge bases already saved as local MCP servers", () => {
    expect(configuredKnowledgeBases(REMOTE_KNOWLEDGE_BASES)).toEqual([REMOTE_KNOWLEDGE_BASES[1]]);
  });

  it("returns an empty local list when no remote knowledge base has been added", () => {
    expect(configuredKnowledgeBases([REMOTE_KNOWLEDGE_BASES[0]])).toEqual([]);
  });
});

describe("mergeRemoteKnowledgeBasePages", () => {
  it("keeps later pages while removing duplicate knowledge bases", () => {
    expect(
      mergeRemoteKnowledgeBasePages([
        { items: [REMOTE_KNOWLEDGE_BASES[0]], page: 1, per: 1, total: 2 },
        { items: [REMOTE_KNOWLEDGE_BASES[0], REMOTE_KNOWLEDGE_BASES[1]], page: 2, per: 1, total: 2 },
      ]),
    ).toEqual(REMOTE_KNOWLEDGE_BASES);
  });
});
