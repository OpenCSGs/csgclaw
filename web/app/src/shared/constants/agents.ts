export const MANAGER_AGENT_ID = "u-manager";
export const MANAGER_AGENT_NAME = "manager";
export const MANAGER_AGENT_ROLE = "manager";
export const WORKER_AGENT_ROLE = "worker";
export const DEFAULT_MANAGER_DESCRIPTION = "Manager Worker Dispatch";

export const DEFAULT_PROVIDER = "csghub_lite";
export const DEFAULT_REASONING_EFFORT = "medium";
export const DEFAULT_RUNTIME_KIND = "picoclaw_sandbox";

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
export const CLIPROXY_AUTH_PROVIDERS = new Set(["codex", "claude_code"]);
export const REASONING_EFFORTS = ["low", "medium", "high", "xhigh"];

export const SHOW_AGENT_LIFECYCLE_ACTIONS = false;
