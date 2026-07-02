import { WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY } from "@/shared/storage/keys";
import { isDirectConversation } from "@/models/conversations";
import type { IMConversation } from "@/models/conversations";

type ValueOf<T> = T[keyof T];

export const WorkspacePaneTypes = {
  conversation: "conversation",
  agent: "agent",
  human: "human",
  team: "team",
  computer: "computer",
  modelProvider: "model_provider",
  hub: "hub",
  task: "task",
} as const;

export type WorkspacePaneType = ValueOf<typeof WorkspacePaneTypes>;

export const WorkspaceTabs = {
  messages: "messages",
  threads: "threads",
  agents: "agents",
  hub: "hub",
  tasks: "tasks",
} as const;

export type WorkspaceTab = ValueOf<typeof WorkspaceTabs>;

export const WORKSPACE_TABS = [
  WorkspaceTabs.messages,
  WorkspaceTabs.threads,
  WorkspaceTabs.agents,
  WorkspaceTabs.hub,
  WorkspaceTabs.tasks,
] as const;

export const DefaultWorkspacePaneIds = {
  computer: "local",
  hub: "hub",
} as const;

export const WorkspaceRouteSegments = {
  computer: "computer",
  agents: "agents",
  agent: "agent",
  humans: "humans",
  human: "human",
  teams: "teams",
  team: "team",
  models: "models",
  model: "model",
  hub: "hub",
  templates: "templates",
  skills: "skills",
  tasks: "tasks",
  channels: "channels",
  channel: "channel",
  dms: "dms",
  dm: "dm",
  rooms: "rooms",
  room: "room",
  conversations: "conversations",
  conversation: "conversation",
} as const;

const agentRouteSegments = new Set<string>([WorkspaceRouteSegments.agents, WorkspaceRouteSegments.agent]);
const humanRouteSegments = new Set<string>([WorkspaceRouteSegments.humans, WorkspaceRouteSegments.human]);
const teamRouteSegments = new Set<string>([WorkspaceRouteSegments.teams, WorkspaceRouteSegments.team]);
const modelRouteSegments = new Set<string>([WorkspaceRouteSegments.models, WorkspaceRouteSegments.model]);
const conversationRouteSegments = new Set<string>([
  WorkspaceRouteSegments.channels,
  WorkspaceRouteSegments.channel,
  WorkspaceRouteSegments.dms,
  WorkspaceRouteSegments.dm,
  WorkspaceRouteSegments.rooms,
  WorkspaceRouteSegments.room,
  WorkspaceRouteSegments.conversations,
  WorkspaceRouteSegments.conversation,
]);

export type WorkspacePane = {
  type: WorkspacePaneType;
  id?: string;
  resourceType?: "skill" | "template";
};

export type CollapsedWorkspaceGroups = Record<string, boolean>;

const DEFAULT_COLLAPSED_WORKSPACE_GROUPS: CollapsedWorkspaceGroups = {
  "direct-messages": false,
  rooms: true,
  threads: true,
};

export function paneFromLocation(pathname = window.location.pathname): WorkspacePane {
  const parts = String(pathname || "/")
    .split("/")
    .filter(Boolean)
    .map(decodePathSegment);
  const section = parts[0] || "";
  const id = parts[1] || "";

  if (!section) {
    return { type: WorkspacePaneTypes.conversation, id: "" };
  }
  if (section === WorkspaceRouteSegments.computer) {
    return { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
  }
  if (agentRouteSegments.has(section)) {
    return id
      ? { type: WorkspacePaneTypes.agent, id }
      : { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
  }
  if (humanRouteSegments.has(section)) {
    return id
      ? { type: WorkspacePaneTypes.human, id }
      : { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
  }
  if (teamRouteSegments.has(section)) {
    return id
      ? { type: WorkspacePaneTypes.team, id }
      : { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
  }
  if (modelRouteSegments.has(section)) {
    return id
      ? { type: WorkspacePaneTypes.modelProvider, id }
      : { type: WorkspacePaneTypes.computer, id: DefaultWorkspacePaneIds.computer };
  }
  if (section === WorkspaceRouteSegments.hub) {
    return { type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub };
  }
  if (section === WorkspaceRouteSegments.templates) {
    return id
      ? { type: WorkspacePaneTypes.hub, id, resourceType: "template" }
      : { type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub };
  }
  if (section === WorkspaceRouteSegments.skills) {
    return id
      ? { type: WorkspacePaneTypes.hub, id, resourceType: "skill" }
      : { type: WorkspacePaneTypes.hub, id: DefaultWorkspacePaneIds.hub };
  }
  if (section === WorkspaceRouteSegments.tasks) {
    return { type: WorkspacePaneTypes.task, id };
  }
  if (conversationRouteSegments.has(section)) {
    return id ? { type: WorkspacePaneTypes.conversation, id } : { type: WorkspacePaneTypes.conversation, id: "" };
  }
  return { type: WorkspacePaneTypes.conversation, id: "" };
}

export function pathForPane(
  pane: WorkspacePane | null | undefined,
  rooms: readonly Pick<IMConversation, "id" | "is_direct">[] = [],
): string {
  if (!pane || pane.type === WorkspacePaneTypes.computer) {
    return `/${WorkspaceRouteSegments.computer}`;
  }
  if (pane.type === WorkspacePaneTypes.agent && pane.id) {
    return `/${WorkspaceRouteSegments.agents}/${encodeURIComponent(pane.id)}`;
  }
  if (pane.type === WorkspacePaneTypes.human && pane.id) {
    return `/${WorkspaceRouteSegments.humans}/${encodeURIComponent(pane.id)}`;
  }
  if (pane.type === WorkspacePaneTypes.team && pane.id) {
    return `/${WorkspaceRouteSegments.teams}/${encodeURIComponent(pane.id)}`;
  }
  if (pane.type === WorkspacePaneTypes.modelProvider && pane.id) {
    return `/${WorkspaceRouteSegments.models}/${encodeURIComponent(pane.id)}`;
  }
  if (pane.type === WorkspacePaneTypes.hub) {
    if (pane.resourceType === "template" && pane.id) {
      return `/${WorkspaceRouteSegments.templates}/${encodeURIComponent(pane.id)}`;
    }
    if (pane.resourceType === "skill" && pane.id) {
      return `/${WorkspaceRouteSegments.skills}/${encodeURIComponent(pane.id)}`;
    }
    return `/${WorkspaceRouteSegments.hub}`;
  }
  if (pane.type === WorkspacePaneTypes.task) {
    if (pane.id) {
      return `/${WorkspaceRouteSegments.tasks}/${encodeURIComponent(pane.id)}`;
    }
    return `/${WorkspaceRouteSegments.tasks}`;
  }
  if (pane.type === WorkspacePaneTypes.conversation && pane.id) {
    const room = rooms.find((item) => item.id === pane.id);
    const prefix =
      room && isDirectConversation(room) ? `/${WorkspaceRouteSegments.dms}/` : `/${WorkspaceRouteSegments.rooms}/`;
    return `${prefix}${encodeURIComponent(pane.id)}`;
  }
  return "/";
}

export function decodePathSegment(value: string): string {
  try {
    return decodeURIComponent(value || "");
  } catch (_) {
    return value || "";
  }
}

export function workspaceTabForPane(pane: WorkspacePane | null | undefined): WorkspaceTab {
  if (pane?.type === WorkspacePaneTypes.hub || pane?.type === WorkspacePaneTypes.modelProvider) {
    return WorkspaceTabs.hub;
  }
  if (pane?.type === WorkspacePaneTypes.task) {
    return WorkspaceTabs.tasks;
  }
  if (
    pane?.type === WorkspacePaneTypes.agent ||
    pane?.type === WorkspacePaneTypes.human ||
    pane?.type === WorkspacePaneTypes.team ||
    pane?.type === WorkspacePaneTypes.computer
  ) {
    return WorkspaceTabs.agents;
  }
  return WorkspaceTabs.messages;
}

export function readCollapsedWorkspaceGroups(): CollapsedWorkspaceGroups {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY) || "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { ...DEFAULT_COLLAPSED_WORKSPACE_GROUPS };
    }
    return { ...DEFAULT_COLLAPSED_WORKSPACE_GROUPS, ...(parsed as CollapsedWorkspaceGroups) };
  } catch (_) {
    return { ...DEFAULT_COLLAPSED_WORKSPACE_GROUPS };
  }
}
