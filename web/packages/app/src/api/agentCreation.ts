import { batchAddAgentMCPServersRequest, fetchAgentMCPServers, updateAgentRequest } from "./agents";
import { bindAgentApp } from "./apps";
import { fetchRemoteKnowledgeBaseMCPConfig } from "./knowledgeBases";
import type { AgentCreationResources } from "@/models/agentCreation";

// Skills are saved with the create request. Failures in the remaining bindings
// must not turn a successful creation into a second create attempt.
export async function applyAgentCreationResources(
  agentID: string,
  resources: Omit<AgentCreationResources, "skills">,
): Promise<string[]> {
  const operations: { name: string; run: () => Promise<unknown> }[] = [];
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
