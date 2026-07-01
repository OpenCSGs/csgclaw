import { del, get, patch, post, put, requestText, type ApiError } from "@/api/client";
import { BOT_TYPE_NOTIFICATION, MANAGER_AGENT_ID } from "@/shared/constants/agents";
import {
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
  api_key?: string;
  base_url?: string;
  headers?: JSONRecord;
  provider?: string;
};

export type FetchAgentsOptions = {
  silent?: boolean;
};

export type FetchAgentOptions = {
  cacheBust?: boolean;
};

export type CreateManagerAgentOptions = {
  runtime_kind?: RuntimeKind;
};

export type FetchAgentLogsOptions = {
  lines?: number;
};

export type DeleteBotOptions = {
  deleteAgent?: boolean;
};

export type AgentUpdatePayload = {
  agent_profile?: JSONRecord;
  description?: string;
  field_mask?: string[];
  instructions?: string;
  image?: string;
  model_config?: JSONRecord;
  name?: string;
  profile?: JSONRecord | string;
  runtime?: { name?: RuntimeName; sandbox_enabled?: boolean; options?: JSONRecord };
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
    get<AgentLike[]>("api/v1/agents?include_participants=true"),
    get<ParticipantLike[]>("api/v1/channels/csgclaw/participants?type=notification"),
  ]);
  return [
    ...(Array.isArray(agents) ? agents : []),
    ...(Array.isArray(notifications) ? notifications.map(participantToAgentLike) : []),
  ];
}

export function fetchAgent(agentID: string, options: FetchAgentOptions = {}): Promise<AgentLike> {
  const params = new URLSearchParams({ include_participants: "true" });
  if (options.cacheBust) {
    params.set("_", String(Date.now()));
  }
  return get(`api/v1/agents/${encodeURIComponent(agentID)}?${params.toString()}`);
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

export function fetchAgentSkillsFile(agentID: string, skillsPath: string): Promise<WorkspaceFile> {
  const params = new URLSearchParams({ path: skillsPath });
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/skills/file?${params.toString()}`);
}

export function batchAddAgentSkillsRequest(agentID: string, skillNames: string[]): Promise<void> {
  return post(`api/v1/agents/${encodeURIComponent(agentID)}/skills:batchAdd`, { names: skillNames });
}

export function deleteAgentSkillRequest(agentID: string, skillName: string): Promise<void> {
  return del(
    `api/v1/agents/${encodeURIComponent(agentID)}/skills/${encodeURIComponent(String(skillName || "").trim())}`,
  );
}

export function createManagerAgentRequest(options: CreateManagerAgentOptions = {}): Promise<AgentLike> {
  const payload: { id: string; replace: boolean; runtime?: { name: RuntimeName; sandbox_enabled: boolean } } = {
    id: MANAGER_AGENT_ID, // Legacy contract: id: "u-manager",
    replace: true,
  };
  if (options.runtime_kind) {
    const runtimeSelection = resolveRuntimeSelection({ runtime_kind: options.runtime_kind });
    payload.runtime = {
      name: runtimeSelection.runtime_name,
      sandbox_enabled: runtimeSelection.sandbox_enabled,
    };
  }
  return post("api/v1/agents", payload);
}

export function fetchAgentProfileDefaults(): Promise<AgentProfileLike> {
  return get("api/v1/agent-profile-defaults");
}

export function fetchAgentProfile(agentID: string): Promise<AgentProfileLike> {
  return get(`api/v1/agents/${encodeURIComponent(agentID)}/profile`);
}

export function fetchAgentProfileModels(draft: AgentProfileModelRequest): Promise<AgentProfileModelsResponse> {
  return post("api/v1/agent-profiles/models", modelPayload(draft));
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

export function updateAgentRequest(agentID: string, payload: AgentUpdatePayload): Promise<AgentLike> {
  const normalizedPayload = normalizeAgentPayload(payload);
  const fieldMask = Object.keys(normalizedPayload).filter(
    (key) => key !== "field_mask" && normalizedPayload[key as keyof AgentUpdatePayload] !== undefined,
  );
  return patch(`api/v1/agents/${encodeURIComponent(agentID)}`, {
    ...normalizedPayload,
    field_mask: fieldMask,
  });
}

function normalizeAgentPayload(payload: AgentUpdatePayload): AgentUpdatePayload {
  const normalized: AgentUpdatePayload = { ...payload };
  if (payload.agent_profile && typeof payload.agent_profile === "object") {
    normalized.model_config = payload.agent_profile;
    delete normalized.agent_profile;
  }
  if (payload.runtime_name || payload.sandbox_enabled !== undefined) {
    const runtimeSelection = resolveRuntimeSelection({
      runtime_kind: payload.runtime_kind,
      runtime_name: payload.runtime_name,
      sandbox_enabled: payload.sandbox_enabled ?? normalized.runtime?.sandbox_enabled,
    });
    normalized.runtime = {
      ...(normalized.runtime ?? {}),
      name: runtimeSelection.runtime_name,
      sandbox_enabled: runtimeSelection.sandbox_enabled,
    };
    delete normalized.runtime_name;
    delete normalized.sandbox_enabled;
    delete normalized.runtime_kind;
  }
  return normalized;
}

export type CreateBotPayload = AgentUpdatePayload & {
  type?: string;
};

export async function createBotRequest(payload: CreateBotPayload): Promise<AgentLike> {
  const runtimeSelection = resolveRuntimeSelection({
    runtime_kind: payload.runtime_kind,
    runtime_name: payload.runtime_name,
    sandbox_enabled: payload.sandbox_enabled,
  });
  const participant = await post<ParticipantLike>("api/v1/channels/csgclaw/participants", {
    name: payload.name,
    type: "agent",
    agent_binding: {
      mode: "create",
      agent: {
        name: payload.name,
        role: payload.role,
        description: payload.description,
        instructions: payload.instructions,
        image: payload.image,
        runtime: {
          name: runtimeSelection.runtime_name,
          sandbox_enabled: runtimeSelection.sandbox_enabled,
          options: payload.runtime_options,
        },
        runtime_name: runtimeSelection.runtime_name,
        sandbox_enabled: runtimeSelection.sandbox_enabled,
        from_template: payload.from_template,
        model_config: payload.agent_profile,
      },
    },
  });
  return participant.agent_id ? fetchAgent(participant.agent_id) : participantToAgentLike(participant);
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

export function deleteAgentRequest(agentID: string): Promise<void> {
  return del(`api/v1/agents/${encodeURIComponent(agentID)}`);
}

export function deleteBotRequest(botID: string, options: DeleteBotOptions = {}): Promise<void> {
  const params = new URLSearchParams();
  if (options.deleteAgent) {
    params.set("delete_agent", "if_unreferenced");
  }
  const query = params.toString();
  return del(`api/v1/channels/csgclaw/participants/${encodeURIComponent(botID)}${query ? `?${query}` : ""}`);
}

export function deleteFeishuParticipantRequest(participantID: string): Promise<void> {
  return del(`api/v1/channels/feishu/participants/${encodeURIComponent(participantID)}`);
}

export function runAgentActionRequest(agentID: string, action: string): Promise<AgentLike> {
  return post(`api/v1/agents/${encodeURIComponent(agentID)}/${action}`);
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
