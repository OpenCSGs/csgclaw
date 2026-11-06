import { useEffect } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { fetchRelayCatalog } from "@/api/relay";
import type { AuthStatus } from "@/models/auth";
import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";

export function cloudAgentCatalogKey(auth?: AuthStatus) {
  const login = auth?.authenticated
    ? JSON.stringify([auth.base_url, auth.user_uuid || auth.user_id, auth.logged_in_at])
    : "";
  return ["workspace", "cloud-agent-catalog", login] as const;
}

type CatalogPoller = { consumers: number; stop: () => void };
const pollers = new WeakMap<QueryClient, Map<string, CatalogPoller>>();

function subscribeToCatalogRefresh(client: QueryClient, login: string) {
  let catalogs = pollers.get(client);
  if (!catalogs) {
    catalogs = new Map();
    pollers.set(client, catalogs);
  }
  let poller = catalogs.get(login);
  if (!poller) {
    const refresh = () => {
      void client.refetchQueries(
        { queryKey: ["workspace", "cloud-agent-catalog", login], exact: true, type: "active" },
        { cancelRefetch: false },
      );
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
    catalogs.set(login, poller);
  }
  const sharedPoller = poller;
  const sharedCatalogs = catalogs;
  sharedPoller.consumers++;
  return () => {
    if (--sharedPoller.consumers === 0) {
      sharedPoller.stop();
      sharedCatalogs.delete(login);
    }
  };
}

/** Passive consumers observe cached metadata without starting requests or refresh timers. */
export function useCloudAgentCatalog(auth?: AuthStatus, enabled = true) {
  const client = useQueryClient();
  const queryKey = cloudAgentCatalogKey(auth);
  const login = queryKey[2];
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => fetchRelayCatalog(signal),
    enabled,
    retry: false,
    staleTime: 5000,
    refetchOnWindowFocus: false,
  });
  useEffect(() => {
    if (!enabled) return;
    return subscribeToCatalogRefresh(client, login);
  }, [client, enabled, login]);
  return query;
}
