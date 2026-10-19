import { useQuery } from "@tanstack/react-query";
import { fetchAppResources } from "@/api/apps";
import {
  useWorkspaceKnowledgeBasesQuery,
  useWorkspaceMCPServersQuery,
  useWorkspaceSkillsQuery,
} from "@/hooks/workspace/workspaceQueries";

export function useAgentCreationResourceQueries(authenticated: boolean) {
  const skills = useWorkspaceSkillsQuery();
  const mcp = useWorkspaceMCPServersQuery();
  const apps = useQuery({
    queryKey: ["agent-creation", "apps"],
    queryFn: ({ signal }) => fetchAppResources(signal),
  });
  const knowledge = useWorkspaceKnowledgeBasesQuery("", { enabled: authenticated });
  return { skills, mcp, apps, knowledge };
}
