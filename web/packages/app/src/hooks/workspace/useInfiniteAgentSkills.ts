import type { AgentSkillSummary } from "@/api/agents";
import { builtinSlashCommandNames, suggestedSlashCommandNames, type SkillContinuation } from "@/models/slashCommands";
import { useEffect, useRef, useState } from "react";
import { useInfiniteAgentResources } from "./useInfiniteAgentResources";
import { fetchAgentSkillBatch, fetchConversationResourceBatch } from "@/api/agentResources";
import { workspaceQueryKeys } from "./workspaceQueries";
import { conversationResourceQueryKey } from "./useConversationResources";

const excludedCommands = [...builtinSlashCommandNames, ...suggestedSlashCommandNames];
export function useInfiniteAgentSkills(agentID: string, search: string | null, enabled: boolean, unified = false) {
  const [settledSearch, setSettledSearch] = useState(search);
  const currentScope = useRef("");
  const scope = JSON.stringify([agentID, search, enabled]);
  useEffect(() => {
    currentScope.current = scope;
  }, [scope]);
  useEffect(() => {
    const timer = setTimeout(() => setSettledSearch(search), 150);
    return () => clearTimeout(timer);
  }, [search]);
  const resources = useInfiniteAgentResources<AgentSkillSummary>(
    unified
      ? conversationResourceQueryKey(agentID, "skill", {
          search: settledSearch ?? "",
          enabled: true,
          exclude: excludedCommands.map((name) => `skill:${name}`),
        })
      : [...workspaceQueryKeys.agentSkills(agentID), "cursor", "chat", settledSearch ?? ""],
    (cursor, signal) =>
      unified
        ? fetchConversationResourceBatch(
            agentID,
            "skill",
            cursor,
            { search: settledSearch ?? "", enabled: true, exclude: excludedCommands.map((name) => `skill:${name}`) },
            signal,
          )
        : fetchAgentSkillBatch(
            agentID,
            cursor,
            { search: settledSearch ?? "", enabled: true, exclude: excludedCommands },
            signal,
          ),
    Boolean(agentID) && enabled && search !== null && search === settledSearch,
  );
  const ready = search === settledSearch;
  const continuation: SkillContinuation = {
    scope,
    hasMore: ready && resources.continuation.hasMore,
    loading: resources.continuation.loading || !ready,
    failed: resources.continuation.failed,
    loadMore: async () => {
      if (!ready) return false;
      const loaded = await resources.continuation.loadMore();
      return currentScope.current === scope && loaded;
    },
    retry: resources.continuation.retry,
  };
  return {
    items: ready ? resources.items : [],
    continuation,
    loading: enabled && search !== null && (!ready || resources.query.isLoading),
  };
}
