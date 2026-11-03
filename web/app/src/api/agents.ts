import type { components } from "@/api/generated/controlPlane";
import { del, get, requestWithResponse, patch, post, put, requestText, type ApiError } from "@/api/client";
import { checkModelProvider } from "@/api/modelProviders";
import { BOT_TYPE_NOTIFICATION, MANAGER_AGENT_ID } from "@/shared/constants/agents";
import {
  isNotificationBotAgent,
  resolveRuntimeSelection,
  type AgentLike,
  type AgentProfileLike,
  type AgentProfileModelsResponse,
  type JSONRecord,
  type RuntimeKind,
  type RuntimeName,
} from "@/models/agents";
import type { WorkspaceFile, WorkspaceListing } from "@/models/workspace";

export type AgentProfileModelRequest = {
  agent_id?: string;
  model_provider_id?: string;
  api_key?: string;
  base_url?: string;
  headers?: JSONRecord;
  provider?: string;
};

export type FetchAgentsOptions = {
  silent?: boolean;
};

export type AgentInstructionsDocument = components["schemas"]["InstructionsDocument"];

export function fetchAgentInstructionsDocument(agentID: string): Promise<AgentInstructionsDocument> {
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/instructions`);
}

export type AgentMemoryDocument = components["schemas"]["MemoryDocument"];

export function fetchAgentMemoryDocument(agentID: string): Promise<AgentMemoryDocument> {
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/memory`);
}

export function updateAgentMemoryEnabled(agentID: string, enabled: boolean): Promise<AgentMemoryDocument> {
  return put(`api/v1/agents/${encodeURIComponent(agentID)}/memory`, { enabled });
}

export function updateAgentEffectiveInstructions(
  agentID: string,
  effective: string,
): Promise<AgentInstructionsDocument> {
  return put(`api/v1/agents/${encodeURIComponent(agentID)}/instructions`, { effective });
}

export type FetchAgentOptions = {
  cacheBust?: boolean;
};

export type FetchAgentLogsOptions = {
  lines?: number;
};

export type DeleteBotOptions = {
  deleteAgent?: boolean;
};

export type AgentMCPServersView = {
  agent_id?: string | null;
  runtime_kind?: string | null;
  servers?: JSONRecord | null;
};

export type AgentUpdatePayload = {
  agent_profile?: JSONRecord;
  description?: string;
  instructions?: string;
  skills?: string[];
  image?: string;
  model_config?: JSONRecord;
  mcpServers?: JSONRecord | null;
  name?: string;
  profile?: JSONRecord | string;
  runtime?: { name?: RuntimeName; sandbox_enabled?: boolean; image?: string; options?: JSONRecord };
  role?: string;
  runtime_name?: RuntimeName;
  sandbox_enabled?: boolean;
  runtime_kind?: RuntimeKind;
  runtime_options?: JSONRecord;
  from_template?: string;
};

export type ParticipantLike = {
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
};

export type HostDirectoryPickResult =
  | { status: "selected"; path: string }
  | { status: "canceled" }
  | { status: "unavailable" };

export type FeishuRegistration = {
  agent_id?: string;
  connect_url?: string;
  expires_at?: string;
  next_poll_seconds?: number;
  participant_id?: string;
  registration_id?: string;
  status?: string;
  user_code?: string;
};

export type FeishuRegistrationFinalizeResult = FeishuRegistration & {
  channel?: string;
  config_saved?: boolean;
  participant_type?: string;
  restart_error?: string;
  restart_status?: string;
  warnings?: string[];
};

function modelPayload(draft: AgentProfileModelRequest): AgentProfileModelRequest {
  return {
    agent_id: draft.agent_id,
    provider: draft.provider,
    base_url: draft.base_url,
    api_key: draft.api_key,
    headers: draft.headers,
  };
}

export function fetchManagerProfile(): Promise<AgentProfileLike> {
  return get(`api/v1/agents/${MANAGER_AGENT_ID}/profile`);
}

export function saveManagerProfileRequest(profile: JSONRecord): Promise<AgentProfileLike> {
  return put(`api/v1/agents/${MANAGER_AGENT_ID}/profile`, profile);
}

export async function fetchAgents(options: FetchAgentsOptions = {}): Promise<AgentLike[]> {
  void options;
  const [agents, notifications] = await Promise.all([
    get<components["schemas"]["AgentList"]>("api/v1/agents?include_participants=true"),
    get<ParticipantLike[]>("api/v1/channels/csgclaw/participants?type=notification"),
  ]);
  return [
    ...agents.items.map(agentFromResponse),
    ...(Array.isArray(notifications) ? notifications.map(participantToAgentLike) : []),
  ];
}

export function fetchAgent(agentID: string, options: FetchAgentOptions = {}): Promise<AgentLike> {
  const params = new URLSearchParams({ include_participants: "true" });
  if (options.cacheBust) {
    params.set("_", String(Date.now()));
  }
  return get<AgentResponse>(`api/v1/agents/${encodeURIComponent(agentID)}?${params.toString()}`).then(
    agentFromResponse,
  );
}

export function fetchAgentLogsRequest(agentID: string, options: FetchAgentLogsOptions = {}): Promise<string> {
  const params = new URLSearchParams();
  const lines = Number(options.lines ?? 400);
  if (Number.isFinite(lines) && lines > 0) {
    params.set("lines", String(Math.floor(lines)));
  }
  return requestText(`api/v1/agents/${encodeURIComponent(agentID)}/logs?${params.toString()}`);
}

export function fetchAgentWorkspace(agentID: string, workspacePath = ""): Promise<WorkspaceListing> {
  const params = new URLSearchParams();
  if (workspacePath.trim()) {
    params.set("path", workspacePath.trim());
  }
  const query = params.toString();
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/workspace${query ? `?${query}` : ""}`);
}

export function fetchAgentWorkspaceFile(agentID: string, workspacePath: string): Promise<WorkspaceFile> {
  const params = new URLSearchParams({ path: workspacePath });
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/workspace/file?${params.toString()}`);
}

export function fetchAgentSkills(agentID: string, skillsPath = ""): Promise<WorkspaceListing> {
  const params = new URLSearchParams();
  if (skillsPath.trim()) {
    params.set("path", skillsPath.trim());
  }
  const query = params.toString();
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/skills${query ? `?${query}` : ""}`);
}

export type AgentSkillSummary = {
  enabled?: boolean;
  name: string;
  description?: string;
  error?: string;
  source?: string;
};

export function fetchAgentSkillSummaries(agentID: string, signal?: AbortSignal): Promise<AgentSkillSummary[]> {
  return get<components["schemas"]["AgentSkillSummaryList"]>(
    `api/v1/agents/${encodeURIComponent(agentID)}/skill-summaries`,
    {
      signal,
    },
  ).then((response) => response.items);
}

export function fetchAgentSkillsFile(agentID: string, skillsPath: string): Promise<WorkspaceFile> {
  const params = new URLSearchParams({ path: skillsPath });
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/skills/file?${params.toString()}`);
}

export function fetchAgentMCPServers(agentID: string): Promise<AgentMCPServersView> {
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/mcp-servers`);
}

export function batchAddAgentMCPServersRequest(
  agentID: string,
  serverNames: string[],
  enabled?: boolean,
): Promise<AgentMCPServersView> {
  return post(`api/v1/agents/${encodeURIComponent(agentID)}/mcp-servers:batchAdd`, {
    names: serverNames,
    ...(enabled === undefined ? {} : { enabled }),
  });
}

export function batchDeleteAgentMCPServersRequest(
  agentID: string,
  serverNames: string[],
): Promise<AgentMCPServersView> {
  return post(`api/v1/agents/${encodeURIComponent(agentID)}/mcp-servers:batchDelete`, { names: serverNames });
}

export type AgentSkillInstallOptions = { replace?: boolean };

export function batchAddAgentSkillsRequest(
  agentID: string,
  skillNames: string[],
  options: AgentSkillInstallOptions = {},
): Promise<void> {
  return post(`api/v1/agents/${encodeURIComponent(agentID)}/skills:batchAdd`, {
    names: skillNames,
    ...(options.replace ? { replace: true } : {}),
  });
}

export function deleteAgentSkillRequest(agentID: string, skillName: string): Promise<void> {
  return del(
    `api/v1/agents/${encodeURIComponent(agentID)}/skills/${encodeURIComponent(String(skillName || "").trim())}`,
  );
}

export function createManagerAgentRequest(): Promise<AgentLike> {
  return post<AgentResponse>(`api/v1/agents/${MANAGER_AGENT_ID}/recreate`, {}).then(agentFromResponse);
}

export function fetchAgentProfileDefaults(): Promise<AgentProfileLike> {
  return get("api/v1/agent-profile-defaults");
}

export function updateAgentProfileDefaults(payload: {
  kind: "agent" | "image" | "video";
  provider_id: string;
  model_id: string;
}): Promise<AgentProfileLike> {
  return patch("api/v1/agent-profile-defaults", payload);
}

export function fetchAgentProfile(agentID: string): Promise<AgentProfileLike> {
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/profile`);
}

export async function fetchAgentProfileModels(draft: AgentProfileModelRequest): Promise<AgentProfileModelsResponse> {
  const providerID = String(draft.model_provider_id || "").trim();
  if (providerID) {
    const result = await checkModelProvider(providerID);
    if (result.status !== "connected") {
      throw new Error(result.message || "Model provider check failed");
    }
    return { models: result.models };
  }
  const { items, provider } = await post<components["schemas"]["ProfileModelList"]>(
    "api/v1/agent-profiles/models",
    modelPayload(draft),
  );
  return { models: items, provider };
}

export async function pickHostDirectoryRequest(): Promise<HostDirectoryPickResult> {
  try {
    const response = await post<{ path?: string | null } | undefined>("api/v1/local/directory-picker", {});
    const path = String(response?.path ?? "").trim();
    return path ? { status: "selected", path } : { status: "canceled" };
  } catch (error) {
    const status = Number((error as { status?: unknown })?.status ?? 0);
    if (status === 404 || status === 405 || status === 501 || status === 503) {
      return { status: "unavailable" };
    }
    throw error;
  }
}

type AgentResponse = components["schemas"]["Agent"];

function agentFromResponse(response: components["schemas"]["AgentList"]["items"][number]): AgentLike {
  if (!("created_at" in response)) return response;
  const { mcp_servers, ...agent } = response;
  return {
    ...agent,
    image: agent.runtime?.image,
    mcpServers: mcp_servers,
  };
}

export function updateAgentRequest(agentID: string, payload: AgentUpdatePayload): Promise<AgentLike> {
  return patch<AgentResponse>(`api/v1/agents/${encodeURIComponent(agentID)}`, normalizeAgentPayload(payload)).then(
    agentFromResponse,
  );
}

function normalizeAgentPayload(payload: AgentUpdatePayload) {
  const runtime = { ...payload.runtime };
  if (
    payload.runtime_name !== undefined ||
    payload.sandbox_enabled !== undefined ||
    payload.runtime_kind !== undefined
  ) {
    const selection = resolveRuntimeSelection({
      runtime_kind: payload.runtime_kind,
      runtime_name: payload.runtime_name ?? runtime.name,
      sandbox_enabled: payload.sandbox_enabled ?? runtime.sandbox_enabled,
    });
    runtime.name = selection.runtime_name;
    runtime.sandbox_enabled = selection.sandbox_enabled;
  }
  if (payload.image !== undefined) runtime.image = payload.image;
  if (payload.runtime_options !== undefined) runtime.options = payload.runtime_options;
  const model =
    payload.model_config ??
    payload.agent_profile ??
    (typeof payload.profile === "object" ? payload.profile : undefined);
  const modelConfig = typeof payload.profile === "string" ? { ...model, selector: payload.profile } : model;
  return {
    name: payload.name,
    description: payload.description,
    instructions: payload.instructions,
    ...(Object.keys(runtime).length ? { runtime } : {}),
    model_config: modelConfig,
    mcp_servers: payload.mcpServers,
  } satisfies components["schemas"]["AgentPatchRequest"];
}

export type CreateBotPayload = AgentUpdatePayload & {
  type?: string;
};

export async function createBotRequest(payload: CreateBotPayload): Promise<AgentLike> {
  const runtimeSelection = resolveRuntimeSelection({
    runtime_kind: payload.runtime_kind,
    runtime_name: payload.runtime_name ?? payload.runtime?.name,
    sandbox_enabled: payload.sandbox_enabled ?? payload.runtime?.sandbox_enabled,
  });
  const request: components["schemas"]["CreateParticipantRequest"] = {
    name: payload.name ?? "",
    type: "agent",
    agent_binding: {
      mode: "create",
      agent: {
        ...normalizeAgentPayload({
          ...payload,
          runtime_kind: undefined,
          runtime_name: runtimeSelection.runtime_name,
          sandbox_enabled: runtimeSelection.sandbox_enabled,
        }),
        name: payload.name ?? "",
        role: payload.role,
        skills: payload.skills,
        from_template: payload.from_template,
      },
    },
  };
  const participant = await post<ParticipantLike>("api/v1/channels/csgclaw/participants", request);
  const skippedResources = Array.isArray(participant.metadata?.skipped_resources)
    ? participant.metadata.skipped_resources
    : [];
  if (!participant.agent_id) {
    return { ...participantToAgentLike(participant), skipped_resources: skippedResources };
  }
  const created = await fetchAgent(participant.agent_id);
  return { ...created, skipped_resources: skippedResources };
}

export async function createNotificationBotRequest(payload: CreateBotPayload): Promise<AgentLike> {
  const participant = await post<ParticipantLike>("api/v1/channels/csgclaw/participants", {
    name: payload.name,
    type: "notification",
    metadata: payload.runtime_options ?? {},
  });
  return participantToAgentLike(participant);
}

export function patchNotificationBotRequest(botID: string, payload: CreateBotPayload): Promise<AgentLike> {
  return patch<ParticipantLike>(`api/v1/channels/csgclaw/participants/${encodeURIComponent(botID)}`, {
    name: payload.name,
    metadata: payload.runtime_options ?? {},
  }).then(participantToAgentLike);
}

export function deleteAgentRequest(agentID: string, name: string): Promise<void> {
  return del(`api/v1/agents/${encodeURIComponent(agentID)}`, { json: { name } });
}

export function deleteBotRequest(botID: string, options: DeleteBotOptions = {}): Promise<void> {
  const params = new URLSearchParams();
  if (options.deleteAgent) {
    params.set("delete_agent", "if_unreferenced");
  }
  const query = params.toString();
  return del(`api/v1/channels/csgclaw/participants/${encodeURIComponent(botID)}${query ? `?${query}` : ""}`);
}

export function deleteAgentLikeRequest(item: AgentLike): Promise<void> {
  const id = String(item.id ?? "").trim();
  if (!id) {
    return Promise.reject(new Error("agent id is required"));
  }
  return isNotificationBotAgent(item) ? deleteBotRequest(id) : deleteAgentRequest(id, String(item.name ?? ""));
}

export function deleteFeishuParticipantRequest(participantID: string): Promise<void> {
  return del(`api/v1/channels/feishu/participants/${encodeURIComponent(participantID)}`);
}

export function runAgentActionRequest(agentID: string, action: string): Promise<AgentLike> {
  return post<AgentResponse>(`api/v1/agents/${encodeURIComponent(agentID)}/${action}`).then(agentFromResponse);
}

export async function startFeishuRegistrationRequest(agentID: string): Promise<FeishuRegistration> {
  try {
    return await post("api/v1/channels/feishu/registrations", { agent_id: agentID });
  } catch (error) {
    const pending = pendingFeishuRegistrationFromAPIError(error);
    if (pending) {
      return pending;
    }
    throw error;
  }
}

export function fetchFeishuRegistrationRequest(registrationID: string): Promise<FeishuRegistration> {
  return get(`api/v1/channels/feishu/registrations/${encodeURIComponent(registrationID)}`);
}

export function finalizeFeishuRegistrationRequest(registrationID: string): Promise<FeishuRegistrationFinalizeResult> {
  return post(`api/v1/channels/feishu/registrations/${encodeURIComponent(registrationID)}:finalize`, {});
}

function pendingFeishuRegistrationFromAPIError(error: unknown): FeishuRegistration | null {
  const apiError = error as ApiError | null;
  if (!apiError || apiError.status !== 409) {
    return null;
  }
  try {
    const parsed = JSON.parse(apiError.message) as Partial<FeishuRegistration>;
    if (
      parsed &&
      typeof parsed === "object" &&
      String(parsed.registration_id || "").trim() &&
      String(parsed.status || "").trim() === "pending"
    ) {
      return parsed as FeishuRegistration;
    }
  } catch {
    return null;
  }
  return null;
}

function participantToAgentLike(participant: ParticipantLike): AgentLike {
  const metadata = participant.metadata ?? {};
  return {
    id: participant.id,
    name: participant.name,
    type: participant.type === "notification" ? BOT_TYPE_NOTIFICATION : participant.type,
    bot_type: participant.type === "notification" ? BOT_TYPE_NOTIFICATION : participant.type,
    available: participant.lifecycle_status === "active",
    runtime_options: metadata,
    notification_profile: metadata,
    notifier_profile: metadata,
    status: participant.lifecycle_status,
    user_id: participant.user_id,
    user_name: participant.user_name,
  };
}

export type AgentResourceKind = "skill" | "mcp";
export type AgentResourceEnabledResult = components["schemas"]["AgentResourceEnabledResponse"];

export async function setAgentResourceEnabled(
  agentID: string,
  kind: AgentResourceKind,
  name: string,
  enabled: boolean,
): Promise<AgentResourceEnabledResult> {
  const base = `api/v1/agents/${encodeURIComponent(agentID)}`;
  const list = kind === "skill" ? "skill-summaries" : "mcp-servers";
  const { headers } = await requestWithResponse<unknown>(`${base}/${list}?pagination=page&page=1&limit=1`);
  const etag = headers.get("ETag");
  if (!etag)
    throw {
      status: 409,
      code: "resource_version_conflict",
      message: "Agent resource version is unavailable",
    } satisfies ApiError;
  const resourceVersion: unknown = JSON.parse(etag);
  if (typeof resourceVersion !== "string") throw new Error("Invalid resource version");
  const resource = kind === "skill" ? "skills" : "mcp-servers";
  return put(`${base}/${resource}/${encodeURIComponent(name)}/enabled`, { enabled, resource_version: resourceVersion });
}

export function fetchNotificationParticipants(): Promise<ParticipantLike[]> {
  return get("api/v1/channels/csgclaw/participants?type=notification");
}

export function createNotificationSourceRequest(payload: { name?: string }): Promise<ParticipantLike> {
  return post("api/v1/channels/csgclaw/participants", { type: "notification", ...payload });
}

export function deleteNotificationSourceRequest(participantID: string): Promise<void> {
  return del(`api/v1/channels/csgclaw/participants/${encodeURIComponent(participantID)}`);
}

export function updateNotificationParticipantRequest(
  participantID: string,
  payload: { name?: string; metadata: JSONRecord },
): Promise<ParticipantLike> {
  return patch(`api/v1/channels/csgclaw/participants/${encodeURIComponent(participantID)}`, payload);
}

export type AgentArchiveImpact = components["schemas"]["AgentArchiveImpact"];
export function fetchAgentArchiveImpact(id: string): Promise<AgentArchiveImpact> {
  return get(`api/v1/agents/${encodeURIComponent(id)}/archive-impact`);
}
export function archiveAgent(id: string): Promise<AgentLike> {
  return post<AgentResponse>(`api/v1/agents/${encodeURIComponent(id)}/archive`, {}).then(agentFromResponse);
}
export function restoreAgent(id: string): Promise<AgentLike> {
  return post<AgentResponse>(`api/v1/agents/${encodeURIComponent(id)}/restore`, {}).then(agentFromResponse);
}
export function fetchArchivedAgents(): Promise<AgentLike[]> {
  return get<components["schemas"]["AgentList"]>("api/v1/agents?archived=true&include_participants=true").then(
    (response) => response.items.map(agentFromResponse),
  );
}
export function deleteArchivedAgent(id: string, name: string): Promise<void> {
  return del(`api/v1/agents/${encodeURIComponent(id)}`, { json: { name } });
}
