import { get, post, put } from "@/api/client";
import type { ConnectorConfigDraft } from "@/models/connectors";
import { normalizeConnectorConfigDraft } from "@/models/connectors";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchConnectors(): Promise<unknown> {
  return get(ApiEndpoints.connectors);
}

export function fetchGitHubConnectorStatus(): Promise<unknown> {
  return get(ApiEndpoints.githubConnector);
}

export function saveGitHubConnectorConfigRequest(draft: ConnectorConfigDraft): Promise<unknown> {
  const normalized = normalizeConnectorConfigDraft(draft);
  const payload: {
    client_id: string;
    client_secret?: string;
    scopes: string[];
  } = {
    client_id: normalized.client_id,
    scopes: normalized.scopes,
  };
  if (normalized.client_secret) {
    payload.client_secret = normalized.client_secret;
  }
  return put(ApiEndpoints.githubConnectorConfig, payload);
}

export function startGitHubConnectorOAuthRequest(returnURL = ""): Promise<unknown> {
  return post(ApiEndpoints.githubConnectorOAuthStart, { return_url: returnURL });
}

export function startGitHubConnectorAppInstallRequest(): Promise<unknown> {
  return post(ApiEndpoints.githubConnectorAppInstallStart);
}

export function gitHubConnectorOAuthStartURL(returnURL = ""): string {
  const params = new URLSearchParams();
  if (returnURL.trim()) {
    params.set("return_url", returnURL);
  }
  const query = params.toString();
  return query ? `${ApiEndpoints.githubConnectorOAuthStart}?${query}` : ApiEndpoints.githubConnectorOAuthStart;
}

export function disconnectGitHubConnectorRequest(): Promise<unknown> {
  return post(ApiEndpoints.githubConnectorDisconnect);
}
