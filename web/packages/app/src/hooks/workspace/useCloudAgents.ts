import { useEffect, useState } from "react";
import type { RelayCatalogTarget } from "@/api/relay";
import type { AuthStatus } from "@/models/auth";
import { useCloudAgentCatalog } from "./useCloudAgentCatalog";
import { parseCloudAgents, sameCloudAgentReference, type CloudAgent } from "@/models/cloudAgents";
import { CLOUD_AGENTS_STORAGE_KEY } from "@/shared/storage/keys";

const MEMBERSHIP_CHANGED_EVENT = "csgclaw-cloud-membership-changed";

export function useCloudAgents(auth?: AuthStatus, enabled = true, target?: RelayCatalogTarget, metadataOnly = false) {
  const query = useCloudAgentCatalog(auth, enabled, target, metadataOnly);
  const [savedAgents, setSavedAgents] = useState<CloudAgent[]>(() => {
    try {
      return parseCloudAgents(localStorage.getItem(CLOUD_AGENTS_STORAGE_KEY));
    } catch {
      return [];
    }
  });
  const catalog = query.isError ? [] : (query.data ?? []);
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
    refresh: () => void query.refetch({ cancelRefetch: false }),
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
