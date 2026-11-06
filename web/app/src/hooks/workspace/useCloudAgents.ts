import { useEffect, useState } from "react";
import type { AuthStatus } from "@/models/auth";
import { fetchRelayCatalog } from "@/api/relay";
import { parseCloudAgents, sameCloudAgentReference, type CloudAgent } from "@/models/cloudAgents";
import { CLOUD_AGENTS_STORAGE_KEY } from "@/shared/storage/keys";
import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";

const MEMBERSHIP_CHANGED_EVENT = "csgclaw-cloud-membership-changed";

export function useCloudAgents(auth?: AuthStatus, enabled = true) {
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
  const [result, setResult] = useState<{ login: string; catalog: CloudAgent[]; error: unknown } | null>(null);
  const currentResult = result?.login === login ? result : null;
  const catalog = currentResult?.catalog ?? [];
  const [refreshVersion, setRefreshVersion] = useState(0);
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
    let active = true;
    let request: AbortController | null = null;
    const load = async () => {
      if (request) return;
      const controller = new AbortController();
      request = controller;
      try {
        const data = await fetchRelayCatalog(controller.signal);
        if (active) {
          setResult({ login, catalog: data, error: null });
        }
      } catch (err) {
        if (active) {
          setResult({ login, catalog: [], error: err });
        }
      } finally {
        request = null;
      }
    };
    void load();
    const refresh = () => {
      void load();
    };
    const timer = window.setInterval(refresh, 5000);
    window.addEventListener("focus", refresh);
    window.addEventListener(RELAY_PUBLICATIONS_CHANGED_EVENT, refresh);
    return () => {
      active = false;
      request?.abort();
      window.clearInterval(timer);
      window.removeEventListener("focus", refresh);
      window.removeEventListener(RELAY_PUBLICATIONS_CHANGED_EVENT, refresh);
    };
  }, [login, refreshVersion, enabled]);
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
    loading: !currentResult,
    error: currentResult?.error ?? null,
    refresh: () => setRefreshVersion((version) => version + 1),
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
