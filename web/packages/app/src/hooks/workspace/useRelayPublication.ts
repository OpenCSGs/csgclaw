import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchRelayPublication, publishRelayAgent, unpublishRelayAgent } from "@/api/relay";

export function useRelayPublication(agentID: string) {
  const client = useQueryClient();
  const queryKey = ["relay-publication", agentID];
  const status = useQuery({
    queryKey,
    enabled: Boolean(agentID),
    queryFn: () => fetchRelayPublication(agentID),
    refetchInterval: 5000,
  });
  const publish = useMutation({
    mutationKey: ["relay-publication", agentID, "publish"],
    mutationFn: async () => {
      const id = agentID;
      return { id, data: await publishRelayAgent(id) };
    },
    onSuccess: ({ id, data }) => {
      client.setQueryData(["relay-publication", id], data);
      window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT));
    },
  });
  const unpublish = useMutation({
    mutationKey: ["relay-publication", agentID, "unpublish"],
    mutationFn: async () => {
      const id = agentID;
      return { id, data: await unpublishRelayAgent(id) };
    },
    onSuccess: ({ id, data }) => {
      client.setQueryData(["relay-publication", id], data);
      window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT));
    },
  });
  return { status, publish, unpublish };
}

export type RelayPublicationController = ReturnType<typeof useRelayPublication>;
