import { createAgentSessionID, isValidAgentSessionID, type AgentSessionMessage } from "@/models/agentSessions";

export const SESSION_DEMO_STORAGE_VERSION = 1 as const;
export const SESSION_DEMO_MAX_SESSIONS = 20;
export const SESSION_DEMO_MAX_MESSAGES = 100;

export type AgentSessionRecord = {
  agentId: string;
  agentName: string;
  sessionId: string;
  messages: AgentSessionMessage[];
  updatedAt: string;
};

export type SessionDemoStorageState = {
  version: typeof SESSION_DEMO_STORAGE_VERSION;
  currentAgentId: string;
  currentSessionId: string;
  sessions: AgentSessionRecord[];
};

export function emptySessionDemoStorageState(): SessionDemoStorageState {
  return {
    version: SESSION_DEMO_STORAGE_VERSION,
    currentAgentId: "",
    currentSessionId: "",
    sessions: [],
  };
}

export function loadSessionDemoStorage(storage: Storage, key: string): SessionDemoStorageState {
  try {
    const parsed = JSON.parse(storage.getItem(key) || "null") as Partial<SessionDemoStorageState> | null;
    if (!parsed || parsed.version !== SESSION_DEMO_STORAGE_VERSION || !Array.isArray(parsed.sessions)) {
      return emptySessionDemoStorageState();
    }
    const sessions = parsed.sessions
      .map(normalizeSessionRecord)
      .filter((record): record is AgentSessionRecord => Boolean(record));
    return {
      version: SESSION_DEMO_STORAGE_VERSION,
      currentAgentId: String(parsed.currentAgentId || "").trim(),
      currentSessionId: isValidAgentSessionID(parsed.currentSessionId) ? parsed.currentSessionId.trim() : "",
      sessions: sessions
        .sort((left, right) => Date.parse(right.updatedAt) - Date.parse(left.updatedAt))
        .slice(0, SESSION_DEMO_MAX_SESSIONS),
    };
  } catch {
    return emptySessionDemoStorageState();
  }
}

export function saveSessionDemoStorage(storage: Storage, key: string, state: SessionDemoStorageState): void {
  const sessions = state.sessions
    .map((record) => ({ ...record, messages: record.messages.slice(-SESSION_DEMO_MAX_MESSAGES) }))
    .sort((left, right) => Date.parse(right.updatedAt) - Date.parse(left.updatedAt))
    .slice(0, SESSION_DEMO_MAX_SESSIONS);
  storage.setItem(
    key,
    JSON.stringify({
      version: SESSION_DEMO_STORAGE_VERSION,
      currentAgentId: state.currentAgentId,
      currentSessionId: state.currentSessionId,
      sessions,
    } satisfies SessionDemoStorageState),
  );
}

export function upsertSessionRecord(
  state: SessionDemoStorageState,
  record: AgentSessionRecord,
): SessionDemoStorageState {
  return {
    version: SESSION_DEMO_STORAGE_VERSION,
    currentAgentId: record.agentId,
    currentSessionId: record.sessionId,
    sessions: [record, ...state.sessions.filter((item) => item.sessionId !== record.sessionId)],
  };
}

export function findSessionRecord(
  state: SessionDemoStorageState,
  sessionId: string,
  agentId: string,
): AgentSessionRecord | null {
  return state.sessions.find((record) => record.sessionId === sessionId && record.agentId === agentId) ?? null;
}

function normalizeSessionRecord(value: unknown): AgentSessionRecord | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const record = value as Partial<AgentSessionRecord>;
  const sessionId = String(record.sessionId || "").trim();
  const agentId = String(record.agentId || "").trim();
  if (!agentId || !isValidAgentSessionID(sessionId) || !Array.isArray(record.messages)) {
    return null;
  }
  const messages = record.messages
    .map(normalizeSessionMessage)
    .filter((message): message is AgentSessionMessage => Boolean(message))
    .slice(-SESSION_DEMO_MAX_MESSAGES);
  const updatedAt = String(record.updatedAt || "").trim();
  return {
    agentId,
    agentName: String(record.agentName || agentId).trim() || agentId,
    sessionId,
    messages,
    updatedAt: Number.isFinite(Date.parse(updatedAt)) ? updatedAt : new Date(0).toISOString(),
  };
}

function normalizeSessionMessage(value: unknown): AgentSessionMessage | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const message = value as Partial<AgentSessionMessage>;
  const role = message.role === "assistant" ? "assistant" : message.role === "user" ? "user" : null;
  const content = String(message.content || "").trim();
  if (!role || !content) {
    return null;
  }
  return {
    id: String(message.id || createAgentSessionID()),
    role,
    content,
    createdAt: String(message.createdAt || new Date(0).toISOString()),
  };
}
