// @ts-nocheck
import { get, post } from "@/api/client";

export function fetchHubTemplates() {
  return get("/api/v1/hub/templates");
}

export function fetchHubTemplate(templateID) {
  return get(`/api/v1/hub/templates/${encodeURIComponent(templateID)}`);
}

export function fetchHubWorkspaceFile(templateID, workspacePath) {
  return get(
    `/api/v1/hub/templates/${encodeURIComponent(templateID)}/workspace/file?path=${encodeURIComponent(workspacePath)}`,
  );
}

export function publishAgentTemplateRequest(agentID) {
  return post("/api/v1/hub/templates", {
    agent_id: agentID,
  });
}
