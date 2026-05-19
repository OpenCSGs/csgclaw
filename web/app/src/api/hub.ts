import { get, post } from "@/api/client";
import type { HubTemplate, HubWorkspaceFile } from "@/models/hubWorkspace";

export function fetchHubTemplates(): Promise<HubTemplate[]> {
  return get("/api/v1/hub/templates");
}

export function fetchHubTemplate(templateID: string): Promise<HubTemplate> {
  return get(`/api/v1/hub/templates/${encodeURIComponent(templateID)}`);
}

export function fetchHubWorkspaceFile(templateID: string, workspacePath: string): Promise<HubWorkspaceFile> {
  return get(
    `/api/v1/hub/templates/${encodeURIComponent(templateID)}/workspace/file?path=${encodeURIComponent(workspacePath)}`,
  );
}

export function publishAgentTemplateRequest(agentID: string): Promise<HubTemplate> {
  return post("/api/v1/hub/templates", {
    agent_id: agentID,
  });
}
