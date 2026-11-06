import { useEffect } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { fetchRelayCatalog, type RelayCatalogTarget } from "@/api/relay";
import type { AuthStatus } from "@/models/auth";
import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";

function catalogKey(login: string, agentID = "", connectionID = "") {
  return ["workspace", "cloud-agent-catalog", login, agentID, connectionID] as const;
}

export function cloudAgentCatalogKey(auth?: AuthStatus, target?: RelayCatalogTarget) {
  const login = auth?.authenticated
    ? JSON.stringify([auth.base_url, auth.user_uuid || auth.user_id, auth.logged_in_at])
    : "";
  return catalogKey(login, target?.agent_id, target?.connection_id);
}

type CatalogPoller = { consumers: number; stop: () => void };
const pollers = new WeakMap<QueryClient, Map<string, CatalogPoller>>();

function subscribeToCatalogRefresh(client: QueryClient, login: string, agentID: string, connectionID: string) {
  const queryKey = catalogKey(login, agentID, connectionID);
  const scope = JSON.stringify(queryKey);
  let catalogs = pollers.get(client);
  if (!catalogs) {
    catalogs = new Map();
    pollers.set(client, catalogs);
  }
  let poller = catalogs.get(scope);
  if (!poller) {
    const refresh = () => {
      void client.refetchQueries({ queryKey, exact: true, type: "active" }, { cancelRefetch: false });
    };
    const timer = window.setInterval(refresh, 5000);
    window.addEventListener("focus", refresh);
    window.addEventListener(RELAY_PUBLICATIONS_CHANGED_EVENT, refresh);
    poller = {
      consumers: 0,
      stop: () => {
        window.clearInterval(timer);
        window.removeEventListener("focus", refresh);
        window.removeEventListener(RELAY_PUBLICATIONS_CHANGED_EVENT, refresh);
      },
    };
    catalogs.set(scope, poller);
  }
  const sharedPoller = poller;
  const sharedCatalogs = catalogs;
  sharedPoller.consumers++;
  return () => {
    if (--sharedPoller.consumers === 0) {
      sharedPoller.stop();
      sharedCatalogs.delete(scope);
    }
  };
}

/** Passive consumers observe cached metadata without starting requests or refresh timers. */
export function useCloudAgentCatalog(auth?: AuthStatus, enabled = true, target?: RelayCatalogTarget) {
  const client = useQueryClient();
  const queryKey = cloudAgentCatalogKey(auth, target);
  const login = queryKey[2];
  const agentID = queryKey[3];
  const connectionID = queryKey[4];
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => fetchRelayCatalog(signal, target),
    enabled,
    retry: false,
    staleTime: 5000,
    refetchOnWindowFocus: false,
  });
  useEffect(() => {
    if (!enabled) return;
    return subscribeToCatalogRefresh(client, login, agentID, connectionID);
  }, [client, enabled, login, agentID, connectionID]);
  return query;
}
