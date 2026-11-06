import { useCallback, useMemo, useSyncExternalStore } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { AuthStatus } from "@/models/auth";
import type { CloudAgent, CloudAgentAvatarMetadata } from "@/models/cloudAgents";
import { cloudAgentCatalogKey } from "./useCloudAgentCatalog";

type CatalogSource = { data: readonly CloudAgent[]; updatedAt: number; targeted: boolean };

// Avatar metadata may come from either discovery or a direct conversation lookup.
// It is display-only: membership and availability still use their own query result.
export function useCloudAgentAvatarMetadata(auth?: AuthStatus): readonly CloudAgentAvatarMetadata[] {
  const client = useQueryClient();
  const cache = client.getQueryCache();
  const login = cloudAgentCatalogKey(auth)[2];
  const subscribe = useCallback((notify: () => void) => cache.subscribe(notify), [cache]);
  const getSnapshot = useMemo(() => {
    let previous: CatalogSource[] = [];
    let snapshot: readonly CloudAgentAvatarMetadata[] = [];
    return () => {
      const sources = cache
        .findAll({ queryKey: ["workspace", "cloud-agent-catalog", login] })
        .flatMap((query): CatalogSource[] => {
          const data = query.state.data as readonly CloudAgent[] | undefined;
          return data ? [{ data, updatedAt: query.state.dataUpdatedAt, targeted: Boolean(query.queryKey[3]) }] : [];
        });
      if (
        sources.length === previous.length &&
        sources.every(
          (source, index) =>
            source.data === previous[index].data &&
            source.updatedAt === previous[index].updatedAt &&
            source.targeted === previous[index].targeted,
        )
      )
        return snapshot;
      previous = sources;
      const metadata = new Map<string, CloudAgentAvatarMetadata>();
      // Prefer the freshest result; a targeted lookup wins timestamp ties.
      for (const source of [...sources].sort(
        (a, b) => a.updatedAt - b.updatedAt || Number(a.targeted) - Number(b.targeted),
      )) {
        for (const { id, connectionID, type, avatar } of source.data) {
          metadata.set(JSON.stringify([id, connectionID]), { id, connectionID, type, avatar });
        }
      }
      snapshot = [...metadata.values()];
      return snapshot;
    };
  }, [cache, login]);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
