import { createUUID } from "@/shared/lib/uuid";
import { useEffect, useRef, useState } from "react";
import { fetchCloudAgentResponse, sendCloudAgentMessage, type CloudChatTurn } from "@/api/relay";
import type { CloudAgent } from "@/models/cloudAgents";
import { cloudResponseText, emptyCloudChat, parseCloudChat, type CloudChatState } from "@/models/cloudChat";
import { CLOUD_CHAT_STORAGE_KEY } from "@/shared/storage/keys";

export function useCloudAgentChat(agent: CloudAgent) {
  const key = `${CLOUD_CHAT_STORAGE_KEY}:${agent.connectionID}:${agent.id}`;
  const [state, setState] = useState<CloudChatState>(() => {
    try {
      return parseCloudChat(localStorage.getItem(key));
    } catch {
      return emptyCloudChat();
    }
  });
  const current = useRef(state);
  const mounted = useRef(false);
  const running = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [storageError, setStorageError] = useState(false);
  const executeRef = useRef<(turn: CloudChatTurn) => Promise<void>>(async () => {});
  function commit(next: CloudChatState) {
    if (!mounted.current) return;
    current.current = next;
    setState(next);
    try {
      localStorage.setItem(key, JSON.stringify(next));
      setStorageError(false);
    } catch {
      setStorageError(true);
    }
  }
  async function execute(turn: CloudChatTurn) {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setError(null);
    try {
      let active = turn;
      if (!active.responseID && Date.now() - active.submittedAt > 10 * 60 * 1000)
        throw {
          code: "relay_chat_request_expired",
          message: "Pending request is too old to resubmit safely. Start a new conversation.",
        };
      const deadline = Date.now() + 6 * 60 * 1000;
      while (mounted.current) {
        if (Date.now() > deadline)
          throw {
            code: "relay_chat_pending_timeout",
            message: "Response is still pending. Retry to query the same request.",
          };
        const response = active.responseID
          ? await fetchCloudAgentResponse(agent, active.responseID)
          : await sendCloudAgentMessage(agent, active);
        if (!mounted.current) return;
        if (response.status === "queued" || response.status === "in_progress") {
          active = { ...active, responseID: response.id };
          commit({ ...current.current, pending: active });
          await new Promise<void>((resolve) => setTimeout(resolve, 1000));
          continue;
        }
        if (response.status !== "completed") {
          commit({ ...current.current, pending: null });
          setError({
            code: response.error?.code || "relay_chat_failed",
            message: response.error?.message || response.status,
          });
          return;
        }
        commit({
          messages: [
            ...current.current.messages,
            { id: response.id, role: "assistant", text: cloudResponseText(response) },
          ],
          previousResponseID: response.id,
          pending: null,
        });
        return;
      }
    } catch (err) {
      if (mounted.current) setError(err);
    } finally {
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  executeRef.current = execute;
  useEffect(() => {
    mounted.current = true;
    if (current.current.pending) void executeRef.current(current.current.pending);
    return () => {
      mounted.current = false;
    };
  }, []);
  return {
    state,
    busy,
    error,
    storageError,
    send: (input: string) => {
      const text = input.trim();
      if (!text || running.current || current.current.pending) return;
      const turn: CloudChatTurn = {
        key: createUUID(),
        submittedAt: Date.now(),
        input: text,
        previousResponseID: current.current.previousResponseID,
      };
      commit({
        ...current.current,
        messages: [...current.current.messages, { id: turn.key, role: "user", text }],
        pending: turn,
      });
      void execute(turn);
    },
    retry: () => {
      if (current.current.pending) void execute(current.current.pending);
    },
    reset: () => {
      if (!running.current) {
        commit(emptyCloudChat());
        setError(null);
      }
    },
  };
}
