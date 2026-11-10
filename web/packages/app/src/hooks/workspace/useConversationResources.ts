import { useEffect, useState } from "react";
import { fetchConversationResourceBatch, type ResourceRequest } from "@/api/agentResources";
import type { ResourceType } from "@/models/conversationResources";
import { useInfiniteAgentResources } from "./useInfiniteAgentResources";
import { workspaceQueryKeys } from "./workspaceQueries";

export function conversationResourceQueryKey(agentID: string, type: ResourceType, request: ResourceRequest = {}) {
  const prefix =
    type === "skill" ? workspaceQueryKeys.agentSkills(agentID) : workspaceQueryKeys.agentMCPServers(agentID);
  return [...prefix, "conversation", type, "cursor", "20", JSON.stringify(request)];
}

export function useConversationResources(agentID: string, type: ResourceType, search: string, enabled = true) {
  const [settledSearch, setSettledSearch] = useState(search);
  useEffect(() => {
    const timer = setTimeout(() => setSettledSearch(search), 150);
    return () => clearTimeout(timer);
  }, [search]);
  const request = { search: settledSearch };
  const ready = settledSearch === search;
  const resources = useInfiniteAgentResources(
    conversationResourceQueryKey(agentID, type, request),
    (cursor, signal) => fetchConversationResourceBatch(agentID, type, cursor, request, signal),
    Boolean(agentID) && enabled && ready,
  );
  return { ...resources, items: ready ? resources.items : [], loading: !ready || resources.query.isLoading };
}
