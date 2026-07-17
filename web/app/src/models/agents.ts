import {
  CLIPROXY_AUTH_PROVIDERS,
  DEFAULT_PROVIDER,
  DEFAULT_REASONING_EFFORT,
  BOT_CREATE_KIND_NOTIFICATION,
  BOT_CREATE_KIND_WORKER,
  BOT_TYPE_NOTIFICATION,
  BOT_TYPE_NORMAL,
  DEFAULT_NOTIFIER_POLL_INTERVAL,
  DEFAULT_RUNTIME_KIND,
  MANAGER_AGENT_ID,
  MANAGER_PARTICIPANT_ID,
  MANAGER_AGENT_ROLE,
  PROVIDER_OPTIONS,
  RUNTIME_KIND_OPTIONS,
  WORKER_AGENT_ROLE,
} from "@/shared/constants/agents";
import { avatarFallbackText, normalizeAvatarPath } from "@/shared/avatar";
import type { LocaleCode } from "@/models/conversations";
import { providerIDForProvider, providerNameForProviderID, selectorForProviderModel } from "@/models/modelProviders";

export type RuntimeKind = "picoclaw_sandbox" | "openclaw_sandbox" | "codex" | string;
export type RuntimeName = "picoclaw" | "openclaw" | "codex" | string;
export type BotType = typeof BOT_TYPE_NORMAL | typeof BOT_TYPE_NOTIFICATION | string;
export type ProviderName = "csghub_lite" | "csghub" | "codex" | "claude_code" | "api" | string;
export type JSONRecord = Record<string, unknown>;

export const MCP_SERVERS_EXAMPLE: JSONRecord = {
  context7: {
    command: "npx",
    args: ["-y", "context7-mcp"],
    startup_timeout_sec: 60,
  },
};

export type MCPServersParseResult =
  | { ok: true; value: JSONRecord | null }
  | { ok: false; error: "invalid_json" | "object_required" };

export type RuntimeOptionSchema = {
  key?: string | null;
  path?: string | null;
  label?: string | null;
  label_zh?: string | null;
  label_en?: string | null;
  description?: string | null;
  description_zh?: string | null;
  description_en?: string | null;
  type?: string | null;
  required?: boolean | null;
  picker?: string | null;
  options?: string[] | null;
};

export type EnvKeyValueRow = {
  key: string;
  required?: boolean;
  value: string;
};

export type ImageEnvContract = {
  name: string;
  required?: boolean;
  secret?: boolean;
  default?: string | null;
  description?: string | null;
  choices?: string[] | null;
  pattern?: string | null;
  example?: string | null;
  placeholder?: string | null;
};

export type AgentProfileLike = {
  api_key_preview?: string | null;
  api_key_set?: boolean | null;
  base_url?: string | null;
  description?: string | null;
  enable_fast_mode?: boolean | null;
  env?: JSONRecord | null;
  env_restart_required?: boolean | null;
  headers?: JSONRecord | null;
  image_upgrade_required?: boolean | null;
  model_id?: string | null;
  model_provider_id?: string | null;
  profile_complete?: boolean | null;
  provider?: ProviderName | null;
  reasoning_effort?: string | null;
  request_options?: JSONRecord | null;
  runtime_options?: JSONRecord | null;
  mcpServers?: JSONRecord | null;
  runtime_kind?: string | null;
  runtime_name?: RuntimeName | null;
  sandbox_enabled?: boolean | null;
  notifier_profile?: JSONRecord | null;
  notification_profile?: JSONRecord | null;
};

export type AgentRuntimeLike = {
  kind?: RuntimeKind | null;
  name?: RuntimeName | null;
  sandbox_enabled?: boolean | null;
  state?: string | null;
  sandbox_id?: string | null;
  options?: JSONRecord | null;
  option_schemas?: RuntimeOptionSchema[] | null;
};

export type AgentLike = AgentProfileLike & {
  agent_profile?: AgentProfileLike | null;
  model_config?: AgentProfileLike | null;
  profile?: AgentProfileLike | string | null;
  runtime?: AgentRuntimeLike | null;
  bot_type?: BotType | null;
  box_id?: string | null;
  default_image?: string | null;
  from_template?: string | null;
  id?: string | null;
  instructions?: string | null;
  type?: BotType | null;
  available?: boolean | null;
  avatar?: string | null;
  image?: string | null;
  name?: string | null;
  role?: string | null;
  runtime_name?: RuntimeName | null;
  sandbox_enabled?: boolean | null;
  runtime_option_schemas?: RuntimeOptionSchema[] | null;
  runtime_options?: JSONRecord | null;
  status?: string | null;
  template_name?: string | null;
  user_id?: string | null;
  user_name?: string | null;
  participants?:
    | {
        agent_id?: string | null;
        channel?: string | null;
        channel_app_ref?: string | null;
        channel_user_kind?: string | null;
        channel_user_ref?: string | null;
        id?: string | null;
        lifecycle_status?: string | null;
        mentionable?: boolean | null;
        metadata?: JSONRecord | null;
        name?: string | null;
        type?: string | null;
        user_id?: string | null;
        user_name?: string | null;
      }[]
    | null;
};

export type AgentChannelID = "feishu";

export type AgentConnectedChannel = {
  id: AgentChannelID;
  name: string;
  participantID: string;
};

export const AGENT_CHANNELS: Record<AgentChannelID, { id: AgentChannelID; name: string }> = {
  feishu: {
    id: "feishu",
    name: "Feishu",
  },
};

export function normalizeRuntimeName(name: unknown): RuntimeName {
  const value = String(name ?? "")
    .trim()
    .toLowerCase();
  switch (value) {
    case "picoclaw":
    case "picoclaw_sandbox":
      return "picoclaw";
    case "openclaw":
    case "openclaw_sandbox":
      return "openclaw";
    case "codex":
      return "codex";
    default:
      return value;
  }
}

export function composeLegacyRuntimeKind(runtimeName: unknown, sandboxEnabled: unknown): RuntimeKind {
  const name = normalizeRuntimeName(runtimeName);
  const sandbox = Boolean(sandboxEnabled);
  if (!sandbox) {
    return name === "codex" || !name ? "codex" : "";
  }
  switch (name) {
    case "openclaw":
      return "openclaw_sandbox";
    case "picoclaw":
      return "picoclaw_sandbox";
    default:
      return "";
  }
}

function runtimeNameForKind(kind: unknown): RuntimeName {
  const value = normalizeRuntimeKind(kind);
  switch (value) {
    case "openclaw_sandbox":
      return "openclaw";
    case "picoclaw_sandbox":
      return "picoclaw";
    case "codex":
      return "codex";
    default:
      return normalizeRuntimeName(value);
  }
}

function sandboxEnabledForKind(kind: unknown): boolean {
  const value = normalizeRuntimeKind(kind);
  return value === "openclaw_sandbox" || value === "picoclaw_sandbox";
}

export type RuntimeSelectionLike = {
  runtime_kind?: RuntimeKind | null;
  runtime_name?: RuntimeName | null;
  sandbox_enabled?: boolean | null;
};

export type RuntimeConfig = {
  name: RuntimeName;
  sandboxed: boolean;
};

export function runtimeConfigFromSelection(selection: RuntimeSelectionLike | null | undefined): RuntimeConfig {
  const runtimeKind = normalizeRuntimeKind(selection?.runtime_kind);
  if (runtimeKind) {
    return {
      name: runtimeNameForKind(runtimeKind) || "codex",
      sandboxed: sandboxEnabledForKind(runtimeKind),
    };
  }
  const sandboxed = Boolean(selection?.sandbox_enabled);
  const name = normalizeRuntimeName(selection?.runtime_name || (sandboxed ? "picoclaw" : "codex"));
  return {
    name: name || (sandboxed ? "picoclaw" : "codex"),
    sandboxed,
  };
}

export function resolveRuntimeSelection(selection: RuntimeSelectionLike | null | undefined): {
  runtime_kind: RuntimeKind;
  runtime_name: RuntimeName;
  sandbox_enabled: boolean;
} {
  const runtimeKind = normalizeRuntimeKind(selection?.runtime_kind);
  const runtimeConfig = runtimeConfigFromSelection(selection);
  return {
    runtime_kind:
      runtimeKind ||
      composeLegacyRuntimeKind(runtimeConfig.name, runtimeConfig.sandboxed) ||
      (runtimeConfig.sandboxed ? DEFAULT_RUNTIME_KIND : "codex"),
    runtime_name: runtimeConfig.name,
    sandbox_enabled: runtimeConfig.sandboxed,
  };
}

export type AvatarLikeUser = {
  avatar?: string | null;
  id: string;
  name?: string | null;
};

export function agentRuntimeKind(item: AgentLike | AgentProfileLike | null | undefined): RuntimeKind {
  const agent = item as AgentLike | null | undefined;
  return resolveRuntimeSelection({
    runtime_kind: agent?.runtime?.kind || agent?.runtime_kind,
    runtime_name: agent?.runtime?.name || agent?.runtime_name,
    sandbox_enabled: agent?.runtime?.sandbox_enabled ?? agent?.sandbox_enabled,
  }).runtime_kind;
}

export function agentRuntimeName(item: AgentLike | AgentProfileLike | null | undefined): RuntimeName {
  const agent = item as AgentLike | null | undefined;
  return resolveRuntimeSelection({
    runtime_kind: agent?.runtime?.kind || agent?.runtime_kind,
    runtime_name: agent?.runtime?.name || agent?.runtime_name,
    sandbox_enabled: agent?.runtime?.sandbox_enabled ?? agent?.sandbox_enabled,
  }).runtime_name;
}

export function agentSandboxEnabled(item: AgentLike | AgentProfileLike | null | undefined): boolean {
  const agent = item as AgentLike | null | undefined;
  return resolveRuntimeSelection({
    runtime_kind: agent?.runtime?.kind || agent?.runtime_kind,
    runtime_name: agent?.runtime?.name || agent?.runtime_name,
    sandbox_enabled: agent?.runtime?.sandbox_enabled ?? agent?.sandbox_enabled,
  }).sandbox_enabled;
}

export function agentRuntimeState(item: AgentLike | null | undefined): string {
  return String(item?.runtime?.state || item?.status || "").trim();
}

export function agentRuntimeSandboxID(item: AgentLike | null | undefined): string {
  return String(item?.runtime?.sandbox_id || item?.box_id || "").trim();
}

export function agentRuntimeOptions(item: AgentLike | AgentProfileLike | null | undefined): JSONRecord {
  const agent = item as AgentLike | null | undefined;
  if (agent?.runtime?.options && typeof agent.runtime.options === "object" && !Array.isArray(agent.runtime.options)) {
    return agent.runtime.options;
  }
  if (agent?.runtime_options && typeof agent.runtime_options === "object" && !Array.isArray(agent.runtime_options)) {
    return agent.runtime_options;
  }
  return {};
}

export function agentMCPServers(item: AgentLike | AgentProfileLike | null | undefined): JSONRecord | null | undefined {
  const agent = item as AgentLike | null | undefined;
  if (agent?.mcpServers == null) {
    return agent?.mcpServers ?? undefined;
  }
  if (typeof agent.mcpServers === "object" && !Array.isArray(agent.mcpServers)) {
    return { ...(agent.mcpServers as JSONRecord) };
  }
  return undefined;
}

export function agentProfileConfig(item: AgentLike | null | undefined): AgentProfileLike | null {
  if (item?.model_config && typeof item.model_config === "object" && !Array.isArray(item.model_config)) {
    return item.model_config;
  }
  const profile = item?.profile;
  if (profile && typeof profile === "object" && !Array.isArray(profile)) {
    return profile as AgentProfileLike;
  }
  if (item?.agent_profile && typeof item.agent_profile === "object" && !Array.isArray(item.agent_profile)) {
    return item.agent_profile;
  }
  return item ?? null;
}

export type AgentDraft = {
  agent_id?: string;
  api_key: string;
  api_key_preview: string;
  api_key_set: boolean;
  avatar?: string;
  base_url: string;
  default_image?: string;
  description?: string;
  instructions?: string;
  enable_fast_mode: boolean;
  envRows: EnvKeyValueRow[];
  from_template?: string;
  headersText: string;
  image?: string;
  model_id: string;
  model_provider_id?: string;
  name?: string;
  notifier_delivery_complete?: boolean;
  notifier_delivery_mode?: string;
  notifier_poll_interval?: string;
  notifier_remote_ack_url?: string;
  notifier_remote_messages_url?: string;
  notifier_remote_subscription_id?: string;
  notifier_remote_token?: string;
  notifier_remote_token_set?: boolean;
  notifier_remote_url?: string;
  notifier_webhook_token?: string;
  notifier_webhook_token_set?: boolean;
  provider: ProviderName;
  reasoning_effort: string;
  requestOptionsText: string;
  role?: string;
  bot_type?: BotType;
  runtime_options?: JSONRecord;
  mcpServers?: JSONRecord | null;
  runtime_name?: RuntimeName;
  sandbox_enabled?: boolean;
  runtime_kind: RuntimeKind;
  template_name?: string;
};

type NullableAgentDraftFields = {
  [Key in keyof AgentDraft]?: AgentDraft[Key] | null;
};

export type AgentDraftSource = AgentLike & NullableAgentDraftFields;

export type AgentTemplateLike = {
  description?: string | null;
  id?: string | null;
  image?: string | null;
  image_env?: ImageEnvContract[] | null;
  name?: string | null;
  role?: string | null;
  runtime_kind?: string | null;
};

export type AgentProfileModelsResponse = {
  models: string[];
};

export type ManagerTemplateVariant = {
  image: string;
  runtimeKind: RuntimeKind;
};

export type RuntimeBootstrapConfig = {
  advertise_base_url?: string | null;
  default_worker_template?: string | null;
  effective_manager_image?: string | null;
  manager_runtime?: ManagerRuntimeLike | null;
  sandbox_provider?: string | null;
  runtime_default_images?: unknown;
  runtime_kind?: string | null;
  worker_runtime_choices?: RuntimeChoiceLike[] | null;
  runtime_option_schemas?: Record<string, RuntimeOptionSchema[]> | null;
  show_upgrade?: boolean | null;
  supported_runtime_kinds?: unknown;
};

export type ManagerRuntimeLike = {
  name?: RuntimeName | null;
  label?: string | null;
  sandbox_enabled?: boolean | null;
  installed?: boolean | null;
  path?: string | null;
  os?: string | null;
  docs_url?: string | null;
  install_guidance?: string | null;
  message?: string | null;
};

export type RuntimeChoiceLike = {
  name?: RuntimeName | null;
  label?: string | null;
  sandbox_enabled?: boolean | null;
  installed?: boolean | null;
  message?: string | null;
};

export type DraftProfileOptions = {
  description?: string;
  name?: string;
};

export type AgentCreateProgressStep = {
  label: string;
  target: number;
};

export type AgentCreateProgressState = {
  index: number;
  percent: number;
  startedAt: number;
  status: "running" | "failed" | "done";
  steps: AgentCreateProgressStep[];
};

type TranslateFn = (key: string) => string;

const NOTIFIER_STORAGE_KEYS = [
  "delivery_mode",
  "webhook_token",
  "remote_url",
  "remote_messages_url",
  "remote_ack_url",
  "remote_subscription_id",
  "poll_interval",
  "remote_token",
];

function normalizeRuntimeOptionSchema(item: unknown): RuntimeOptionSchema | null {
  if (!item || typeof item !== "object" || Array.isArray(item)) {
    return null;
  }
  const record = item as JSONRecord;
  const path = String(record.path ?? "").trim();
  if (!path) {
    return null;
  }
  return {
    key: String(record.key ?? path).trim() || path,
    path,
    label: String(record.label ?? path).trim() || path,
    label_zh: String(record.label_zh ?? "").trim(),
    label_en: String(record.label_en ?? "").trim(),
    description: String(record.description ?? "").trim(),
    description_zh: String(record.description_zh ?? "").trim(),
    description_en: String(record.description_en ?? "").trim(),
    type: String(record.type ?? "text").trim() || "text",
    required: Boolean(record.required),
    picker: String(record.picker ?? "").trim(),
    options: Array.isArray(record.options)
      ? record.options.map((option) => String(option ?? "").trim()).filter(Boolean)
      : [],
  };
}

export function localizedRuntimeOptionLabel(
  schema: RuntimeOptionSchema | null | undefined,
  locale: LocaleCode,
): string {
  const localized = locale === "zh" ? String(schema?.label_zh ?? "").trim() : String(schema?.label_en ?? "").trim();
  if (localized) {
    return localized;
  }
  return String(schema?.label ?? schema?.path ?? "").trim();
}

export function localizedRuntimeOptionDescription(
  schema: RuntimeOptionSchema | null | undefined,
  locale: LocaleCode,
): string {
  const localized =
    locale === "zh" ? String(schema?.description_zh ?? "").trim() : String(schema?.description_en ?? "").trim();
  if (localized) {
    return localized;
  }
  return String(schema?.description ?? "").trim();
}

export function normalizeRuntimeOptionSchemas(value: unknown): RuntimeOptionSchema[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => normalizeRuntimeOptionSchema(item))
    .filter((item): item is RuntimeOptionSchema => item != null);
}

export function normalizeRuntimeOptionSchemaMap(value: unknown): Record<string, RuntimeOptionSchema[]> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const out: Record<string, RuntimeOptionSchema[]> = {};
  for (const [key, schemas] of Object.entries(value as JSONRecord)) {
    const normalizedKey = normalizeRuntimeKind(key);
    if (!normalizedKey) {
      continue;
    }
    const normalizedSchemas = normalizeRuntimeOptionSchemas(schemas);
    if (normalizedSchemas.length === 0) {
      continue;
    }
    out[normalizedKey] = normalizedSchemas;
  }
  return out;
}

function normalizeRuntimeOptionsRecord(value: unknown): JSONRecord {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return { ...(value as JSONRecord) };
}

function isJSONRecord(value: unknown): value is JSONRecord {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

export function supportsMCPServers(runtimeKind: unknown): boolean {
  const normalized = normalizeRuntimeKind(runtimeKind);
  return normalized === "openclaw_sandbox" || normalized === "picoclaw_sandbox" || normalized === "codex";
}

export function mcpServersText(mcpServers: JSONRecord | null | undefined): string {
  if (mcpServers == null) {
    return "";
  }
  return JSON.stringify(normalizeRuntimeOptionsRecord(mcpServers), null, 2);
}

export function parseMCPServersText(text: string): MCPServersParseResult {
  const trimmed = text.trim();
  if (!trimmed) {
    return { ok: true, value: null };
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed) as unknown;
  } catch {
    return { ok: false, error: "invalid_json" };
  }
  if (!isJSONRecord(parsed)) {
    return { ok: false, error: "object_required" };
  }
  return { ok: true, value: parsed };
}

export function setMCPServers(value: JSONRecord | null): JSONRecord | null {
  if (value === null) {
    return null;
  }
  return normalizeRuntimeOptionsRecord(value);
}

export function isManagerAgent(item: AgentLike | null | undefined): boolean {
  return item?.role === MANAGER_AGENT_ROLE || item?.id === MANAGER_AGENT_ID;
}

export function resolveAgentChannelUserID(item: AgentLike | null | undefined): string {
  if (!item) {
    return "";
  }
  const participant = item.participants?.find(
    (candidate) => String(candidate?.channel || "").trim() === "csgclaw" && String(candidate?.id || "").trim(),
  );
  const channelUserID = String(participant?.channel_user_ref || participant?.id || "").trim();
  if (channelUserID) {
    return channelUserID;
  }
  if (isManagerAgent(item)) {
    return MANAGER_PARTICIPANT_ID;
  }
  return String(item.user_id || item.id || "").trim();
}

function agentAvatarUserIDs(item: AgentLike | null | undefined): string[] {
  const out: string[] = [];
  const push = (value: unknown) => {
    const id = String(value ?? "").trim();
    if (!id || out.includes(id)) {
      return;
    }
    out.push(id);
  };
  const pushWithCanonicalUser = (value: unknown) => {
    push(value);
    let suffix = String(value ?? "").trim();
    if (!suffix) {
      return;
    }
    while (true) {
      const next = suffix.replace(/^(?:user-|agent-|pt-|u-)/, "");
      if (next === suffix) {
        break;
      }
      suffix = next;
    }
    if (suffix) {
      push(`user-${suffix}`);
    }
  };
  pushWithCanonicalUser(item?.user_id);
  const participant = item?.participants?.find(
    (candidate) => String(candidate?.channel || "").trim() === "csgclaw" && String(candidate?.id || "").trim(),
  );
  pushWithCanonicalUser(participant?.user_id);
  pushWithCanonicalUser(participant?.channel_user_ref);
  pushWithCanonicalUser(resolveAgentChannelUserID(item));
  pushWithCanonicalUser(item?.id);
  return out;
}

export function resolveAgentAvatarUserID(
  agent: AgentLike | null | undefined,
  usersById?: Map<string, AvatarLikeUser> | null,
): string {
  for (const userID of agentAvatarUserIDs(agent)) {
    if (usersById?.has(userID)) {
      return userID;
    }
  }
  return resolveAgentChannelUserID(agent);
}

export function feishuAgentParticipant(
  item: AgentLike | null | undefined,
): NonNullable<AgentLike["participants"]>[number] | null {
  const agentID = String(item?.id ?? "").trim();
  const participant = item?.participants?.find((candidate) => {
    if (String(candidate?.channel || "").trim() !== "feishu") {
      return false;
    }
    if (String(candidate?.type || "").trim() !== "agent") {
      return false;
    }
    if (String(candidate?.channel_user_kind || "").trim() !== "app_id") {
      return false;
    }
    if (!String(candidate?.id || "").trim()) {
      return false;
    }
    const participantAgentID = String(candidate?.agent_id || "").trim();
    return !agentID || !participantAgentID || participantAgentID === agentID;
  });
  return participant ?? null;
}

export function agentConnectedChannels(item: AgentLike | null | undefined): AgentConnectedChannel[] {
  const feishuParticipant = feishuAgentParticipant(item);
  if (!feishuParticipant) {
    return [];
  }
  return [
    {
      id: "feishu",
      name: AGENT_CHANNELS.feishu.name,
      participantID: String(feishuParticipant.id || ""),
    },
  ];
}

export function hasConnectedAgentChannel(item: AgentLike | null | undefined, channelID: AgentChannelID): boolean {
  return agentConnectedChannels(item).some((channel) => channel.id === channelID);
}

type TranslateFnWithParams = (key: string, params?: Record<string, string | number>) => string;

export function agentDeleteConfirmationMessage(item: AgentLike | null | undefined, t: TranslateFnWithParams): string {
  const name = String(item?.name || item?.id || "").trim();
  const message = t("agentDeleteConfirmMessage", { name });
  const channels = agentConnectedChannels(item).map((channel) => channel.name);
  if (channels.length === 0) {
    return message;
  }
  return [
    message,
    "",
    t("agentDeleteBoundChannels", { channels: channels.join(", ") }),
    "",
    t("agentDeleteCascadeNote"),
  ].join("\n");
}

export function normalizeNotifierDeliveryMode(mode: unknown): string {
  const value = String(mode || "")
    .trim()
    .toLowerCase();
  return value === "remote_pull" ? "remote_pull" : "webhook";
}

export function ensureNotifierPullSubscriptionDraft<T extends Partial<AgentDraft> | null | undefined>(
  draft: T,
): T | AgentDraft {
  if (!draft || !isNotifierRuntimeDraft(draft) || draft.notifier_delivery_mode !== "remote_pull") {
    return draft;
  }
  if (String(draft.notifier_remote_subscription_id || "").trim()) {
    return draft;
  }
  return { ...draft, notifier_remote_subscription_id: newNotifierSubscriptionId() } as AgentDraft;
}

export function newNotifierSubscriptionId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.getRandomValues === "function") {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return `sub-${Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("")}`;
  }
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `sub-${crypto.randomUUID().replace(/-/g, "")}`;
  }
  return `sub-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 14)}`;
}

export function normalizeBotType(value: unknown): BotType {
  const normalized = String(value ?? "")
    .trim()
    .toLowerCase();
  return normalized === BOT_TYPE_NOTIFICATION ? BOT_TYPE_NOTIFICATION : BOT_TYPE_NORMAL;
}

export function isNotificationBotAgent(item: AgentLike | null | undefined): boolean {
  return normalizeBotType(item?.bot_type ?? item?.type) === BOT_TYPE_NOTIFICATION;
}

export function resolveAgentAvatarSource(
  agent: AgentLike | null | undefined,
  usersById?: Map<string, AvatarLikeUser> | null,
): string {
  for (const userID of agentAvatarUserIDs(agent)) {
    const userAvatar = String(usersById?.get(userID)?.avatar ?? "").trim();
    if (userAvatar) {
      return userAvatar;
    }
  }

  return "";
}

export function resolveAgentAvatarFallback(
  agent: AgentLike | null | undefined,
  usersById?: Map<string, AvatarLikeUser> | null,
): string {
  for (const userID of agentAvatarUserIDs(agent)) {
    const user = usersById?.get(userID);
    if (user) {
      return avatarFallbackText(user.avatar, user.name, user.id);
    }
  }
  return avatarFallbackText(agent?.name, agent?.id);
}

export function partitionWorkspaceAgentItems(
  agents: readonly AgentLike[] | null | undefined,
  managerID = MANAGER_AGENT_ID,
): { workerAgentItems: AgentLike[]; notificationAgentItems: AgentLike[] } {
  const manager = (agents ?? []).find((item) => item.role === MANAGER_AGENT_ROLE || item.id === managerID) ?? null;
  const rest = (agents ?? []).filter((item) => item.id !== manager?.id);
  const notificationAgentItems = rest.filter((item) => isNotificationBotAgent(item));
  const workerAgentItems = [manager, ...rest.filter((item) => !isNotificationBotAgent(item))].filter(
    (item): item is AgentLike => Boolean(item),
  );
  return { workerAgentItems, notificationAgentItems };
}

export function mergeAgentIntoList(
  items: readonly AgentLike[] | null | undefined,
  updated: AgentLike | null | undefined,
): AgentLike[] {
  const currentItems = [...(items ?? [])];
  const id = String(updated?.id ?? "").trim();
  if (!id || !updated) {
    return currentItems;
  }

  let found = false;
  const next = currentItems.map((item) => {
    if (String(item?.id ?? "").trim() !== id) {
      return item;
    }
    found = true;
    const merged: AgentLike = { ...item, ...updated };
    if (
      item?.model_config &&
      updated.model_config &&
      typeof item.model_config === "object" &&
      typeof updated.model_config === "object" &&
      !Array.isArray(item.model_config) &&
      !Array.isArray(updated.model_config)
    ) {
      merged.model_config = { ...(item.model_config ?? {}), ...(updated.model_config ?? {}) };
    }
    if (
      item?.profile &&
      updated.profile &&
      typeof item.profile === "object" &&
      typeof updated.profile === "object" &&
      !Array.isArray(item.profile) &&
      !Array.isArray(updated.profile)
    ) {
      merged.profile = { ...(item.profile ?? {}), ...(updated.profile ?? {}) };
    }
    if (item?.agent_profile || updated.agent_profile) {
      merged.agent_profile = { ...(item.agent_profile ?? {}), ...(updated.agent_profile ?? {}) };
    }
    return merged;
  });

  return found ? next : [...next, updated];
}

export function agentDraftWithRuntimeFieldsFromAgent(
  draft: AgentDraft | null | undefined,
  updated: AgentLike | null | undefined,
): AgentDraft | null {
  if (!draft) {
    return null;
  }
  const agentID = String(updated?.id ?? "").trim();
  const draftID = String(draft.agent_id ?? "").trim();
  if (!agentID || (draftID && draftID !== agentID)) {
    return draft;
  }

  const next: AgentDraft = { ...draft, agent_id: draftID || agentID };
  if (updated?.image != null) {
    const image = String(updated.image).trim();
    next.image = image;
    next.default_image = image;
  }
  const updatedRuntimeKind = agentRuntimeKind(updated);
  if (updatedRuntimeKind) {
    next.runtime_kind = normalizeRuntimeKind(updatedRuntimeKind || next.runtime_kind);
  }
  next.runtime_name = agentRuntimeName(updated);
  next.sandbox_enabled = agentSandboxEnabled(updated);
  const updatedRuntimeOptions = agentRuntimeOptions(updated);
  if (Object.keys(updatedRuntimeOptions).length > 0) {
    next.runtime_options = normalizeRuntimeOptionsRecord(updatedRuntimeOptions);
  }
  return next;
}

export function notificationBotStatusLabel(item: AgentLike | null | undefined, t: TranslateFn): string {
  if (isNotificationBotAgent(item)) {
    return item?.available === true ? t("notificationBotReady") : t("notificationBotNotReady");
  }
  return agentStatusLabel(agentRuntimeState(item), t);
}

export function agentStatusLabel(status: unknown, t: TranslateFn): string {
  const normalized = String(status || "unknown")
    .trim()
    .toLowerCase();
  if (normalized === "running" || normalized === "online") {
    return t("online");
  }
  if (normalized === "offline" || normalized === "stopped" || normalized === "exited") {
    return t("offline");
  }
  if (normalized === "profile_incomplete") {
    return t("offline");
  }
  if (normalized === "failed" || normalized === "error") {
    return t("agentStatusFailed");
  }
  if (normalized === "done" || normalized === "complete" || normalized === "completed") {
    return t("agentStatusDone");
  }
  return t("agentStatusUnknown");
}

export function notificationBotMetaLabel(item: AgentLike | null | undefined, t: TranslateFn): string {
  const draft = agentToDraft(item);
  const mode = draft.notifier_delivery_mode || "webhook";
  if (mode === "remote_pull") {
    return t("notifierDeliveryRemotePull");
  }
  return t("notifierDeliveryWebhook");
}

export function notificationPushWebhookPathForBot(botID: unknown): string {
  const id = String(botID || "").trim();
  if (!id) {
    return "/api/v1/channels/csgclaw/participants/<participant_id>/notifications";
  }
  return `/api/v1/channels/csgclaw/participants/${encodeURIComponent(id)}/notifications`;
}

export function notifierPushWebhookPathForAgent(botID: unknown): string {
  return notificationPushWebhookPathForBot(botID);
}

/** Public webhook base URL from bootstrap (server resolves empty advertise_base_url via listen_addr). */
export function resolvedNotifierWebhookOrigin(bootstrap: RuntimeBootstrapConfig | null | undefined): string {
  return String(bootstrap?.advertise_base_url ?? "")
    .trim()
    .replace(/\/+$/, "");
}

export function notificationPushWebhookNotifyURL(
  origin: unknown,
  botID: unknown,
  placeholderHost = "https://<your-csgclaw-host>",
): string {
  let base = String(origin ?? "")
    .trim()
    .replace(/\/+$/, "");
  if (!base) {
    base = String(placeholderHost || "https://<your-csgclaw-host>").trim();
  }
  return `${base}${notificationPushWebhookPathForBot(botID)}`;
}

export function notifierPushWebhookNotifyURL(
  origin: unknown,
  botID: unknown,
  placeholderHost = "https://<your-csgclaw-host>",
): string {
  return notificationPushWebhookNotifyURL(origin, botID, placeholderHost);
}

const RELAY_ROUTE_SUFFIXES = ["/webhooks/ingress", "/webhook/ingress", "/inbox/messages", "/inbox/ack"] as const;
const RELAY_ROUTE_WEBHOOKS_INGRESS = "/webhooks/ingress";

function parseNotifierRelayURL(input: string): URL | null {
  let raw = String(input ?? "").trim();
  if (!raw) {
    return null;
  }
  if (!/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(raw)) {
    const local = /^(localhost|127\.0\.0\.1|\[::1\])(:|\/?|\?|$)/i.test(raw) || /^\[::1\]/i.test(raw);
    raw = `${local ? "http" : "https"}://${raw.replace(/^\/+/, "")}`;
  }
  try {
    const url = new URL(raw);
    if (!url.hostname) {
      return null;
    }
    return url;
  } catch {
    return null;
  }
}

/** Mirrors Go relayURLPathBase: strip known relay route suffixes from pathname. */
function normalizeRelayBasePath(pathname: string): string {
  let path = (pathname || "/").replace(/\/+$/, "") || "/";
  let lower = path.toLowerCase();
  for (;;) {
    let trimmed = false;
    for (const suffix of RELAY_ROUTE_SUFFIXES) {
      if (lower.endsWith(suffix)) {
        path = path.slice(0, -suffix.length).replace(/\/+$/, "");
        lower = path.toLowerCase();
        trimmed = true;
        break;
      }
    }
    if (!trimmed) {
      break;
    }
  }
  return path || "/";
}

/** Mirrors Go relayURLWithSuffix for /webhooks/ingress under remote_url base. */
function relayIngressPath(pathname: string): string {
  const root = normalizeRelayBasePath(pathname);
  const suffix = RELAY_ROUTE_WEBHOOKS_INGRESS.replace(/^\/+/, "");
  if (!root || root === "/") {
    return `/${suffix}`;
  }
  return `${root}/${suffix}`.replace(/\/+/g, "/");
}

export function notifierThirdPartyRelayWebhookURL(remoteBase: unknown, subscriptionID: unknown): string {
  const base = String(remoteBase ?? "").trim();
  const sid = String(subscriptionID ?? "").trim();
  if (!base || !sid) {
    return "";
  }
  const url = parseNotifierRelayURL(base);
  if (!url) {
    const ingress = `${base.replace(/\/+$/, "")}${RELAY_ROUTE_WEBHOOKS_INGRESS}`;
    const joiner = ingress.includes("?") ? "&" : "?";
    return `${ingress}${joiner}subscription_id=${encodeURIComponent(sid)}`;
  }
  url.pathname = relayIngressPath(url.pathname);
  url.search = "";
  url.hash = "";
  url.searchParams.set("subscription_id", sid);
  return url.toString();
}

export function notifierComputedPullRoutes(remoteURL: unknown): { messages: string; ack: string } {
  const base = String(remoteURL ?? "").trim();
  if (!base) {
    return { messages: "", ack: "" };
  }
  let input = base;
  if (!/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(input)) {
    const local = /^(localhost|127\.0\.0\.1|\[::1\])(:|\/?|\?|$)/i.test(input) || /^\[::1\]/i.test(input);
    input = `${local ? "http" : "https"}://${input.replace(/^\/+/, "")}`;
  }
  let url: URL;
  try {
    url = new URL(input);
  } catch {
    return { messages: "", ack: "" };
  }
  if (!url.hostname) {
    return { messages: "", ack: "" };
  }
  const cleanPath = (url.pathname || "/").replace(/\/+$/, "") || "/";
  const lower = cleanPath.toLowerCase();
  for (const suffix of ["/webhooks/ingress", "/webhook/ingress"]) {
    const index = lower.lastIndexOf(suffix);
    if (index < 0 || index + suffix.length !== lower.length) {
      continue;
    }
    const parent = cleanPath.slice(0, index).replace(/\/+$/, "");
    const messagesPath = parent ? `${parent}/inbox/messages`.replace(/\/+/g, "/") : "/inbox/messages";
    const ackPath = parent ? `${parent}/inbox/ack`.replace(/\/+/g, "/") : "/inbox/ack";
    const messagesURL = new URL(url.href);
    messagesURL.pathname = messagesPath;
    const ackURL = new URL(url.href);
    ackURL.pathname = ackPath;
    ackURL.search = "";
    return { messages: messagesURL.toString(), ack: ackURL.toString() };
  }
  const trimmed = cleanPath.replace(/\/+$/, "");
  if (!trimmed || trimmed === "/") {
    return {
      messages: `${url.origin}/api/v1/inbox/messages`,
      ack: `${url.origin}/api/v1/inbox/ack`,
    };
  }
  const lastSlash = trimmed.lastIndexOf("/");
  const parentDir = lastSlash <= 0 ? "/" : trimmed.slice(0, lastSlash);
  const ackPath = parentDir === "/" ? "/ack" : `${parentDir}/ack`.replace(/\/+/g, "/");
  return {
    messages: url.toString(),
    ack: new URL(`${url.origin}${ackPath}`).toString(),
  };
}

function mergedRuntimeOptionsForView(
  profile: AgentProfileLike | null | undefined,
  agent: AgentLike | null | undefined,
): JSONRecord {
  const agentOptions = agentRuntimeOptions(agent);
  const profileOptions =
    profile?.runtime_options && typeof profile.runtime_options === "object" && !Array.isArray(profile.runtime_options)
      ? profile.runtime_options
      : {};
  return { ...profileOptions, ...agentOptions };
}

function notifierKeysFromFlatRoot(value: unknown): JSONRecord | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  const source = value as JSONRecord;
  const out: JSONRecord = {};
  for (const key of NOTIFIER_STORAGE_KEYS) {
    if (Object.prototype.hasOwnProperty.call(source, key) && source[key] != null && String(source[key]).trim() !== "") {
      out[key] = source[key];
    }
  }
  if (Object.keys(out).length > 0) {
    return out;
  }
  if (source.delivery_mode != null && String(source.delivery_mode).trim() !== "") {
    const withMode: JSONRecord = {};
    for (const key of NOTIFIER_STORAGE_KEYS) {
      if (Object.prototype.hasOwnProperty.call(source, key) && source[key] != null) {
        withMode[key] = source[key];
      }
    }
    return Object.keys(withMode).length ? withMode : null;
  }
  return null;
}

function notifierProfileSummaryFlags(
  profile: AgentProfileLike | null | undefined,
  agent: AgentLike | null | undefined,
) {
  const runtimeOptions = mergedRuntimeOptionsForView(profile, agent);
  const summary =
    runtimeOptions.notification_profile &&
    typeof runtimeOptions.notification_profile === "object" &&
    !Array.isArray(runtimeOptions.notification_profile)
      ? (runtimeOptions.notification_profile as JSONRecord)
      : runtimeOptions.notifier_profile &&
          typeof runtimeOptions.notifier_profile === "object" &&
          !Array.isArray(runtimeOptions.notifier_profile)
        ? (runtimeOptions.notifier_profile as JSONRecord)
        : profile?.notification_profile &&
            typeof profile.notification_profile === "object" &&
            !Array.isArray(profile.notification_profile)
          ? profile.notification_profile
          : profile?.notifier_profile &&
              typeof profile.notifier_profile === "object" &&
              !Array.isArray(profile.notifier_profile)
            ? profile.notifier_profile
            : null;
  if (!summary) {
    return {
      notifier_delivery_complete: false,
      notifier_webhook_token_set: false,
      notifier_remote_token_set: false,
    };
  }
  return {
    notifier_delivery_complete: Boolean(summary.delivery_complete),
    notifier_webhook_token_set: Boolean(summary.webhook_token_set),
    notifier_remote_token_set: Boolean(summary.remote_token_set),
  };
}

function notifierFlatFromSources(profile: AgentProfileLike | null | undefined, agent?: AgentLike | null): JSONRecord {
  const fromAgentTop = notifierKeysFromFlatRoot(agentRuntimeOptions(agent));
  if (fromAgentTop) {
    return fromAgentTop;
  }
  const profileRuntimeOptions =
    profile?.runtime_options && typeof profile.runtime_options === "object" && !Array.isArray(profile.runtime_options)
      ? profile.runtime_options
      : {};
  const nested =
    profileRuntimeOptions.notifier &&
    typeof profileRuntimeOptions.notifier === "object" &&
    !Array.isArray(profileRuntimeOptions.notifier)
      ? (profileRuntimeOptions.notifier as JSONRecord)
      : {};
  if (Object.keys(nested).length > 0) {
    return nested;
  }
  const fromProfileFlat = notifierKeysFromFlatRoot(profileRuntimeOptions);
  if (fromProfileFlat) {
    return fromProfileFlat;
  }
  const requestOptions =
    profile?.request_options && typeof profile.request_options === "object" && !Array.isArray(profile.request_options)
      ? profile.request_options
      : {};
  const fromRequestOptions =
    requestOptions.notifier && typeof requestOptions.notifier === "object" && !Array.isArray(requestOptions.notifier)
      ? (requestOptions.notifier as JSONRecord)
      : {};
  return fromRequestOptions;
}

export function profileToDraft(profile: AgentProfileLike | null | undefined, agent?: AgentLike | null): AgentDraft {
  const requestOptions =
    profile?.request_options && typeof profile.request_options === "object" && !Array.isArray(profile.request_options)
      ? profile.request_options
      : {};
  const notifier = notifierFlatFromSources(profile, agent);
  const { notifier: _notifier, ...requestOptionsWithoutNotifier } = requestOptions;
  const notifierProfile = notifierProfileSummaryFlags(profile, agent);
  const modelProviderID = String(profile?.model_provider_id || "").trim() || providerIDForProvider(profile?.provider);
  return {
    runtime_kind: normalizeRuntimeKind(profile?.runtime_kind),
    runtime_name: normalizeRuntimeName(profile?.runtime_name),
    sandbox_enabled: Boolean(profile?.sandbox_enabled),
    provider: profile?.provider || providerNameForProviderID(modelProviderID) || DEFAULT_PROVIDER,
    model_provider_id: modelProviderID,
    base_url: profile?.base_url || "",
    api_key: "",
    api_key_set: Boolean(profile?.api_key_set),
    api_key_preview: profile?.api_key_preview || "",
    model_id: profile?.model_id || "",
    reasoning_effort: profile?.reasoning_effort || DEFAULT_REASONING_EFFORT,
    enable_fast_mode: Boolean(profile?.enable_fast_mode),
    headersText: stringifyJSON(profile?.headers || {}),
    requestOptionsText: stringifyJSON(requestOptionsWithoutNotifier),
    envRows: mapToEnvRows(profile?.env || {}),
    notifier_delivery_mode: normalizeNotifierDeliveryMode(notifier.delivery_mode || "webhook"),
    notifier_webhook_token: String(notifier.webhook_token || ""),
    notifier_remote_url: String(notifier.remote_url || ""),
    notifier_remote_subscription_id: String(notifier.remote_subscription_id || ""),
    notifier_poll_interval: String(notifier.poll_interval || DEFAULT_NOTIFIER_POLL_INTERVAL),
    notifier_remote_token: String(notifier.remote_token || ""),
    notifier_remote_messages_url: String(notifier.remote_messages_url || ""),
    notifier_remote_ack_url: String(notifier.remote_ack_url || ""),
    ...notifierProfile,
  };
}

export function modelRequestKey(draft: Partial<AgentDraft> | null | undefined): string {
  if (!draft) {
    return "";
  }
  return JSON.stringify({
    agent_id: draft.agent_id || "",
    provider: draft.provider || "",
    model_provider_id: draft.model_provider_id || "",
    base_url: draft.base_url || "",
    api_key: draft.api_key || "",
    headersText: draft.headersText || "",
  });
}

export function notifierConfiguredFromFlatDetails(flat: unknown): boolean {
  if (!flat || typeof flat !== "object" || Array.isArray(flat)) {
    return false;
  }
  const record = flat as JSONRecord;
  const deliveryRaw = String(record.delivery_mode ?? "")
    .trim()
    .toLowerCase();
  const mode = deliveryRaw === "remote_pull" ? "remote_pull" : deliveryRaw === "both" ? "both" : "webhook";
  const webhookToken = String(record.webhook_token ?? "").trim();
  const remoteURL = String(record.remote_url ?? "").trim();
  const allowsWebhook = (mode === "webhook" || mode === "both") && webhookToken !== "";
  const allowsPull = remoteURL !== "" && (mode === "remote_pull" || mode === "both");
  return allowsWebhook || allowsPull;
}

export function notifierDeliveryConfiguredInProfile(
  profile: AgentProfileLike | null | undefined,
  agent?: AgentLike | null,
): boolean {
  return notifierConfiguredFromFlatDetails(notifierFlatFromSources(profile, agent));
}

export function agentToDraft(agent: AgentDraftSource | null | undefined): AgentDraft {
  const profile = agentProfileConfig(agent as AgentLike | null | undefined) || {};
  const botType = normalizeBotType(agent?.bot_type ?? agent?.type);
  const base = profileToDraft(profile, agent);
  return {
    agent_id: agent?.id || "",
    name: agent?.name || "",
    role: agent?.role || WORKER_AGENT_ROLE,
    bot_type: botType,
    description: agent?.description || profile.description || "",
    instructions: agent?.instructions || "",
    avatar: normalizeAvatarPath(agent?.avatar),
    default_image: agent?.image || "",
    image: agent?.image || "",
    from_template: agent?.from_template || "",
    template_name: agent?.template_name || "",
    runtime_options: normalizeRuntimeOptionsRecord(agentRuntimeOptions(agent as AgentLike | null | undefined)),
    mcpServers: agentMCPServers(agent as AgentLike | null | undefined),
    ...base,
    notifier_delivery_mode: normalizeNotifierDeliveryMode(agent?.notifier_delivery_mode || base.notifier_delivery_mode),
    runtime_name: agentRuntimeName(agent as AgentLike | null | undefined),
    sandbox_enabled: agentSandboxEnabled(agent as AgentLike | null | undefined),
    runtime_kind: normalizeRuntimeKind(agentRuntimeKind(agent as AgentLike | null | undefined) || profile.runtime_kind),
  };
}

export function normalizeTemplateSelection<T extends object>(template: T | null | undefined): T | null {
  return template && typeof template === "object" ? template : null;
}

// Legacy contract: function templateMatchesRuntime(template, runtimeKind)
export function templateMatchesRuntime(template: AgentTemplateLike | null | undefined, runtimeKind: unknown): boolean {
  const requestedRuntime = normalizeRuntimeKind(runtimeKind);
  if (!template || !requestedRuntime) {
    return true;
  }
  const templateRuntime = normalizeRuntimeKind(template.runtime_kind);
  return !templateRuntime || templateRuntime === requestedRuntime;
}

export function workerSelectableTemplates(
  templates: readonly AgentTemplateLike[] | null | undefined,
): AgentTemplateLike[] {
  if (!Array.isArray(templates) || templates.length === 0) {
    return [];
  }
  return templates.filter((item) => {
    const role = String(item?.role ?? "")
      .trim()
      .toLowerCase();
    return role !== MANAGER_AGENT_ROLE;
  });
}

export function pickDefaultAgentTemplate(
  templates: readonly AgentTemplateLike[] | null | undefined,
  runtimeKind = "",
  bootstrapConfig: RuntimeBootstrapConfig | null = null,
): AgentTemplateLike | null {
  const selectableTemplates = workerSelectableTemplates(templates);
  if (selectableTemplates.length === 0) {
    return null;
  }
  const requestedRuntime = normalizeRuntimeKind(runtimeKind || bootstrapConfig?.runtime_kind);
  const candidates = requestedRuntime
    ? selectableTemplates.filter((item) => templateMatchesRuntime(item, requestedRuntime))
    : selectableTemplates.slice();
  if (!candidates.length) {
    return null;
  }
  if (requestedRuntime === BOT_TYPE_NOTIFICATION) {
    return null;
  }
  const configuredDefault = String(bootstrapConfig?.default_worker_template || "").trim();
  if (configuredDefault) {
    const configured = candidates.find((item) => item.id === configuredDefault);
    if (configured) {
      return configured;
    }
  }
  if (requestedRuntime === "openclaw_sandbox") {
    return (
      candidates.find((item) => item.id === "builtin.openclaw-worker") ||
      candidates.find((item) => item.name === "openclaw-worker") ||
      candidates.find((item) => String(item.id || "").endsWith(".openclaw-worker")) ||
      candidates[0]
    );
  }
  if (requestedRuntime === DEFAULT_RUNTIME_KIND || !requestedRuntime) {
    return (
      candidates.find((item) => item.id === "builtin.picoclaw-worker") ||
      candidates.find((item) => item.name === "picoclaw-worker") ||
      candidates.find((item) => String(item.id || "").endsWith(".picoclaw-worker")) ||
      candidates[0]
    );
  }
  return candidates[0];
}

export function applyTemplateToDraft(
  draft: AgentDraft,
  template: AgentTemplateLike | null | undefined,
  bootstrapConfig: RuntimeBootstrapConfig | null | undefined,
  fallbackImage?: string,
): AgentDraft;
export function applyTemplateToDraft(
  draft: null | undefined,
  template: AgentTemplateLike | null | undefined,
  bootstrapConfig: RuntimeBootstrapConfig | null | undefined,
  fallbackImage?: string,
): null | undefined;
export function applyTemplateToDraft(
  draft: AgentDraft | null | undefined,
  template: AgentTemplateLike | null | undefined,
  bootstrapConfig: RuntimeBootstrapConfig | null | undefined,
  fallbackImage = "",
): AgentDraft | null | undefined {
  if (!draft) {
    return draft;
  }
  if (!template) {
    return {
      ...draft,
      from_template: "",
      template_name: "",
    };
  }
  const runtimeKind = normalizeRuntimeKind(
    template.runtime_kind || draft.runtime_kind || bootstrapConfig?.runtime_kind,
  );
  const runtimeSelection = resolveRuntimeSelection({ runtime_kind: runtimeKind });
  return {
    ...draft,
    from_template: template.id || "",
    template_name: template.name || template.id || "",
    name: template.name || draft.name || "",
    runtime_kind: runtimeSelection.runtime_kind,
    runtime_name: runtimeSelection.runtime_name,
    sandbox_enabled: runtimeSelection.sandbox_enabled,
    image:
      template.image || runtimeImageForKind(runtimeKind, bootstrapConfig, fallbackImage || draft.default_image || ""),
    description: template.description || draft.description || "",
    envRows: mapImageEnvToRows(template.image_env),
  };
}

export function draftToProfile(draft: AgentDraft, options: DraftProfileOptions = {}): JSONRecord {
  void options;
  const requestOptions = parseJSONMap(draft.requestOptionsText);
  const modelProviderID = String(draft.model_provider_id || "").trim();
  return {
    model_provider_id: modelProviderID,
    base_url: "",
    api_key: "",
    model_id: draft.model_id,
    reasoning_effort: draft.reasoning_effort || DEFAULT_REASONING_EFFORT,
    enable_fast_mode: Boolean(draft.enable_fast_mode),
    headers: {},
    request_options: requestOptions,
    env: envRowsToMap(draft.envRows),
  };
}

export function draftToProfileComparePayload(draft: AgentDraft, options: DraftProfileOptions = {}): JSONRecord {
  void options;
  const requestOptions = parseJSONMapForCompare(draft.requestOptionsText);
  const modelProviderID = String(draft.model_provider_id || "").trim();
  return {
    model_provider_id: modelProviderID,
    base_url: "",
    api_key: "",
    model_id: draft.model_id,
    reasoning_effort: draft.reasoning_effort || DEFAULT_REASONING_EFFORT,
    enable_fast_mode: Boolean(draft.enable_fast_mode),
    headers: {},
    request_options: requestOptions,
    env: envRowsToMapForCompare(draft.envRows),
  };
}

export function draftNotifierDetailsFromDraft(draft: Partial<AgentDraft> | null | undefined): JSONRecord | null {
  if (!draft) {
    return null;
  }
  return {
    delivery_mode: normalizeNotifierDeliveryMode(draft.notifier_delivery_mode || "webhook"),
    webhook_token: String(draft.notifier_webhook_token ?? "").trim(),
    remote_url: String(draft.notifier_remote_url ?? "").trim(),
    remote_subscription_id: String(draft.notifier_remote_subscription_id ?? "").trim(),
    poll_interval: String(draft.notifier_poll_interval ?? DEFAULT_NOTIFIER_POLL_INTERVAL).trim(),
    remote_token: String(draft.notifier_remote_token ?? "").trim(),
    remote_messages_url: String(draft.notifier_remote_messages_url ?? "").trim(),
    remote_ack_url: String(draft.notifier_remote_ack_url ?? "").trim(),
  };
}

export function draftNotifierRuntimeOptionsForSave(
  draft: Partial<AgentDraft> | null | undefined,
  options: { mergeNotifier?: boolean } = {},
): JSONRecord | null {
  const mergeNotifier = Boolean(options.mergeNotifier) || isNotifierRuntimeDraft(draft);
  if (!mergeNotifier) {
    return null;
  }
  const notifier = draftNotifierDetailsFromDraft(draft);
  if (!notifier || Object.keys(notifier).length === 0) {
    return null;
  }
  return { ...notifier };
}

export function runtimeOptionValueForPath(
  runtimeOptions: JSONRecord | null | undefined,
  path: string | null | undefined,
): string {
  const key = String(path ?? "").trim();
  if (!key) {
    return "";
  }
  return String(normalizeRuntimeOptionsRecord(runtimeOptions)[key] ?? "");
}

export function setRuntimeOptionValue(
  runtimeOptions: JSONRecord | null | undefined,
  path: string | null | undefined,
  value: string,
): JSONRecord {
  const key = String(path ?? "").trim();
  const next = normalizeRuntimeOptionsRecord(runtimeOptions);
  if (!key) {
    return next;
  }
  next[key] = value;
  return next;
}

export function runtimeOptionSchemasForAgent(
  runtimeKind: unknown,
  item?: AgentLike | null,
  bootstrapConfig?: RuntimeBootstrapConfig | null,
): RuntimeOptionSchema[] {
  if (isManagerAgent(item)) {
    return [];
  }
  const fromRuntime = normalizeRuntimeOptionSchemas(item?.runtime?.option_schemas);
  if (fromRuntime.length > 0) {
    return fromRuntime;
  }
  const fromAgent = normalizeRuntimeOptionSchemas(item?.runtime_option_schemas);
  if (fromAgent.length > 0) {
    return fromAgent;
  }
  const kind = normalizeRuntimeKind(runtimeKind);
  if (!kind) {
    return [];
  }
  return normalizeRuntimeOptionSchemaMap(bootstrapConfig?.runtime_option_schemas)[kind] || [];
}

function trimmedRuntimeOptionsRecord(runtimeOptions: JSONRecord | null | undefined): JSONRecord | null {
  const source = normalizeRuntimeOptionsRecord(runtimeOptions);
  const out: JSONRecord = {};
  for (const [key, rawValue] of Object.entries(source)) {
    const normalizedKey = String(key ?? "").trim();
    if (!normalizedKey || rawValue == null) {
      continue;
    }
    if (typeof rawValue === "string") {
      const text = rawValue.trim();
      if (!text) {
        continue;
      }
      out[normalizedKey] = text;
      continue;
    }
    out[normalizedKey] = rawValue;
  }
  return Object.keys(out).length > 0 ? out : null;
}

export function draftRuntimeOptionsForSave(
  draft: Partial<AgentDraft> | null | undefined,
  options: { mergeNotifier?: boolean } = {},
): JSONRecord | null {
  const base = trimmedRuntimeOptionsRecord(draft?.runtime_options);
  const notifier = draftNotifierRuntimeOptionsForSave(draft, options);
  if (!base && !notifier) {
    return null;
  }
  return { ...(base || {}), ...(notifier || {}) };
}

export function draftMCPServersForSave(draft: Partial<AgentDraft> | null | undefined): JSONRecord | null | undefined {
  if (!draft || draft.mcpServers === undefined) {
    return undefined;
  }
  if (draft.mcpServers === null) {
    return null;
  }
  return normalizeRuntimeOptionsRecord(draft.mcpServers);
}

export function notifierRemoteTokenPlaceholderText(
  draft: Partial<AgentDraft> | null | undefined,
  t: TranslateFn,
): string {
  if (String(draft?.notifier_remote_token ?? "").trim()) {
    return "";
  }
  if (draft?.notifier_remote_token_set) {
    return t("notifierRemoteTokenLeaveUnchangedPlaceholder");
  }
  return t("notifierRemoteTokenInputPlaceholder");
}

export function notifierFormIsComplete(
  draft: Partial<AgentDraft> | null | undefined,
  item?: AgentLike | null,
): boolean {
  const hasItem = item != null && typeof item === "object";
  const isNotifier = hasItem ? isNotifierRuntimeDraftOnAgentPage(draft, item) : isNotifierRuntimeDraft(draft);
  if (!draft || !isNotifier) {
    return true;
  }
  if (draft.notifier_delivery_complete || draft.notifier_webhook_token_set || draft.notifier_remote_token_set) {
    return true;
  }
  const profile = agentProfileConfig(item);
  const runtimeOptions = agentRuntimeOptions(item);
  const draftProfile = {
    request_options: {
      notifier: {
        delivery_mode: draft.notifier_delivery_mode,
        webhook_token: draft.notifier_webhook_token,
        remote_url: draft.notifier_remote_url,
        remote_token: draft.notifier_remote_token,
      },
    },
  };
  if (notifierDeliveryConfiguredInProfile(draftProfile as AgentProfileLike)) {
    return true;
  }
  if (hasItem && notifierDeliveryConfiguredInProfile(profile, item)) {
    return true;
  }
  const profileRuntimeOptions =
    profile?.runtime_options && typeof profile.runtime_options === "object" && !Array.isArray(profile.runtime_options)
      ? profile.runtime_options
      : {};
  const runtimeSummary =
    runtimeOptions?.notification_profile &&
    typeof runtimeOptions.notification_profile === "object" &&
    !Array.isArray(runtimeOptions.notification_profile)
      ? (runtimeOptions.notification_profile as JSONRecord)
      : runtimeOptions?.notifier_profile &&
          typeof runtimeOptions.notifier_profile === "object" &&
          !Array.isArray(runtimeOptions.notifier_profile)
        ? (runtimeOptions.notifier_profile as JSONRecord)
        : profileRuntimeOptions?.notification_profile &&
            typeof profileRuntimeOptions.notification_profile === "object" &&
            !Array.isArray(profileRuntimeOptions.notification_profile)
          ? (profileRuntimeOptions.notification_profile as JSONRecord)
          : profileRuntimeOptions?.notifier_profile &&
              typeof profileRuntimeOptions.notifier_profile === "object" &&
              !Array.isArray(profileRuntimeOptions.notifier_profile)
            ? (profileRuntimeOptions.notifier_profile as JSONRecord)
            : null;
  const legacySummary =
    profile?.notifier_profile &&
    typeof profile.notifier_profile === "object" &&
    !Array.isArray(profile.notifier_profile)
      ? profile.notifier_profile
      : null;
  const summary = runtimeSummary || legacySummary;
  if (hasItem && Boolean(summary?.webhook_token_set)) {
    return true;
  }
  if (hasItem && Boolean(summary?.remote_token_set)) {
    return true;
  }
  return false;
}

export function mapToEnvRows(value: unknown): EnvKeyValueRow[] {
  const object = value && typeof value === "object" && !Array.isArray(value) ? value : {};
  const entries = Object.entries(object).sort(([left], [right]) => left.localeCompare(right));
  if (entries.length === 0) {
    return [{ key: "", value: "" }];
  }
  return entries.map(([key, val]) => ({ key, value: String(val ?? "") }));
}

export function mapImageEnvToRows(contracts: readonly ImageEnvContract[] | null | undefined): EnvKeyValueRow[] {
  if (!contracts?.length) {
    return [{ key: "", value: "" }];
  }
  return contracts.map((contract) => ({
    key: String(contract.name || "").trim(),
    required: Boolean(contract.required),
    value: contract.secret ? "" : String(contract.default ?? ""),
  }));
}

export function agentDraftMissingRequiredEnv(draft: Pick<AgentDraft, "envRows"> | null | undefined): boolean {
  return Boolean(
    draft?.envRows?.some((row) => {
      const required = Boolean(row?.required);
      if (!required) {
        return false;
      }
      return !String(row?.value ?? "").trim();
    }),
  );
}

export function envRowsToMap(rows: readonly EnvKeyValueRow[] | null | undefined): Record<string, string> {
  const result: Record<string, string> = {};
  const seen = new Set<string>();
  for (const row of rows ?? []) {
    const key = String(row?.key ?? "").trim();
    const value = String(row?.value ?? "");
    if (!key && !value.trim()) {
      continue;
    }
    if (!key) {
      throw new Error("Environment variable key is required");
    }
    if (!value.trim()) {
      continue;
    }
    const normalized = key.toUpperCase();
    if (seen.has(normalized)) {
      throw new Error(`Duplicate environment variable: ${key}`);
    }
    seen.add(normalized);
    result[key] = value;
  }
  return result;
}

function envRowsToMapForCompare(rows: readonly EnvKeyValueRow[] | null | undefined): JSONRecord {
  const result: Record<string, string> = {};
  const invalidRows: EnvKeyValueRow[] = [];
  const seen = new Set<string>();
  for (const row of rows ?? []) {
    const key = String(row?.key ?? "").trim();
    const value = String(row?.value ?? "");
    if (!key && !value.trim()) {
      continue;
    }
    if (!key) {
      invalidRows.push({ key, value });
      continue;
    }
    if (!value.trim()) {
      continue;
    }
    const normalized = key.toUpperCase();
    if (seen.has(normalized)) {
      invalidRows.push({ key, value });
      continue;
    }
    seen.add(normalized);
    result[key] = value;
  }
  if (invalidRows.length === 0) {
    return result;
  }
  return {
    ...result,
    __invalid_env_rows: invalidRows,
  };
}

export function isAgentRunning(item: AgentLike | null | undefined): boolean {
  if (isNotificationBotAgent(item)) {
    return item?.available === true;
  }
  const status = agentRuntimeState(item).toLowerCase();
  return status === "running" || status === "online";
}

export function isAgentProfileMarkedComplete(item: AgentLike | null | undefined): boolean {
  const profile = agentProfileConfig(item);
  return item?.profile_complete === true || profile?.profile_complete === true;
}

export function isAgentProfileDraftComplete(draft: Partial<AgentDraft> | null | undefined): boolean {
  if (!String(draft?.model_id ?? "").trim()) {
    return false;
  }
  const modelProviderID = String(draft?.model_provider_id ?? "").trim();
  return Boolean(modelProviderID);
}

export function profileSelectorFromDraft(draft: Partial<AgentDraft> | null | undefined): string {
  const providerID = String(draft?.model_provider_id ?? "").trim();
  if (!providerID) {
    return "";
  }
  return selectorForProviderModel(providerID, String(draft?.model_id ?? "").trim());
}

export function llmProfilePayloadForCompare(draft: AgentDraft | null | undefined): string {
  if (!draft) {
    return "";
  }
  const normalized = ensureNotifierPullSubscriptionDraft(draft);
  const profile = draftToProfileComparePayload(normalized, {
    name: normalized.name,
    description: normalized.description,
  });
  return JSON.stringify({
    provider: profile.provider,
    model_provider_id: profile.model_provider_id,
    base_url: profile.base_url,
    api_key: profile.api_key,
    model_id: profile.model_id,
    reasoning_effort: profile.reasoning_effort,
    enable_fast_mode: profile.enable_fast_mode,
    headers: profile.headers,
    request_options: profile.request_options,
    env: profile.env,
  });
}

export function agentPageLLMProfileChanged(
  draft: AgentDraft | null | undefined,
  savedDraft: AgentDraft | null | undefined,
): boolean {
  return llmProfilePayloadForCompare(draft) !== llmProfilePayloadForCompare(savedDraft);
}

export function agentProfilePageSaveDisabled(
  draft: AgentDraft | null | undefined,
  item: AgentLike | null | undefined,
  options: { saving?: boolean; savedDraft?: AgentDraft | null } = {},
): boolean {
  if (options.saving || !draft) {
    return true;
  }
  if (!String(draft.name ?? "").trim()) {
    return true;
  }
  if (isNotifierRuntimeDraftOnAgentPage(draft, item)) {
    return !notifierFormIsComplete(draft, item);
  }
  if (!agentPageLLMProfileChanged(draft, options.savedDraft ?? null)) {
    return false;
  }
  return !isAgentProfileDraftComplete(draft);
}

export function isAgentIncomplete(
  item: AgentLike | null | undefined,
  draftOverride?: AgentDraft | null | undefined,
): boolean {
  if (isAgentProfileMarkedComplete(item)) {
    return false;
  }
  if (isNotificationBotAgent(item) && item?.available === true) {
    return false;
  }
  const draft = draftOverride ?? agentToDraft(item);
  if (isNotifierRuntimeDraftOnAgentPage(draft, item)) {
    return !notifierFormIsComplete(draft, item);
  }
  if (isAgentProfileDraftComplete(draft)) {
    return false;
  }
  const profile = agentProfileConfig(item);
  return item?.profile_complete === false || profile?.profile_complete === false;
}

export function isAgentRestartNeeded(item: AgentLike | null | undefined): boolean {
  const profile = agentProfileConfig(item);
  return Boolean(item?.env_restart_required || profile?.env_restart_required);
}

export function shouldWaitForManagerRuntimeAfterProfileSave(
  agent: AgentLike | null | undefined,
  options: { profileIncompleteBeforeSave?: boolean } = {},
): boolean {
  if (options.profileIncompleteBeforeSave) {
    return true;
  }
  if (!isAgentProfileMarkedComplete(agent)) {
    return true;
  }
  if (!agentRuntimeSandboxID(agent)) {
    return true;
  }
  if (agentRuntimeState(agent).toLowerCase() === "profile_incomplete") {
    return true;
  }
  return false;
}

export function agentRuntimePollSettled(item: AgentLike | null | undefined): boolean {
  if (isAgentRunning(item)) {
    return true;
  }
  const status = agentRuntimeState(item).toLowerCase();
  if (status === "stopped" || status === "offline" || status === "failed" || status === "error") {
    return true;
  }
  if (
    isAgentProfileMarkedComplete(item) &&
    status !== "profile_incomplete" &&
    status !== "starting" &&
    status !== "provisioning"
  ) {
    return true;
  }
  return false;
}

export function isAgentUpgradeNeeded(item: AgentLike | null | undefined): boolean {
  if (agentRuntimeKind(item) === "codex") {
    return false;
  }
  const profile = agentProfileConfig(item);
  return Boolean(item?.image_upgrade_required || profile?.image_upgrade_required);
}

export function agentModelID(item: AgentLike | null | undefined): string {
  const profile = agentProfileConfig(item);
  return item?.model_id || profile?.model_id || "no model";
}

export function stringifyJSON(value: unknown): string {
  const object = value && typeof value === "object" ? value : {};
  return JSON.stringify(object, null, 2);
}

export function parseJSONMap(text: unknown): JSONRecord {
  const cleaned = String(text ?? "").trim();
  if (!cleaned) {
    return {};
  }
  const parsed = JSON.parse(cleaned);
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error("Expected a JSON object");
  }
  return parsed as JSONRecord;
}

function parseJSONMapForCompare(text: unknown): JSONRecord {
  try {
    return parseJSONMap(text);
  } catch {
    return {
      __invalid_json_text: String(text ?? ""),
    };
  }
}

export function normalizeAuthProviderName(provider: unknown): string {
  const value = String(provider ?? "")
    .trim()
    .toLowerCase();
  if (value === "claude" || value === "claude-code") {
    return "claude_code";
  }
  return value;
}

export function providerNeedsAuth(provider: unknown): boolean {
  return CLIPROXY_AUTH_PROVIDERS.has(normalizeAuthProviderName(provider));
}

export function formatProviderLabel(provider: ProviderName | null | undefined): string {
  return PROVIDER_OPTIONS.find((option) => option.value === provider)?.label || provider || "";
}

export function normalizeRuntimeKind(kind: unknown): RuntimeKind {
  const value = String(kind ?? "")
    .trim()
    .toLowerCase();
  if (value === "") {
    return "";
  }
  switch (value) {
    case "openclaw":
      return "openclaw_sandbox";
    case "openclaw_sandbox":
      return "openclaw_sandbox";
    case "picoclaw":
      return "picoclaw_sandbox";
    case "codex":
      return "codex";
    case "picoclaw_sandbox":
      return "picoclaw_sandbox";
    default:
      return value;
  }
}

export function isNotifierRuntimeDraft(draft: Partial<AgentDraft> | null | undefined): boolean {
  return normalizeBotType(draft?.bot_type) === BOT_TYPE_NOTIFICATION;
}

export function effectiveAgentRuntimeKind(
  draft: Partial<AgentDraft> | null | undefined,
  item: AgentLike | null | undefined,
): RuntimeKind {
  return normalizeRuntimeKind(draft?.runtime_kind || agentRuntimeKind(item) || "");
}

export function isNotifierRuntimeDraftOnAgentPage(
  draft: Partial<AgentDraft> | null | undefined,
  item: AgentLike | null | undefined,
): boolean {
  if (normalizeBotType(draft?.bot_type) === BOT_TYPE_NOTIFICATION) {
    return true;
  }
  return isNotificationBotAgent(item);
}

/** Create-modal tab context: worker vs notification (overrides draft bot_type on create). */
export function isNotificationBotDraftContext(
  draft: Partial<AgentDraft> | null | undefined,
  item?: AgentLike | null,
  createBotKind?: string,
): boolean {
  if (createBotKind === BOT_CREATE_KIND_NOTIFICATION) {
    return true;
  }
  if (createBotKind === BOT_CREATE_KIND_WORKER) {
    return false;
  }
  return isNotifierRuntimeDraftOnAgentPage(draft, item ?? null);
}

/** When a hub template is selected on create, runtime and image come from the template. */
export function agentCreateTemplateLocked(draft: Partial<AgentDraft> | null | undefined, modalMode: string): boolean {
  if (modalMode !== "create") {
    return false;
  }
  return Boolean(String(draft?.from_template ?? "").trim());
}

export function normalizeRuntimeImageMap(value: unknown): Partial<Record<RuntimeKind, string>> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const out: Partial<Record<RuntimeKind, string>> = {};
  for (const [key, image] of Object.entries(value)) {
    const runtimeKind = normalizeRuntimeKind(key);
    const trimmed = String(image ?? "").trim();
    if (runtimeKind && trimmed) {
      out[runtimeKind] = trimmed;
    }
  }
  return out;
}

export function runtimeImageForKind(
  kind: unknown,
  bootstrapConfig: RuntimeBootstrapConfig | null | undefined,
  fallbackImage = "",
): string {
  let runtimeKind = normalizeRuntimeKind(kind);
  if (!runtimeKind) {
    runtimeKind = DEFAULT_RUNTIME_KIND;
  }
  if (runtimeKind === "codex" || runtimeKind === BOT_TYPE_NOTIFICATION) {
    return "";
  }
  const images = normalizeRuntimeImageMap(bootstrapConfig?.runtime_default_images);
  const configuredImage = images[runtimeKind];
  if (configuredImage) {
    return configuredImage;
  }
  if (normalizeRuntimeKind(bootstrapConfig?.runtime_kind) === runtimeKind && bootstrapConfig?.effective_manager_image) {
    return String(bootstrapConfig.effective_manager_image).trim();
  }
  return String(fallbackImage ?? "").trim();
}

export function defaultWorkerImageForRuntime(
  templates: readonly AgentTemplateLike[] | null | undefined,
  runtimeKind: unknown,
  bootstrapConfig: RuntimeBootstrapConfig | null | undefined,
  fallbackImage = "",
): string {
  const configuredImage = runtimeImageForKind(runtimeKind, bootstrapConfig, "");
  if (configuredImage) {
    return configuredImage;
  }
  const templateImage = String(
    pickDefaultAgentTemplate(templates, normalizeRuntimeKind(runtimeKind), bootstrapConfig)?.image ?? "",
  ).trim();
  if (templateImage) {
    return templateImage;
  }
  return String(fallbackImage ?? "").trim();
}

export function availableManagerRuntimeOptions(_bootstrapConfig: RuntimeBootstrapConfig | null | undefined) {
  return RUNTIME_KIND_OPTIONS.filter((option) => option.value === "codex");
}

export function collectManagerTemplateVariants(
  templates: readonly AgentTemplateLike[] | null | undefined,
): ManagerTemplateVariant[] {
  if (!Array.isArray(templates) || templates.length === 0) {
    return [];
  }
  const out: ManagerTemplateVariant[] = [];
  const seen = new Set<string>();
  for (const item of templates) {
    if (
      String(item?.role ?? "")
        .trim()
        .toLowerCase() !== MANAGER_AGENT_ROLE
    ) {
      continue;
    }
    const runtimeKind = normalizeRuntimeKind(item?.runtime_kind);
    const image = String(item?.image ?? "").trim();
    if (!runtimeKind && !image) {
      continue;
    }
    const key = `${runtimeKind}\n${image}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push({ runtimeKind, image });
  }
  return out;
}

export function availableManagerRebuildRuntimeOptions(
  _variants: readonly ManagerTemplateVariant[] | null | undefined,
  _bootstrapConfig: RuntimeBootstrapConfig | null | undefined,
  _currentRuntimeKind = "",
) {
  const values: RuntimeKind[] = [];
  const seen = new Set<string>();
  const push = (kind: unknown) => {
    const normalized = normalizeRuntimeKind(kind);
    if (!normalized || normalized !== "codex" || seen.has(normalized)) {
      return;
    }
    seen.add(normalized);
    values.push(normalized);
  };
  push("codex");
  if (!values.length) {
    push("codex");
  }
  return values.map((value) => ({ value, label: value }));
}

export function defaultManagerRebuildImageForRuntime(
  variants: readonly ManagerTemplateVariant[] | null | undefined,
  runtimeKind: unknown,
  bootstrapConfig: RuntimeBootstrapConfig | null | undefined,
  fallbackImage = "",
): string {
  const selectedRuntime = normalizeRuntimeKind(runtimeKind) || "codex";
  if (selectedRuntime === "codex") {
    return "";
  }
  if (Array.isArray(variants)) {
    for (const item of variants) {
      if (selectedRuntime && normalizeRuntimeKind(item?.runtimeKind) !== selectedRuntime) {
        continue;
      }
      const image = String(item?.image ?? "").trim();
      if (image) {
        return image;
      }
    }
  }
  if (selectedRuntime && normalizeRuntimeKind(bootstrapConfig?.runtime_kind) === selectedRuntime) {
    const effectiveManagerImage = String(bootstrapConfig?.effective_manager_image ?? "").trim();
    if (effectiveManagerImage) {
      return effectiveManagerImage;
    }
  }
  return String(fallbackImage ?? "").trim();
}

export function agentCreateProgressSteps(runtimeKind: unknown): AgentCreateProgressStep[] {
  const kind = normalizeRuntimeKind(runtimeKind) || DEFAULT_RUNTIME_KIND;
  if (kind === BOT_TYPE_NOTIFICATION) {
    return [
      { label: "agentCreateProgressPreparing", target: 40 },
      { label: "agentCreateProgressFinishing", target: 96 },
    ];
  }
  if (kind === "openclaw_sandbox" || kind === DEFAULT_RUNTIME_KIND) {
    return [
      { label: "agentCreateProgressSandboxConfig", target: 16 },
      { label: "agentCreateProgressImage", target: 42 },
      { label: "agentCreateProgressRuntime", target: 72 },
      { label: "agentCreateProgressStart", target: 88 },
      { label: "agentCreateProgressFinishing", target: 96 },
    ];
  }
  return [
    { label: "agentCreateProgressPreparing", target: 24 },
    { label: "agentCreateProgressRuntime", target: 66 },
    { label: "agentCreateProgressStart", target: 88 },
    { label: "agentCreateProgressFinishing", target: 96 },
  ];
}

export function startAgentCreateProgress(runtimeKind: unknown): AgentCreateProgressState {
  const steps = agentCreateProgressSteps(runtimeKind);
  return {
    steps,
    index: 0,
    percent: 4,
    status: "running",
    startedAt: Date.now(),
  };
}

export function advanceAgentProgress<T extends AgentCreateProgressState | null | undefined>(
  current: T,
): T | AgentCreateProgressState {
  if (!current || current.status !== "running" || !current.steps?.length) {
    return current;
  }
  const step = current.steps[Math.min(current.index, current.steps.length - 1)];
  const target = step?.target ?? 96;
  if (current.percent < target) {
    const delta = Math.max(1, Math.ceil((target - current.percent) / 3));
    return { ...current, percent: Math.min(target, current.percent + delta) };
  }
  if (current.index < current.steps.length - 1) {
    return { ...current, index: current.index + 1 };
  }
  return { ...current, percent: Math.min(96, current.percent) };
}

export function formatRuntimeKindLabel(kind: unknown, t: TranslateFn): string {
  const runtimeKind = normalizeRuntimeKind(kind);
  if (!runtimeKind) {
    return t("runtimePicoclaw");
  }
  switch (runtimeKind) {
    case "openclaw_sandbox":
      return t("runtimeOpenclaw");
    case "codex":
      return t("runtimeCodexCLI");
    case "picoclaw_sandbox":
      return t("runtimePicoclaw");
    default:
      return runtimeKind;
  }
}
