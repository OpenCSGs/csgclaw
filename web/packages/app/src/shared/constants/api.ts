export const API_BASE_PATH = "api/v1";

export const ApiEndpoints = {
  imEvents: `${API_BASE_PATH}/events`,
  version: `${API_BASE_PATH}/version`,
  upgradeStatus: `${API_BASE_PATH}/upgrade/status`,
  upgradeChannel: `${API_BASE_PATH}/upgrade/channel`,
  upgradeApply: `${API_BASE_PATH}/upgrade/apply`,
  agentRuntimes: `${API_BASE_PATH}/agent-runtimes`,
  serverConfig: `${API_BASE_PATH}/server/config`,
  serverRestart: `${API_BASE_PATH}/server/restart`,
  serverRestartStatus: `${API_BASE_PATH}/server/restart/status`,
  authStatus: `${API_BASE_PATH}/auth/status`,
  authLogin: `${API_BASE_PATH}/auth/login`,
  authLogout: `${API_BASE_PATH}/auth/logout`,
  notifierRelayWebhookIngress: `${API_BASE_PATH}/webhooks/ingress`,
} as const;

export const IM_EVENTS_SHARED_WORKER_PATH = "sse-shared-worker.js";
