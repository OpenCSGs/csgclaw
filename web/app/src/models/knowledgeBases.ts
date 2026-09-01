import type { JSONRecord } from "@/models/agents";

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
  page: number;
  per: number;
  total: number;
};

export type RemoteKnowledgeBaseMCPConfig = {
  config: JSONRecord;
  name: string;
};

export function configuredKnowledgeBases(items: readonly RemoteKnowledgeBase[]): RemoteKnowledgeBase[] {
  return items.filter((item) => Boolean(item.configuredMCPName?.trim()));
}
