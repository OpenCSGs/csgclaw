export const API_BASE_PATH = "api/v1";

export const ApiEndpoints = {
  imEvents: `${API_BASE_PATH}/events`,
  version: `${API_BASE_PATH}/version`,
  upgradeStatus: `${API_BASE_PATH}/upgrade/status`,
  upgradeApply: `${API_BASE_PATH}/upgrade/apply`,
  serverConfig: `${API_BASE_PATH}/server/config`,
  serverRestart: `${API_BASE_PATH}/server/restart`,
  serverRestartStatus: `${API_BASE_PATH}/server/restart/status`,
  notifierRelayWebhookIngress: `${API_BASE_PATH}/webhooks/ingress`,
} as const;

export const IM_EVENTS_SHARED_WORKER_PATH = "sse-shared-worker.js";
