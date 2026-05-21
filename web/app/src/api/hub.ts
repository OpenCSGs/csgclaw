import { get, post } from "@/api/client";
import type { HubTemplate, HubWorkspaceFile } from "@/models/hubWorkspace";

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

function hubTemplatePath(templateID: string): string {
  const id = String(templateID || "");
  const separator = id.indexOf("/");
  const registry = id.slice(0, separator);
  const name = id.slice(separator + 1);
  return `${HUB_TEMPLATES_PATH}/${registry}/${name}`;
}
