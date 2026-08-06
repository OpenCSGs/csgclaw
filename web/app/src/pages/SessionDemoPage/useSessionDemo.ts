import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  cancelAgentSessionResponse,
  fetchSessionAgents,
  streamAgentSessionResponse,
  type SessionAgent,
} from "@/api/agentSessions";
import { errorMessage } from "@/api/client";
import {
  agentSessionConversationLabel,
  createAgentSessionID,
  isValidAgentSessionID,
  type AgentSessionMessage,
} from "@/models/agentSessions";
import type { TranslateFn } from "@/models/conversations";
import {
  findSessionRecord,
  loadSessionDemoStorage,
  saveSessionDemoStorage,
  upsertSessionRecord,
  type SessionDemoStorageState,
} from "@/pages/SessionDemoPage/sessionDemoStorage";
import { SESSION_DEMO_STORAGE_KEY } from "@/shared/storage/keys";

export type SessionDemoTransport = {
  listAgents: typeof fetchSessionAgents;
  streamResponse: typeof streamAgentSessionResponse;
  cancelResponse: typeof cancelAgentSessionResponse;
};

export const liveSessionDemoTransport: SessionDemoTransport = {
  listAgents: fetchSessionAgents,
  streamResponse: streamAgentSessionResponse,
  cancelResponse: cancelAgentSessionResponse,
};

export function useSessionDemo(t: TranslateFn, transport: SessionDemoTransport = liveSessionDemoTransport) {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialStorage = useMemo(() => loadSessionDemoStorage(window.localStorage, SESSION_DEMO_STORAGE_KEY), []);
  const storageRef = useRef<SessionDemoStorageState>(initialStorage);
  const requestedAgentName = useRef(String(searchParams.get("agent") || "").trim()).current;
  const requestedSessionID = useRef(String(searchParams.get("session") || "").trim()).current;
  const initialAgentID = initialStorage.currentAgentId;
  const initialSessionID = useRef(
    isValidAgentSessionID(requestedSessionID)
      ? requestedSessionID
      : initialStorage.currentSessionId || createAgentSessionID(),
  ).current;
  const initialRecord = requestedAgentName ? null : findSessionRecord(initialStorage, initialSessionID, initialAgentID);

  const [agents, setAgents] = useState<SessionAgent[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(true);
  const [agentsError, setAgentsError] = useState("");
  const [agentId, setAgentId] = useState(initialAgentID);
  const [sessionId, setSessionId] = useState(initialSessionID);
  const [sessionDraft, setSessionDraft] = useState(initialSessionID);
  const [sessionError, setSessionError] = useState("");
  const [messages, setMessages] = useState<AgentSessionMessage[]>(initialRecord?.messages ?? []);
  const [draft, setDraft] = useState("");
  const [pendingInput, setPendingInput] = useState("");
  const [streamingOutput, setStreamingOutput] = useState("");
  const [requestError, setRequestError] = useState("");
  const [busy, setBusy] = useState(false);
  const [canceling, setCanceling] = useState(false);
  const activeRequestRef = useRef<{
    controller: AbortController;
    agentName: string;
    sessionId: string;
    cancelPromise?: Promise<void>;
  } | null>(null);

  const selectedAgent = useMemo(() => agents.find((item) => item.id === agentId) ?? null, [agentId, agents]);
  const agentLocked = messages.length > 0 || busy;
  const conversationLabel = selectedAgent
    ? agentSessionConversationLabel(sessionId, selectedAgent.name, selectedAgent.id)
    : "";

  const persist = useCallback((state: SessionDemoStorageState) => {
    saveSessionDemoStorage(window.localStorage, SESSION_DEMO_STORAGE_KEY, state);
    storageRef.current = loadSessionDemoStorage(window.localStorage, SESSION_DEMO_STORAGE_KEY);
  }, []);

  useEffect(() => {
    let active = true;
    setAgentsLoading(true);
    transport
      .listAgents()
      .then((items) => {
        if (!active) return;
        setAgents(items);
        setAgentsError("");
        const requestedAgent = items.find(
          (item) => requestedAgentName && item.name.toLowerCase() === requestedAgentName.toLowerCase(),
        );
        const storedAgent = items.find((item) => item.id === initialStorage.currentAgentId);
        const nextAgentID = requestedAgent?.id || storedAgent?.id || items[0]?.id || "";
        setAgentId(nextAgentID);
        setMessages(findSessionRecord(storageRef.current, initialSessionID, nextAgentID)?.messages ?? []);
      })
      .catch((error) => {
        if (!active) return;
        setAgentsError(errorMessage(error, t("sessionDemoAgentsLoadFailed")));
      })
      .finally(() => {
        if (active) setAgentsLoading(false);
      });
    return () => {
      active = false;
    };
  }, [initialSessionID, initialStorage.currentAgentId, requestedAgentName, t, transport]);

  useEffect(() => {
    if (agentsLoading) return;
    const next = new URLSearchParams();
    if (selectedAgent) next.set("agent", selectedAgent.name);
    else if (requestedAgentName) next.set("agent", requestedAgentName);
    if (sessionId) next.set("session", sessionId);
    setSearchParams(next, { replace: true });
    persist({
      ...storageRef.current,
      currentAgentId: agentId,
      currentSessionId: sessionId,
    });
  }, [agentId, agentsLoading, persist, requestedAgentName, selectedAgent, sessionId, setSearchParams]);

  const selectAgent = useCallback(
    (nextAgentID: string) => {
      if (agentLocked) return;
      setAgentId(nextAgentID);
      setMessages(findSessionRecord(storageRef.current, sessionId, nextAgentID)?.messages ?? []);
      setStreamingOutput("");
      setRequestError("");
    },
    [agentLocked, sessionId],
  );

  const reuseSession = useCallback(() => {
    const nextSessionID = sessionDraft.trim();
    if (!isValidAgentSessionID(nextSessionID)) {
      setSessionError(t("sessionDemoSessionInvalid"));
      return;
    }
    setSessionError("");
    setSessionId(nextSessionID);
    setSessionDraft(nextSessionID);
    setMessages(findSessionRecord(storageRef.current, nextSessionID, agentId)?.messages ?? []);
    setStreamingOutput("");
    setRequestError("");
  }, [agentId, sessionDraft, t]);

  const newSession = useCallback(() => {
    const nextSessionID = createAgentSessionID();
    setSessionId(nextSessionID);
    setSessionDraft(nextSessionID);
    setMessages([]);
    setDraft("");
    setPendingInput("");
    setStreamingOutput("");
    setRequestError("");
    setSessionError("");
  }, []);

  const send = useCallback(async () => {
    const input = draft.trim();
    if (busy || !input) return;
    if (!selectedAgent) {
      setRequestError(t("sessionDemoSelectAgentFirst"));
      return;
    }
    if (!isValidAgentSessionID(sessionId)) {
      setSessionError(t("sessionDemoSessionInvalid"));
      return;
    }

    const controller = new AbortController();
    activeRequestRef.current = { controller, agentName: selectedAgent.name, sessionId };
    setBusy(true);
    setDraft("");
    setPendingInput(input);
    setStreamingOutput("");
    setRequestError("");
    try {
      const response = await transport.streamResponse(
        {
          agentName: selectedAgent.name,
          sessionId,
          input,
          signal: controller.signal,
        },
        (delta) => setStreamingOutput((current) => current + delta),
      );
      const output = response.text.trim();
      if (!output) {
        throw new Error(t("sessionDemoEmptyResponse"));
      }
      const now = new Date().toISOString();
      const nextMessages: AgentSessionMessage[] = [
        ...messages,
        { id: `user-${response.id}`, role: "user", content: input, createdAt: now },
        {
          id: `assistant-${response.id}`,
          role: "assistant",
          content: output,
          createdAt: now,
        },
      ];
      setMessages(nextMessages);
      persist(
        upsertSessionRecord(storageRef.current, {
          agentId: selectedAgent.id,
          agentName: selectedAgent.name,
          sessionId,
          messages: nextMessages,
          updatedAt: now,
        }),
      );
    } catch (error) {
      setDraft(input);
      const cancellationRequested =
        activeRequestRef.current?.controller === controller && activeRequestRef.current.cancelPromise !== undefined;
      if (cancellationRequested || (error instanceof DOMException && error.name === "AbortError")) {
        setRequestError(t("sessionDemoRequestCanceled"));
      } else {
        setRequestError(errorMessage(error, t("sessionDemoRequestFailed")));
      }
    } finally {
      const activeRequest = activeRequestRef.current;
      if (activeRequest?.controller === controller && activeRequest.cancelPromise) {
        try {
          await activeRequest.cancelPromise;
        } catch (error) {
          setRequestError(errorMessage(error, t("sessionDemoCancelFailed")));
        }
      }
      if (activeRequestRef.current?.controller === controller) activeRequestRef.current = null;
      setPendingInput("");
      setStreamingOutput("");
      setCanceling(false);
      setBusy(false);
    }
  }, [busy, draft, messages, persist, selectedAgent, sessionId, t, transport]);

  const cancel = useCallback(() => {
    const activeRequest = activeRequestRef.current;
    if (!activeRequest || activeRequest.cancelPromise) return;
    setCanceling(true);
    const cancelPromise = transport.cancelResponse({
      agentName: activeRequest.agentName,
      sessionId: activeRequest.sessionId,
    });
    activeRequest.cancelPromise = cancelPromise;
    void cancelPromise.catch(() => activeRequest.controller.abort());
  }, [transport]);

  return {
    agents,
    agentsError,
    agentsLoading,
    selectedAgent,
    agentId,
    agentLocked,
    selectAgent,
    sessionId,
    sessionDraft,
    sessionError,
    setSessionDraft,
    reuseSession,
    newSession,
    messages,
    draft,
    setDraft,
    pendingInput,
    streamingOutput,
    requestError,
    conversationLabel,
    busy,
    canceling,
    send,
    cancel,
  };
}
