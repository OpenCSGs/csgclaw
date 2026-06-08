import { get, post, put } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";
import type { ConfigSettings, ConfigSettingsUpdatePayload } from "@/models/configSettings";

export type ConfigRestartStatusResponse = {
  manual_restart_required?: boolean;
  message?: string;
  last_error?: string;
};

export function fetchConfigSettings(): Promise<ConfigSettings> {
  return get(ApiEndpoints.configSettings);
}

export function updateConfigSettings(payload: ConfigSettingsUpdatePayload): Promise<ConfigSettings> {
  return put(ApiEndpoints.configSettings, payload);
}

export function applyConfigRestart(): Promise<void> {
  return post(ApiEndpoints.configApply);
}

export function fetchConfigRestartStatus(): Promise<ConfigRestartStatusResponse> {
  return get(ApiEndpoints.configRestartStatus);
}
