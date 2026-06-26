import { del, get, post } from "@/api/client";
import type { HubTemplate, HubWorkspaceFile, HubWorkspaceListing } from "@/models/hubWorkspace";

const HUB_TEMPLATES_PATH = "/api/v1/hub/templates";

type PublishAgentTemplatePayload = {
  agent_id: string;
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

export function publishAgentTemplateRequest(agentID: string): Promise<HubTemplate> {
  const payload: PublishAgentTemplatePayload = {
    agent_id: agentID,
  };
  return post<HubTemplate>(HUB_TEMPLATES_PATH, payload);
}

export function deleteHubTemplateRequest(templateID: string): Promise<void> {
  return del(hubTemplatePath(templateID));
}

function hubTemplatePath(templateID: string): string {
  return `${HUB_TEMPLATES_PATH}/${encodeURIComponent(String(templateID || "").trim())}`;
}
