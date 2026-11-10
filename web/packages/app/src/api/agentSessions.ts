import { get, readResponseText, resolveRequestPath, type ApiError } from "@/api/client";
import type { AgentLike } from "@/models/agents";

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

export type CancelAgentSessionResponseRequest = {
  agentName: string;
  sessionId: string;
};

export type AgentSessionStreamResult = {
  id: string;
  text: string;
};

export async function fetchSessionAgents(): Promise<SessionAgent[]> {
  const response = await get<AgentLike[]>("api/v1/agents");
  return (Array.isArray(response) ? response : [])
    .filter(supportsSessionAPI)
    .map((item) => ({
      id: String(item.id || "").trim(),
      name: String(item.name || item.id || "").trim(),
      status: String(item.status || item.runtime?.state || "").trim(),
    }))
    .filter((item) => item.id && item.name)
    .sort((left, right) => left.name.localeCompare(right.name));
}

export async function streamAgentSessionResponse(
  request: CreateAgentSessionResponseRequest,
  onTextDelta?: (delta: string) => void,
): Promise<AgentSessionStreamResult> {
  const path = `api/v1/agents/${encodeURIComponent(request.agentName)}/sessions/${encodeURIComponent(request.sessionId)}/responses`;
  let response: Response;
  try {
    response = await fetch(resolveRequestPath(path), {
      method: "POST",
      headers: { Accept: "text/event-stream", "Content-Type": "application/json" },
      body: JSON.stringify({ input: request.input, stream: true }),
      signal: request.signal,
    });
  } catch (error) {
    throw normalizeAgentSessionAPIError(error);
  }

  if (!response.ok) {
    const message = (await readResponseText(response)) || response.statusText;
    throw normalizeAgentSessionAPIError({ status: response.status, message } satisfies ApiError);
  }
  if (!response.headers.get("Content-Type")?.includes("text/event-stream")) {
    throw new Error("Session API returned a non-streaming response");
  }
  if (!response.body) {
    throw new Error("Session API returned an empty stream");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let responseID = "";
  let text = "";
  let stopped = false;

  const consume = (block: string) => {
    const event = parseAgentSessionSSEBlock(block);
    if (!event) return;
    if (event.type === "message_start") {
      responseID = stringField(objectField(event, "message"), "id");
      return;
    }
    if (event.type === "content_block_delta") {
      const delta = objectField(event, "delta");
      if (stringField(delta, "type") !== "text_delta") return;
      const value = stringField(delta, "text");
      if (!value) return;
      text += value;
      onTextDelta?.(value);
      return;
    }
    if (event.type === "error") {
      throw new Error(stringField(objectField(event, "error"), "message") || "Session stream failed");
    }
    if (event.type === "message_stop") {
      stopped = true;
    }
  };

  try {
    for (;;) {
      const { done, value } = await reader.read();
      buffer = (buffer + decoder.decode(value, { stream: !done })).replaceAll("\r\n", "\n");
      let boundary = buffer.indexOf("\n\n");
      while (boundary >= 0) {
        consume(buffer.slice(0, boundary));
        buffer = buffer.slice(boundary + 2);
        boundary = buffer.indexOf("\n\n");
      }
      if (done) break;
    }
    if (buffer.trim()) consume(buffer);
  } finally {
    reader.releaseLock();
  }

  if (!stopped || !responseID) {
    throw new Error("Session stream ended before completion");
  }
  return { id: responseID, text };
}

export async function cancelAgentSessionResponse(request: CancelAgentSessionResponseRequest): Promise<void> {
  const path = `api/v1/agents/${encodeURIComponent(request.agentName)}/sessions/${encodeURIComponent(request.sessionId)}/responses/cancel`;
  let response: Response;
  try {
    response = await fetch(resolveRequestPath(path), {
      method: "POST",
      headers: { Accept: "application/json" },
    });
  } catch (error) {
    throw normalizeAgentSessionAPIError(error);
  }
  if (!response.ok) {
    const message = (await readResponseText(response)) || response.statusText;
    throw normalizeAgentSessionAPIError({ status: response.status, message } satisfies ApiError);
  }
}

function parseAgentSessionSSEBlock(block: string): Record<string, unknown> | null {
  let eventType = "";
  const data: string[] = [];
  for (const line of block.split("\n")) {
    if (!line || line.startsWith(":")) continue;
    const separator = line.indexOf(":");
    const field = separator < 0 ? line : line.slice(0, separator);
    const value = separator < 0 ? "" : line.slice(separator + 1).replace(/^ /, "");
    if (field === "event") eventType = value;
    if (field === "data") data.push(value);
  }
  if (data.length === 0) return null;
  const parsed = JSON.parse(data.join("\n")) as unknown;
  if (!parsed || typeof parsed !== "object") return null;
  const event = parsed as Record<string, unknown>;
  if (!stringField(event, "type") && eventType) event.type = eventType;
  return event;
}

function objectField(value: Record<string, unknown>, field: string): Record<string, unknown> {
  const candidate = value[field];
  return candidate && typeof candidate === "object" ? (candidate as Record<string, unknown>) : {};
}

function stringField(value: Record<string, unknown>, field: string): string {
  return typeof value[field] === "string" ? value[field] : "";
}

function supportsSessionAPI(item: AgentLike): boolean {
  const runtimeName = String(item.runtime?.name || item.runtime_name || "")
    .trim()
    .toLowerCase();
  const runtimeState = String(item.runtime?.state || item.status || "")
    .trim()
    .toLowerCase();
  return Boolean(item.id && runtimeName === "codex" && runtimeState === "running");
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
