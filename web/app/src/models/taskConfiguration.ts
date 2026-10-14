import { agentRuntimeKind, type AgentLike } from "@/models/agents";

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
