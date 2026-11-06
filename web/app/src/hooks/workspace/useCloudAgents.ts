import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { AuthStatus } from "@/models/auth";
import { fetchRelayCatalog, type RelayCatalogTarget } from "@/api/relay";
import { parseCloudAgents, sameCloudAgentReference, type CloudAgent } from "@/models/cloudAgents";
import { CLOUD_AGENTS_STORAGE_KEY } from "@/shared/storage/keys";
import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";

const MEMBERSHIP_CHANGED_EVENT = "csgclaw-cloud-membership-changed";

export function useCloudAgents(auth?: AuthStatus, enabled = true, target?: RelayCatalogTarget) {
  const login = auth?.authenticated
    ? JSON.stringify([auth.base_url, auth.user_uuid || auth.user_id, auth.logged_in_at])
    : "";
  const [savedAgents, setSavedAgents] = useState<CloudAgent[]>(() => {
    try {
      return parseCloudAgents(localStorage.getItem(CLOUD_AGENTS_STORAGE_KEY));
    } catch {
      return [];
    }
  });
  const queryClient = useQueryClient();
  const agentID = target?.agent_id;
  const connectionID = target?.connection_id;
  const queryKey = useMemo(() => ["cloud-agent-catalog", login, agentID, connectionID], [login, agentID, connectionID]);
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => fetchRelayCatalog(signal, target),
    enabled,
    staleTime: 5000,
    refetchInterval: 5000,
    retry: false,
  });
  const catalog = query.isError ? [] : (query.data ?? []);
  const refresh = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey, exact: true }, { cancelRefetch: false });
  }, [queryClient, queryKey]);
  const [storageError, setStorageError] = useState(false);
  useEffect(() => {
    const sync = () => {
      try {
        setSavedAgents(parseCloudAgents(localStorage.getItem(CLOUD_AGENTS_STORAGE_KEY)));
      } catch {
        setSavedAgents([]);
      }
    };
    const onStorage = (event: StorageEvent) => {
      if (event.key === null || event.key === CLOUD_AGENTS_STORAGE_KEY) sync();
    };
    // Retire the prototype cache, which could contain mock relay registrations.
    try {
      localStorage.removeItem("csgclaw.agents.cloud.v2");
    } catch {
      // Storage can be unavailable; the old cache is never read.
    }
    sync();
    window.addEventListener(MEMBERSHIP_CHANGED_EVENT, sync);
    window.addEventListener("storage", onStorage);
    return () => {
      window.removeEventListener(MEMBERSHIP_CHANGED_EVENT, sync);
      window.removeEventListener("storage", onStorage);
    };
  }, []);
  useEffect(() => {
    if (!enabled) return;
    window.addEventListener("focus", refresh);
    window.addEventListener(RELAY_PUBLICATIONS_CHANGED_EVENT, refresh);
    return () => {
      window.removeEventListener("focus", refresh);
      window.removeEventListener(RELAY_PUBLICATIONS_CHANGED_EVENT, refresh);
    };
  }, [enabled, refresh]);
  function save(update: (agents: CloudAgent[]) => CloudAgent[]) {
    try {
      const agents = update(parseCloudAgents(localStorage.getItem(CLOUD_AGENTS_STORAGE_KEY)));
      localStorage.setItem(CLOUD_AGENTS_STORAGE_KEY, JSON.stringify(agents));
      setSavedAgents(agents);
      setStorageError(false);
      window.dispatchEvent(new Event(MEMBERSHIP_CHANGED_EVENT));
      return true;
    } catch {
      setStorageError(true);
      return false;
    }
  }
  return {
    catalog,
    loading: query.isPending,
    error: query.error,
    refresh,
    addedIDs: catalog
      .filter((agent) => savedAgents.some((saved) => sameCloudAgentReference(saved, agent)))
      .map((agent) => agent.id),
    // Community membership is authoritative; cached metadata cannot resurrect an
    // unpublished agent or expose a different site's/account's saved selection.
    agents: catalog.filter((agent) => savedAgents.some((saved) => sameCloudAgentReference(agent, saved))),
    storageError,
    add: (id: string) => {
      const agent = catalog.find((item) => item.id === id);
      return agent ? save((saved) => [...saved.filter((item) => !sameCloudAgentReference(item, agent)), agent]) : false;
    },
    remove: (id: string) => {
      const agent = catalog.find((item) => item.id === id);
      return agent ? save((saved) => saved.filter((item) => !sameCloudAgentReference(item, agent))) : false;
    },
  };
}
