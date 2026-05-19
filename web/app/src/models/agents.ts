import {
  CLIPROXY_AUTH_PROVIDERS,
  NOTIFIER_RELAY_WEBHOOK_INGRESS_PATH,
  RUNTIME_KIND_OPTIONS,
} from "@/bootstrap/constants";

export type RuntimeKind = "picoclaw_sandbox" | "openclaw_sandbox" | "codex" | "notifier" | string;
export type ProviderName = "csghub_lite" | "codex" | "claude_code" | "api" | string;
export type JSONRecord = Record<string, unknown>;

export type EnvKeyValueRow = {
  key: string;
  value: string;
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
  model_id?: string | null;
  profile_complete?: boolean | null;
  provider?: ProviderName | null;
  reasoning_effort?: string | null;
  request_options?: JSONRecord | null;
  runtime_options?: JSONRecord | null;
  runtime_kind?: string | null;
  notifier_profile?: JSONRecord | null;
};

export type AgentLike = AgentProfileLike & {
  agent_profile?: AgentProfileLike | null;
  default_image?: string | null;
  from_template?: string | null;
  id?: string | null;
  image?: string | null;
  name?: string | null;
  role?: string | null;
  runtime_options?: JSONRecord | null;
  status?: string | null;
  template_name?: string | null;
};

export type AgentDraft = {
  agent_id?: string;
  api_key: string;
  api_key_preview: string;
  api_key_set: boolean;
  base_url: string;
  default_image?: string;
  description?: string;
  enable_fast_mode: boolean;
  envRows: EnvKeyValueRow[];
  from_template?: string;
  headersText: string;
  image?: string;
  model_id: string;
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
  runtime_kind: RuntimeKind;
  template_name?: string;
};

export type AgentTemplateLike = {
  description?: string | null;
  id?: string | null;
  image?: string | null;
  name?: string | null;
  runtime_kind?: string | null;
};

export type RuntimeBootstrapConfig = {
  default_worker_template?: string | null;
  effective_manager_image?: string | null;
  runtime_default_images?: unknown;
  runtime_kind?: string | null;
  supported_runtime_kinds?: unknown;
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

export function isManagerAgent(item: AgentLike | null | undefined): boolean {
  return item?.role === "manager" || item?.id === "u-manager";
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

export function notifierPushWebhookPathForAgent(agentID: unknown): string {
  const id = String(agentID || "").trim();
  if (!id) {
    return "/api/v1/notify/<agent_id>";
  }
  return `/api/v1/notify/${encodeURIComponent(id)}`;
}

export function notifierPushWebhookNotifyURL(
  origin: unknown,
  agentID: unknown,
  placeholderHost = "https://<your-csgclaw-host>",
): string {
  let base = String(origin ?? "")
    .trim()
    .replace(/\/+$/, "");
  if (!base) {
    base = String(placeholderHost || "https://<your-csgclaw-host>").trim();
  }
  return `${base}${notifierPushWebhookPathForAgent(agentID)}`;
}

export function notifierThirdPartyRelayWebhookURL(remoteBase: unknown, subscriptionID: unknown): string {
  const base = String(remoteBase ?? "").trim();
  const sid = String(subscriptionID ?? "").trim();
  if (!base || !sid) {
    return "";
  }
  let input = base;
  if (!/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(input)) {
    const local = /^(localhost|127\.0\.0\.1|\[::1\])(:|\/?|\?|$)/i.test(input) || /^\[::1\]/i.test(input);
    input = `${local ? "http" : "https"}://${input.replace(/^\/+/, "")}`;
  }
  try {
    const url = new URL(input);
    if (!url.pathname || url.pathname === "/") {
      url.pathname = NOTIFIER_RELAY_WEBHOOK_INGRESS_PATH;
    } else if (/\/inbox\/messages\/?$/i.test(url.pathname)) {
      url.pathname = url.pathname.replace(/\/inbox\/messages\/?$/i, "/webhooks/ingress");
    }
    url.searchParams.set("subscription_id", sid);
    return url.toString();
  } catch {
    const joiner = base.includes("?") ? "&" : "?";
    return `${base}${joiner}subscription_id=${encodeURIComponent(sid)}`;
  }
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
  const agentOptions =
    agent?.runtime_options && typeof agent.runtime_options === "object" && !Array.isArray(agent.runtime_options)
      ? agent.runtime_options
      : {};
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
    runtimeOptions.notifier_profile &&
    typeof runtimeOptions.notifier_profile === "object" &&
    !Array.isArray(runtimeOptions.notifier_profile)
      ? (runtimeOptions.notifier_profile as JSONRecord)
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
  const fromAgentTop = notifierKeysFromFlatRoot(agent?.runtime_options);
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
  return {
    runtime_kind: normalizeRuntimeKind(profile?.runtime_kind),
    provider: profile?.provider || "csghub_lite",
    base_url: profile?.base_url || "",
    api_key: "",
    api_key_set: Boolean(profile?.api_key_set),
    api_key_preview: profile?.api_key_preview || "",
    model_id: profile?.model_id || "",
    reasoning_effort: profile?.reasoning_effort || "medium",
    enable_fast_mode: Boolean(profile?.enable_fast_mode),
    headersText: stringifyJSON(profile?.headers || {}),
    requestOptionsText: stringifyJSON(requestOptionsWithoutNotifier),
    envRows: mapToEnvRows(profile?.env || {}),
    notifier_delivery_mode: normalizeNotifierDeliveryMode(notifier.delivery_mode || "webhook"),
    notifier_webhook_token: String(notifier.webhook_token || ""),
    notifier_remote_url: String(notifier.remote_url || ""),
    notifier_remote_subscription_id: String(notifier.remote_subscription_id || ""),
    notifier_poll_interval: String(notifier.poll_interval || "30s"),
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

function inferNotifierRuntimeKindIfUnset(
  agent: AgentLike | null | undefined,
  profile: AgentProfileLike | null | undefined,
): string {
  if (String(agent?.runtime_kind ?? "").trim()) {
    return "";
  }
  const profileSummary = notifierProfileSummaryFlags(profile || agent?.agent_profile, agent);
  if (
    profileSummary.notifier_delivery_complete ||
    profileSummary.notifier_webhook_token_set ||
    profileSummary.notifier_remote_token_set
  ) {
    return "notifier";
  }
  if (!notifierDeliveryConfiguredInProfile(profile || agent?.agent_profile || {}, agent)) {
    return "";
  }
  return "notifier";
}

export function agentToDraft(agent: AgentLike | null | undefined): AgentDraft {
  const profile = agent?.agent_profile || agent || {};
  const inferredRuntimeKind = inferNotifierRuntimeKindIfUnset(agent, profile);
  const merged = inferredRuntimeKind ? { ...agent, runtime_kind: inferredRuntimeKind } : agent;
  return {
    agent_id: merged?.id || "",
    name: merged?.name || "",
    role: merged?.role || "worker",
    description: merged?.description || profile.description || "",
    default_image: merged?.image || "",
    image: merged?.image || "",
    from_template: merged?.from_template || "",
    template_name: merged?.template_name || "",
    ...profileToDraft(profile, merged),
    runtime_kind: normalizeRuntimeKind(merged?.runtime_kind || profile.runtime_kind),
  };
}

export function normalizeTemplateSelection<T extends object>(template: T | null | undefined): T | null {
  return template && typeof template === "object" ? template : null;
}

export function templateMatchesRuntime(template, runtimeKind): boolean {
  const requestedRuntime = normalizeRuntimeKind(runtimeKind);
  if (!template || !requestedRuntime) {
    return true;
  }
  const templateRuntime = normalizeRuntimeKind(template.runtime_kind);
  return !templateRuntime || templateRuntime === requestedRuntime;
}

export function pickDefaultAgentTemplate(
  templates: readonly AgentTemplateLike[] | null | undefined,
  runtimeKind = "",
  bootstrapConfig: RuntimeBootstrapConfig | null = null,
): AgentTemplateLike | null {
  if (!Array.isArray(templates) || templates.length === 0) {
    return null;
  }
  const requestedRuntime = normalizeRuntimeKind(runtimeKind || bootstrapConfig?.runtime_kind);
  const candidates = requestedRuntime
    ? templates.filter((item) => templateMatchesRuntime(item, requestedRuntime))
    : templates.slice();
  if (!candidates.length) {
    return null;
  }
  if (requestedRuntime === "notifier") {
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
      candidates.find((item) => item.id === "builtin/openclaw-worker") ||
      candidates.find((item) => item.name === "openclaw-worker") ||
      candidates.find((item) => String(item.id || "").endsWith("/openclaw-worker")) ||
      candidates[0]
    );
  }
  if (requestedRuntime === "picoclaw_sandbox" || !requestedRuntime) {
    return (
      candidates.find((item) => item.id === "builtin/picoclaw-worker") ||
      candidates.find((item) => item.name === "picoclaw-worker") ||
      candidates.find((item) => String(item.id || "").endsWith("/picoclaw-worker")) ||
      candidates[0]
    );
  }
  return candidates[0];
}

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
  return {
    ...draft,
    from_template: template.id || "",
    template_name: template.name || template.id || "",
    runtime_kind: runtimeKind,
    image:
      template.image || runtimeImageForKind(runtimeKind, bootstrapConfig, fallbackImage || draft.default_image || ""),
    description: template.description || draft.description || "",
  };
}

export function draftToProfile(draft: AgentDraft, options: DraftProfileOptions = {}): JSONRecord {
  const requestOptions = parseJSONMap(draft.requestOptionsText);
  return {
    name: options.name || draft.name || "manager",
    description: options.description || draft.description || "Manager Worker Dispatch",
    provider: draft.provider,
    base_url: draft.base_url,
    api_key: draft.api_key,
    model_id: draft.model_id,
    reasoning_effort: draft.reasoning_effort || "medium",
    enable_fast_mode: Boolean(draft.enable_fast_mode),
    headers: parseJSONMap(draft.headersText),
    request_options: requestOptions,
    env: envRowsToMap(draft.envRows),
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
    poll_interval: String(draft.notifier_poll_interval ?? "30s").trim(),
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
  if (hasItem && notifierDeliveryConfiguredInProfile(item?.agent_profile, item)) {
    return true;
  }
  const runtimeOptions = item?.runtime_options;
  const profileRuntimeOptions = item?.agent_profile?.runtime_options;
  const runtimeSummary =
    runtimeOptions?.notifier_profile &&
    typeof runtimeOptions.notifier_profile === "object" &&
    !Array.isArray(runtimeOptions.notifier_profile)
      ? (runtimeOptions.notifier_profile as JSONRecord)
      : profileRuntimeOptions?.notifier_profile &&
          typeof profileRuntimeOptions.notifier_profile === "object" &&
          !Array.isArray(profileRuntimeOptions.notifier_profile)
        ? (profileRuntimeOptions.notifier_profile as JSONRecord)
        : null;
  const legacySummary =
    item?.agent_profile?.notifier_profile &&
    typeof item.agent_profile.notifier_profile === "object" &&
    !Array.isArray(item.agent_profile.notifier_profile)
      ? item.agent_profile.notifier_profile
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

export function envRowsToMap(rows: readonly EnvKeyValueRow[] | null | undefined): Record<string, string> {
  const result: Record<string, string> = {};
  const seen = new Set();
  for (const row of rows ?? []) {
    const key = String(row?.key ?? "").trim();
    const value = String(row?.value ?? "");
    if (!key && !value.trim()) {
      continue;
    }
    if (!key) {
      throw new Error("Environment variable key is required");
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

export function isAgentRunning(item: AgentLike | null | undefined): boolean {
  const status = String(item?.status || "").toLowerCase();
  return status === "running" || status === "online";
}

export function isAgentIncomplete(item: AgentLike | null | undefined): boolean {
  const draft = agentToDraft(item);
  if (isNotifierRuntimeDraftOnAgentPage(draft, item)) {
    return !notifierFormIsComplete(draft, item);
  }
  return item?.profile_complete === false || item?.agent_profile?.profile_complete === false;
}

export function isAgentRestartNeeded(item: AgentLike | null | undefined): boolean {
  return Boolean(item?.env_restart_required || item?.agent_profile?.env_restart_required);
}

export function agentModelID(item: AgentLike | null | undefined): string {
  return item?.model_id || item?.agent_profile?.model_id || "no model";
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
  switch (provider) {
    case "csghub_lite":
      return "CSGHub Lite";
    case "codex":
      return "Codex";
    case "claude_code":
      return "Claude Code";
    case "api":
      return "OpenAI API";
    default:
      return provider || "";
  }
}

export function normalizeRuntimeKind(kind: unknown): RuntimeKind {
  const value = String(kind ?? "")
    .trim()
    .toLowerCase();
  if (value === "") {
    return "";
  }
  switch (value) {
    case "openclaw_sandbox":
      return "openclaw_sandbox";
    case "codex":
      return "codex";
    case "notifier":
      return "notifier";
    case "picoclaw_sandbox":
      return "picoclaw_sandbox";
    default:
      return value;
  }
}

export function isNotifierRuntimeDraft(draft: Partial<AgentDraft> | null | undefined): boolean {
  return normalizeRuntimeKind(draft?.runtime_kind) === "notifier";
}

export function effectiveAgentRuntimeKind(
  draft: Partial<AgentDraft> | null | undefined,
  item: AgentLike | null | undefined,
): RuntimeKind {
  return normalizeRuntimeKind(draft?.runtime_kind || item?.runtime_kind || "");
}

export function isNotifierRuntimeDraftOnAgentPage(
  draft: Partial<AgentDraft> | null | undefined,
  item: AgentLike | null | undefined,
): boolean {
  return effectiveAgentRuntimeKind(draft, item) === "notifier";
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
    runtimeKind = "picoclaw_sandbox";
  }
  if (runtimeKind === "codex" || runtimeKind === "notifier") {
    return "";
  }
  const images = normalizeRuntimeImageMap(bootstrapConfig?.runtime_default_images);
  if (images[runtimeKind]) {
    return images[runtimeKind];
  }
  if (normalizeRuntimeKind(bootstrapConfig?.runtime_kind) === runtimeKind && bootstrapConfig?.effective_manager_image) {
    return String(bootstrapConfig.effective_manager_image).trim();
  }
  return String(fallbackImage ?? "").trim();
}

export function availableManagerRuntimeOptions(bootstrapConfig: RuntimeBootstrapConfig | null | undefined) {
  const configuredKinds = Array.isArray(bootstrapConfig?.supported_runtime_kinds)
    ? bootstrapConfig.supported_runtime_kinds
    : [];
  const gatewayKinds = (configuredKinds.length ? configuredKinds : ["picoclaw_sandbox", "openclaw_sandbox"])
    .map((kind) => normalizeRuntimeKind(kind))
    .filter((kind, index, array) => kind && kind !== "codex" && kind !== "notifier" && array.indexOf(kind) === index);
  return RUNTIME_KIND_OPTIONS.filter((option) => gatewayKinds.includes(option.value));
}

export function agentCreateProgressSteps(runtimeKind: unknown): AgentCreateProgressStep[] {
  const kind = normalizeRuntimeKind(runtimeKind) || "picoclaw_sandbox";
  if (kind === "notifier") {
    return [
      { label: "agentCreateProgressPreparing", target: 40 },
      { label: "agentCreateProgressFinishing", target: 96 },
    ];
  }
  if (kind === "openclaw_sandbox" || kind === "picoclaw_sandbox") {
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
      return "Codex";
    case "notifier":
      return "notifier";
    case "picoclaw_sandbox":
      return t("runtimePicoclaw");
    default:
      return runtimeKind;
  }
}
