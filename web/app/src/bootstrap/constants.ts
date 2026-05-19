export const MESSAGE_LIST_BOTTOM_THRESHOLD = 24;
export const AGENT_STATUS_REFRESH_INTERVAL_MS = 2000;

export const IM_EVENTS_ENDPOINT = "/api/v1/events";
export const IM_EVENTS_SHARED_WORKER_PATH = "/sse-shared-worker.js";
export const VERSION_ENDPOINT = "/api/v1/version";
export const UPGRADE_STATUS_ENDPOINT = "/api/v1/upgrade/status";
export const UPGRADE_APPLY_ENDPOINT = "/api/v1/upgrade/apply";

export const PROVIDERS = ["csghub_lite", "codex", "claude_code", "api"];
export const RUNTIME_KIND_OPTIONS = [
  { value: "picoclaw_sandbox", label: "picoclaw_sandbox" },
  { value: "openclaw_sandbox", label: "openclaw_sandbox" },
  { value: "codex", label: "codex" },
  { value: "notifier", label: "notifier" },
];
export const GATEWAY_RUNTIME_KIND_OPTIONS = RUNTIME_KIND_OPTIONS.filter(
  (option) => option.value === "picoclaw_sandbox",
);
export const NOTIFIER_DELIVERY_OPTIONS = ["webhook", "remote_pull"];
export const NOTIFIER_RELAY_WEBHOOK_INGRESS_PATH = "/api/v1/webhooks/ingress";
export const CLIPROXY_AUTH_PROVIDERS = new Set(["codex", "claude_code"]);
export const REASONING_EFFORTS = ["low", "medium", "high", "xhigh"];

export const WORKSPACE_TAB_MESSAGES = "messages";
export const WORKSPACE_TAB_AGENTS = "agents";
export const WORKSPACE_TAB_HUB = "hub";

export const CSGCLAW_ACTION_CARD_TYPE = "csgclaw.action_card";
export const CSGCLAW_NOTIFY_CARD_TYPE = "csgclaw.notify_card";
export const ACTION_REBUILD_MANAGER = "rebuild-manager";
export const SHOW_AGENT_LIFECYCLE_ACTIONS = false;
