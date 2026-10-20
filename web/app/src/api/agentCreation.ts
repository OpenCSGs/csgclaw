import {
  batchAddAgentMCPServersRequest,
  batchAddAgentSkillsRequest,
  fetchAgentMCPServers,
  updateAgentRequest,
} from "./agents";
import { bindAgentApp } from "./apps";
import { fetchRemoteKnowledgeBaseMCPConfig } from "./knowledgeBases";
import type { AgentCreationResources } from "@/models/agentCreation";

// Resource failures must not turn a successful creation into a second create attempt.
export async function applyAgentCreationResources(
  agentID: string,
  resources: AgentCreationResources,
): Promise<string[]> {
  const operations: { name: string; run: () => Promise<unknown> }[] = [];
  if (resources.skills.length)
    operations.push({
      name: resources.skills.join(", "),
      run: () => batchAddAgentSkillsRequest(agentID, resources.skills),
    });
  const names = [
    ...resources.connectors,
    ...resources.knowledgeBases.flatMap((item) => (item.configuredMCPName ? [item.configuredMCPName] : [])),
  ];
  if (names.length)
    operations.push({
      name: names.join(", "),
      run: () => batchAddAgentMCPServersRequest(agentID, Array.from(new Set(names))),
    });
  for (const id of resources.apps) operations.push({ name: id, run: () => bindAgentApp(agentID, id) });
  const pendingKnowledge = resources.knowledgeBases.filter((item) => !item.configuredMCPName);
  if (pendingKnowledge.length)
    operations.push({
      name: pendingKnowledge.map((item) => item.name).join(", "),
      run: async () => {
        const configs = await Promise.all(pendingKnowledge.map((item) => fetchRemoteKnowledgeBaseMCPConfig(item.id)));
        const current = await fetchAgentMCPServers(agentID);
        await updateAgentRequest(agentID, {
          mcpServers: { ...current.servers, ...Object.fromEntries(configs.map((item) => [item.name, item.config])) },
        });
      },
    });
  const failed: string[] = [];
  // Preserve the existing server map across additions and knowledge configuration.
  for (const operation of operations) {
    try {
      await operation.run();
    } catch {
      failed.push(operation.name);
    }
  }
  return failed;
}
