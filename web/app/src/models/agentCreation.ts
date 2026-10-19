export type AgentCreationResources = {
  skills: string[];
  connectors: string[];
  apps: string[];
  knowledgeBases: { id: string; name: string; configuredMCPName?: string }[];
};
