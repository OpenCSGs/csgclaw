type ValueOf<T> = T[keyof T];

export const WorkspaceContextSectionIds = {
  messages: "messages",
  rooms: "rooms",
  directMessages: "direct-messages",
  threads: "threads",
  agents: "agents",
  humans: "humans",
  computers: "computers",
  notifications: "notifications",
  teams: "teams",
  hubTemplates: "hub-templates",
  knowledgeBases: "knowledge-bases",
  mcpServers: "mcp-servers",
  hubSkills: "hub-skills",
  models: "models",
  apps: "apps",
  tasks: "tasks",
} as const;

export type WorkspaceContextSectionId = ValueOf<typeof WorkspaceContextSectionIds>;
