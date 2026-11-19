export const cloudRuntimeProfileSections = [
  { id: "profile", label: "agentProfileTab" },
  { id: "instructions", label: "agentInstructions" },
  { id: "skills", label: "agentProfileSkillsTab" },
  { id: "apps", label: "agentAppsTab" },
  { id: "knowledge", label: "profileKnowledgeTitle" },
] as const;
export type CloudProfileSection = (typeof cloudRuntimeProfileSections)[number]["id"];
