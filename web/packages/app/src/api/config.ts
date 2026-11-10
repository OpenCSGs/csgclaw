import { get, post, put } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";
import type { ConfigSettings, ConfigSettingsUpdatePayload } from "@/models/configSettings";

export type ServerRestartStatusResponse = {
  manual_restart_required?: boolean;
  message?: string;
  last_error?: string;
};

export function fetchServerConfig(): Promise<ConfigSettings> {
  return get(ApiEndpoints.serverConfig);
}

export function updateServerConfig(payload: ConfigSettingsUpdatePayload): Promise<ConfigSettings> {
  return put(ApiEndpoints.serverConfig, payload);
}

export function restartServer(): Promise<void> {
  return post(ApiEndpoints.serverRestart);
}

export function fetchServerRestartStatus(): Promise<ServerRestartStatusResponse> {
  return get(ApiEndpoints.serverRestartStatus);
}
