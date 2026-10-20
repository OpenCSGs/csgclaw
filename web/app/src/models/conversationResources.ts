import { serializeComposerSegments, type ComposerSegment } from "./composer";
import { parseSlashCommand, parseSlashShorthandToPayload, normalizeSlashShorthandForPayload } from "./slashCommands";

import { resourceKey, isResourceSnapshot, type ResourceReference, type ResourceSnapshot } from "./agentResources";
export type {
  ResourceSource,
  ResourceType,
  ResourceReference,
  ResourceSnapshot,
  AgentResourceSummary,
} from "./agentResources";
export { resourceKey } from "./agentResources";
export const maxResourceReferences = 16;

export function uniqueResources<T extends ResourceReference>(resources: readonly T[]): T[] {
  const seen = new Set<string>();
  return resources.filter((resource) => {
    const key = resourceKey(resource);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function resourceSegment(resource: ResourceSnapshot, agentID?: string): ComposerSegment {
  return {
    ...(agentID ? { agentID } : {}),
    type: "resource",
    text: resource.name,
    resource: { source: resource.source, id: resource.id, type: resource.type, name: resource.name },
  };
}

export function selectedResources(segments: readonly ComposerSegment[]): ResourceSnapshot[] {
  return uniqueResources(segments.flatMap((segment) => (segment.type === "resource" ? [segment.resource] : [])));
}

export function addResources(
  segments: readonly ComposerSegment[],
  resources: readonly ResourceSnapshot[],
  agentID?: string,
): ComposerSegment[] {
  const existing = segments.filter((segment) => segment.type === "resource");
  const keys = new Set(selectedResources(segments).map(resourceKey));
  const added = uniqueResources(resources).filter((resource) => !keys.has(resourceKey(resource)));
  return [
    ...existing,
    ...added.map((resource) => resourceSegment(resource, agentID)),
    ...segments.filter((segment) => segment.type !== "resource"),
  ];
}

export function resourcesMatchAgent(segments: readonly ComposerSegment[], agentID: string): boolean {
  return segments.every((segment) => segment.type !== "resource" || (Boolean(agentID) && segment.agentID === agentID));
}

export function resourceMessageInput(
  segments: readonly ComposerSegment[],
  content = serializeComposerSegments(segments),
) {
  const refs: ResourceReference[] = selectedResources(segments).map(({ source, id }) => ({ source, id }));
  const command = parseSlashCommand(content) ?? parseSlashShorthandToPayload(content);
  if (command?.name === "use-skill") {
    refs.push({ source: "skill", id: command.arg });
    content = command.body;
  } else if (command?.name === "new") {
    content = normalizeSlashShorthandForPayload(content);
  }
  return { content, resource_refs: uniqueResources(refs) };
}

export function canSendResources(
  content: string,
  resources: readonly ResourceReference[],
  attachmentCount: number,
): boolean {
  const refs = uniqueResources(resources);
  if (refs.length && (parseSlashCommand(content) ?? parseSlashShorthandToPayload(content))?.name === "new")
    return false;
  return (
    refs.length <= maxResourceReferences &&
    Boolean(content.trim() || attachmentCount || (refs.length === 1 && refs[0].source === "skill"))
  );
}

export function canSendComposer(segments: readonly ComposerSegment[], attachmentCount: number): boolean {
  const input = resourceMessageInput(segments);
  return canSendResources(input.content, input.resource_refs, attachmentCount);
}

export function messageResources(metadata: unknown): ResourceSnapshot[] {
  if (!metadata || typeof metadata !== "object") return [];
  const namespace = (metadata as Record<string, unknown>).csgclaw;
  if (!namespace || typeof namespace !== "object") return [];
  const selection = (namespace as Record<string, unknown>).resource_selection;
  if (!selection || typeof selection !== "object") return [];
  const value = selection as { version?: unknown; refs?: unknown };
  return value.version === 1 && Array.isArray(value.refs) ? uniqueResources(value.refs.filter(isResourceSnapshot)) : [];
}

export function resourceMessageText(content: unknown, metadata: unknown): string {
  return [...messageResources(metadata).map((resource) => `[${resource.name}]`), String(content ?? "")]
    .filter(Boolean)
    .join(" ");
}
