import { agentProfileConfig, agentRuntimeKind, type AgentLike, type AgentProfileLike } from "@/models/agents";
import type { ModelProviderCatalog } from "@/models/modelProviders";

export type TaskConfiguration = {
  model_provider_id?: string;
  model_id?: string;
  reasoning_effort?: string;
  workspace_dir?: string;
  execution_mode?: string;
};

export function supportsTaskConfiguration(agent?: AgentLike): boolean {
  return Boolean(agent && ["codex", "dsh"].includes(agentRuntimeKind(agent)));
}

export function effectiveConversationProfile(
  agent: AgentLike,
  configuration?: TaskConfiguration,
  providers?: ModelProviderCatalog | null,
  defaultProfile = agentProfileConfig(agent),
): AgentProfileLike | null {
  if (!configuration?.model_provider_id && !configuration?.model_id) return defaultProfile ?? null;
  const providerID = configuration.model_provider_id;
  const provider = providers?.providers.find((item) => item.id === providerID);
  return {
    model_provider_id: providerID,
    model_id: configuration.model_id,
    provider: provider?.kind || providerID,
    profile_complete: Boolean(providerID && configuration.model_id),
    reasoning_effort: configuration.reasoning_effort || defaultProfile?.reasoning_effort,
  };
}
