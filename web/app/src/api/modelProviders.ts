import { del, get, post, put } from "@/api/client";
import { normalizeModelProviderCatalog, type ModelProvider, type ModelProviderCatalog } from "@/models/modelProviders";

export type ModelProviderPayload = {
  id?: string;
  display_name?: string;
  preset?: string;
  base_url?: string;
  api_key?: string;
  headers?: Record<string, unknown>;
  models?: string[];
  reasoning_effort?: string;
};

export type ModelProviderCheckResult = {
  id: string;
  status: string;
  message?: string;
  models: string[];
  last_checked_at?: string;
};

export async function fetchModelProviders(): Promise<ModelProviderCatalog> {
  return normalizeModelProviderCatalog(await get("api/v1/model-providers"));
}

export function createModelProvider(payload: ModelProviderPayload): Promise<ModelProvider> {
  return post("api/v1/model-providers", payload);
}

export function updateModelProvider(providerID: string, payload: ModelProviderPayload): Promise<ModelProvider> {
  return put(`api/v1/model-providers/${encodeURIComponent(providerID)}`, payload);
}

export function deleteModelProvider(providerID: string): Promise<void> {
  return del(`api/v1/model-providers/${encodeURIComponent(providerID)}`);
}

export function checkModelProvider(
  providerID: string,
  payload: ModelProviderPayload = {},
): Promise<ModelProviderCheckResult> {
  return post(`api/v1/model-providers/${encodeURIComponent(providerID)}/check`, payload);
}
