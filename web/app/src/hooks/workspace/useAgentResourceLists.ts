import { fetchAgentSkillBatch, fetchAgentMCPBatch } from "@/api/agentResources";
import { useInfiniteAgentResources } from "./useInfiniteAgentResources";
import { workspaceQueryKeys } from "./workspaceQueries";

export function useAgentResourceLists(agentID: string) {
  const skills = useInfiniteAgentResources(
    [...workspaceQueryKeys.agentSkills(agentID), "cursor", "profile"],
    (cursor, signal) => fetchAgentSkillBatch(agentID, cursor, {}, signal),
    Boolean(agentID),
  );
  const mcp = useInfiniteAgentResources(
    [...workspaceQueryKeys.agentMCPServers(agentID), "cursor", "profile", "mcp"],
    (cursor, signal) => fetchAgentMCPBatch(agentID, cursor, signal, "mcp"),
    Boolean(agentID),
  );
  const knowledge = useInfiniteAgentResources(
    [...workspaceQueryKeys.agentMCPServers(agentID), "cursor", "profile", "knowledge"],
    (cursor, signal) => fetchAgentMCPBatch(agentID, cursor, signal, "knowledge"),
    Boolean(agentID),
  );
  return { skills, mcp, knowledge };
}
