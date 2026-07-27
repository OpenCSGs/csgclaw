export const MANAGER_AGENT_ID = "u-manager";
export const MANAGER_PARTICIPANT_ID = "manager";
export const MANAGER_AGENT_NAME = "manager";
export const MANAGER_AGENT_ROLE = "manager";
export const WORKER_AGENT_ROLE = "worker";
export const DEFAULT_MANAGER_DESCRIPTION = "Agent Teams Manager";

export const DEFAULT_PROVIDER = "csghub_lite";
export const DEFAULT_REASONING_EFFORT = "auto";
export const REASONING_DISABLED_EFFORT = "none";
export const DEFAULT_RUNTIME_KIND = "picoclaw_sandbox";

export const BOT_TYPE_NORMAL = "normal";
export const BOT_TYPE_NOTIFICATION = "notification";

/** Create-agent modal tab: sandbox worker (default). */
export const BOT_CREATE_KIND_WORKER = "worker";
/** Create-agent modal tab: notification bot (no sandbox). */
export const BOT_CREATE_KIND_NOTIFICATION = "notification";

export const PROVIDER_OPTIONS = [
  { value: "csghub_lite", label: "CSGHub Lite" },
  { value: "csghub", label: "OpenCSG" },
  { value: "codex", label: "Codex" },
  { value: "claude_code", label: "Claude Code" },
  { value: "api", label: "OpenAI API" },
] as const;
export const PROVIDERS = PROVIDER_OPTIONS.map((option) => option.value);
export const RUNTIME_KIND_OPTIONS = [
  { value: "picoclaw_sandbox", label: "picoclaw_sandbox" },
  { value: "openclaw_sandbox", label: "openclaw_sandbox" },
  { value: "codex", label: "codex" },
];
/** Worker create flow only (excludes legacy notifier runtime_kind). */
export const WORKER_RUNTIME_KIND_OPTIONS = RUNTIME_KIND_OPTIONS;
export const GATEWAY_RUNTIME_KIND_OPTIONS = RUNTIME_KIND_OPTIONS.filter(
  (option) => option.value === "picoclaw_sandbox",
);
export const NOTIFIER_DELIVERY_OPTIONS = ["webhook", "remote_pull"];
export const DEFAULT_NOTIFIER_POLL_INTERVAL = "5s";
export const CLIPROXY_AUTH_PROVIDERS = new Set(["codex", "claude_code"]);
export const REASONING_EFFORTS = ["minimal", "low", "medium", "high", "xhigh"] as const;
export const REASONING_OPTIONS = [DEFAULT_REASONING_EFFORT, REASONING_DISABLED_EFFORT, ...REASONING_EFFORTS] as const;

export const SHOW_AGENT_LIFECYCLE_ACTIONS = false;
