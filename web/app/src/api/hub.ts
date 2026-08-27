import { del, get, post, put } from "@/api/client";
import type { HubTemplate, HubWorkspaceFile, HubWorkspaceListing } from "@/models/hubWorkspace";

const HUB_TEMPLATES_PATH = "/api/v1/hub/templates";
const HUB_TEMPLATE_ESCAPE = "~";
const HUB_TEMPLATE_NAMESPACE_SEPARATOR = "~s~";

export type AgentTemplatePublishTarget = "local" | "official" | "official_deploy";

type PublishAgentTemplatePayload = {
  agent_id: string;
  registry: AgentTemplatePublishTarget;
  name: string;
  description: string;
  include_memory?: boolean;
  deploy?: boolean;
};

export function fetchHubTemplates(): Promise<HubTemplate[]> {
  return get<HubTemplate[]>(HUB_TEMPLATES_PATH);
}

export function fetchHubTemplate(templateID: string): Promise<HubTemplate> {
  return get<HubTemplate>(hubTemplatePath(templateID));
}

export function fetchHubWorkspace(templateID: string, workspacePath = ""): Promise<HubWorkspaceListing> {
  const query = workspacePath ? `?path=${encodeURIComponent(workspacePath)}` : "";
  return get<HubWorkspaceListing>(`${hubTemplatePath(templateID)}/workspace${query}`);
}

export function fetchHubWorkspaceFile(templateID: string, workspacePath: string): Promise<HubWorkspaceFile> {
  return get<HubWorkspaceFile>(
    `${hubTemplatePath(templateID)}/workspace/file?path=${encodeURIComponent(workspacePath)}`,
  );
}

export function updateHubWorkspaceFile(
  templateID: string,
  workspacePath: string,
  content: string,
): Promise<HubWorkspaceFile> {
  return put(`${hubTemplatePath(templateID)}/workspace/file?path=${encodeURIComponent(workspacePath)}`, { content });
}

export function publishAgentTemplateRequest(
  agentID: string,
  registry: AgentTemplatePublishTarget,
  name: string,
  description: string,
  includeMemory = false,
): Promise<HubTemplate> {
  const payload: PublishAgentTemplatePayload = {
    agent_id: agentID,
    registry: registry === "official_deploy" ? "official" : registry,
    name,
    description,
    ...(includeMemory ? { include_memory: true } : {}),
    ...(registry === "official_deploy" ? { deploy: true } : {}),
  };
  return post<HubTemplate>(HUB_TEMPLATES_PATH, payload);
}

export function publishHubTemplateToCommunityRequest(templateID: string, deploy = false): Promise<HubTemplate> {
  return post<HubTemplate>(HUB_TEMPLATES_PATH, {
    template_id: templateID,
    registry: "official",
    ...(deploy ? { deploy: true } : {}),
  });
}

export function deleteHubTemplateRequest(templateID: string): Promise<void> {
  return del(hubTemplatePath(templateID));
}

function hubTemplatePath(templateID: string): string {
  const routeID = String(templateID || "")
    .trim()
    .replaceAll(HUB_TEMPLATE_ESCAPE, HUB_TEMPLATE_ESCAPE.repeat(2))
    .replaceAll("/", HUB_TEMPLATE_NAMESPACE_SEPARATOR);
  return `${HUB_TEMPLATES_PATH}/${encodeURIComponent(routeID)}`;
}
