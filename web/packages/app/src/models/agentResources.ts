export type ResourceSource = "skill" | "mcp" | "app";
export type ResourceType = "skill" | "connector" | "knowledge_base";
export type ResourceReference = { source: ResourceSource; id: string };
export type ResourceSnapshot = ResourceReference & { type: ResourceType; name: string };
export type AgentResourceSummary = ResourceSnapshot & {
  description?: string;
  enabled: boolean;
  available: boolean;
  unavailable_reason?: string;
};
export const resourceKey = (resource: ResourceReference) => `${resource.source}:${resource.id}`;
export const resourceLabel = (resource: ResourceSnapshot) =>
  resource.type === "skill" ? `/${resource.name}` : resource.name;

export function isResourceSnapshot(value: unknown): value is ResourceSnapshot {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return (
    ["skill", "mcp", "app"].includes(String(item.source)) &&
    typeof item.id === "string" &&
    Boolean(item.id) &&
    typeof item.name === "string" &&
    ["skill", "connector", "knowledge_base"].includes(String(item.type))
  );
}
