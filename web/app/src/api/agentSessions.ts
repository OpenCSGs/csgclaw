import { get, post, type ApiError } from "@/api/client";
import type { AgentLike } from "@/models/agents";
import type { AgentSessionResponse } from "@/models/agentSessions";

export type SessionAgent = {
  id: string;
  name: string;
  status: string;
};

export type CreateAgentSessionResponseRequest = {
  agentName: string;
  sessionId: string;
  input: string;
  signal?: AbortSignal;
};

export async function fetchSessionAgents(): Promise<SessionAgent[]> {
  const response = await get<AgentLike[]>("api/v1/agents?include_participants=true");
  return (Array.isArray(response) ? response : [])
    .filter(hasActiveLocalParticipant)
    .map((item) => ({
      id: String(item.id || "").trim(),
      name: String(item.name || item.id || "").trim(),
      status: String(item.status || item.runtime?.state || "").trim(),
    }))
    .filter((item) => item.id && item.name)
    .sort((left, right) => left.name.localeCompare(right.name));
}

export async function createAgentSessionResponse(
  request: CreateAgentSessionResponseRequest,
): Promise<AgentSessionResponse> {
  try {
    return await post<AgentSessionResponse>(
      `api/v1/agents/${encodeURIComponent(request.agentName)}/sessions/${encodeURIComponent(request.sessionId)}/responses`,
      { input: request.input },
      { signal: request.signal },
    );
  } catch (error) {
    throw normalizeAgentSessionAPIError(error);
  }
}

function hasActiveLocalParticipant(item: AgentLike): boolean {
  const activeLocalParticipants = (item.participants ?? []).filter(
    (entry) =>
      entry.channel === "csgclaw" &&
      entry.type === "agent" &&
      entry.lifecycle_status === "active" &&
      entry.channel_user_kind === "local_user_id" &&
      entry.channel_user_ref,
  );
  return Boolean(item.id && activeLocalParticipants.length === 1);
}

function normalizeAgentSessionAPIError(error: unknown): unknown {
  if (!error || typeof error !== "object" || !("message" in error)) {
    return error;
  }
  const apiError = error as ApiError;
  try {
    const parsed = JSON.parse(apiError.message) as { error?: { message?: unknown; code?: unknown } };
    const message = String(parsed.error?.message || "").trim();
    if (!message) {
      return error;
    }
    return {
      status: apiError.status,
      message,
      code: String(parsed.error?.code || "").trim(),
    };
  } catch {
    return error;
  }
}
