import type { JSONRecord } from "@/models/agents";
import { mcpManagedKnowledgeBaseSource, type MCPServer } from "@/models/mcp";

export type RemoteKnowledgeBaseAvailability = "available" | "unavailable";

export type RemoteKnowledgeBase = {
  availability: RemoteKnowledgeBaseAvailability;
  csgHubResponse?: JSONRecord;
  configuredMCPName?: string;
  contentID: string;
  description?: string;
  id: string;
  name: string;
  unavailableReason?: string;
};

export type RemoteKnowledgeBasePage = {
  items: RemoteKnowledgeBase[];
  nextPage?: number;
  page: number;
  per: number;
  total: number;
};

export type RemoteKnowledgeBaseMCPConfig = {
  config: JSONRecord;
  name: string;
};

export function knowledgeBaseMCPServer(kb: RemoteKnowledgeBase, servers: readonly MCPServer[]): MCPServer | null {
  return (
    servers.find((server) => server.name === kb.configuredMCPName?.trim()) ??
    servers.find((server) => {
      const source = mcpManagedKnowledgeBaseSource(server.config);
      return source?.resourceID === kb.id && source.contentID === kb.contentID;
    }) ??
    null
  );
}

export function configuredKnowledgeBases(items: readonly RemoteKnowledgeBase[]): RemoteKnowledgeBase[] {
  return items.filter((item) => Boolean(item.configuredMCPName?.trim()));
}

export function mergeRemoteKnowledgeBasePages(pages: readonly RemoteKnowledgeBasePage[]): RemoteKnowledgeBase[] {
  const seen = new Set<string>();
  return pages.flatMap((page) =>
    page.items.filter((item) => {
      if (!item.id || seen.has(item.id)) {
        return false;
      }
      seen.add(item.id);
      return true;
    }),
  );
}
