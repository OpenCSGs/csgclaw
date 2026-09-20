import { useCallback, useEffect, useRef, useState } from "react";
import {
  connectAgentApp,
  createAgentApp,
  deleteAgentApp,
  disconnectAgentApp,
  fetchAgentApps,
  fetchAppDefinitions,
  updateAgentApp,
  type AppCreateRequest,
  type AppDefinition,
  type AppInstallation,
  type AppUpdateRequest,
} from "@/api/apps";

export function useAgentApps(agentID: string, active: boolean) {
  const [snapshot, setSnapshot] = useState<{
    agentID: string;
    items: AppInstallation[];
    definitions: AppDefinition[];
    feishuChannelAvailable?: boolean;
  }>({
    agentID: "",
    items: [],
    definitions: [],
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busyID, setBusyID] = useState("");
  const requestSequence = useRef(0);
  const currentAgent = useRef(agentID);

  useEffect(() => {
    currentAgent.current = agentID;
    setBusyID("");
    setError(null);
  }, [agentID]);

  const reload = useCallback(
    async (signal?: AbortSignal, silent = false) => {
      if (currentAgent.current !== agentID) return;
      const sequence = ++requestSequence.current;
      if (!silent) setLoading(true);
      try {
        const [definitions, apps] = await Promise.all([fetchAppDefinitions(signal), fetchAgentApps(agentID, signal)]);
        if (!signal?.aborted && sequence === requestSequence.current) {
          setSnapshot({
            agentID,
            definitions,
            items: apps.items,
            feishuChannelAvailable: apps.feishu_channel_available,
          });
          setError(null);
        }
      } catch (failure) {
        if (!signal?.aborted && sequence === requestSequence.current) setError(failure);
      } finally {
        if (!signal?.aborted && sequence === requestSequence.current) setLoading(false);
      }
    },
    [agentID],
  );

  useEffect(() => {
    if (!active || !agentID) return;
    const controller = new AbortController();
    void reload(controller.signal);
    const interval = window.setInterval(() => {
      if (!document.hidden) void reload(controller.signal, true);
    }, 15000);
    return () => {
      controller.abort();
      requestSequence.current += 1;
      window.clearInterval(interval);
    };
  }, [active, agentID, reload]);

  async function mutate<T>(id: string, operation: () => Promise<T>): Promise<T> {
    setBusyID(id);
    try {
      const result = await operation();
      await reload();
      return result;
    } finally {
      if (currentAgent.current === agentID) setBusyID("");
    }
  }

  return {
    items: snapshot.agentID === agentID ? snapshot.items : [],
    definitions: snapshot.definitions,
    feishuChannelAvailable: snapshot.agentID === agentID ? snapshot.feishuChannelAvailable : undefined,
    loading,
    error,
    busyID,
    reload: () => reload(),
    create: (payload: AppCreateRequest) => mutate("new", () => createAgentApp(agentID, payload)),
    update: (id: string, payload: AppUpdateRequest) => mutate(id, () => updateAgentApp(agentID, id, payload)),
    connect: (id: string) => mutate(id, () => connectAgentApp(agentID, id)),
    disconnect: (id: string) => mutate(id, () => disconnectAgentApp(agentID, id)),
    remove: (id: string) => mutate(id, () => deleteAgentApp(agentID, id)),
  };
}

export type AgentAppsController = ReturnType<typeof useAgentApps>;
