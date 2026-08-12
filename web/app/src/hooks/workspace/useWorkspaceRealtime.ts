import { useEffect, useRef } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import type { FetchAgentsOptions } from "@/api/agents";
import { agentMatchesUser, applyIMEvent, isAgentRosterEvent, resolveUserByLocalIdentity } from "@/models/conversations";
import { isAgentRunning } from "@/models/agents";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import type { AgentLike } from "@/models/agents";
import type { IMData, IMServerEvent, UsersById } from "@/models/conversations";
import type { WorkspaceQuerySetter } from "./types";
import { workspaceQueryKeys } from "./workspaceQueries";

const WORKSPACE_REALTIME_REFRESH_DEBOUNCE_MS = 100;

type WorkspaceRealtimeHandler = (payload: IMServerEvent) => void;

type UseWorkspaceRealtimeArgs = {
  agents: readonly AgentLike[];
  onConversationEvent?: WorkspaceRealtimeHandler;
  onFloatingConversationEvent?: WorkspaceRealtimeHandler;
  onAgentTurnNotificationEvent?: WorkspaceRealtimeHandler;
  onParticipantWorkEvent?: WorkspaceRealtimeHandler;
  onRefreshAgentState: (agentID: string) => Promise<AgentLike | null>;
  onUpgradeStatusChange: (payload: unknown) => void;
  refreshWorkspaceAgents: (options?: FetchAgentsOptions) => Promise<AgentLike[]>;
  refreshWorkspaceBootstrap: () => Promise<IMData | null>;
  setBootstrapData: WorkspaceQuerySetter<IMData | null>;
  usersById: UsersById;
};

type WorkspaceRealtimeRefs = UseWorkspaceRealtimeArgs & {
  queryClient: QueryClient;
};

function isParticipantRosterEvent(event: IMServerEvent | null | undefined): boolean {
  return (
    event?.type === "participant.created" ||
    event?.type === "participant.updated" ||
    event?.type === "participant.deleted"
  );
}

function isTeamRosterEvent(event: IMServerEvent | null | undefined): boolean {
  return event?.type === "team.created" || event?.type === "team.updated" || event?.type === "team.deleted";
}

export function useWorkspaceRealtime({
  agents,
  onConversationEvent,
  onFloatingConversationEvent,
  onAgentTurnNotificationEvent,
  onParticipantWorkEvent,
  onRefreshAgentState,
  onUpgradeStatusChange,
  refreshWorkspaceAgents,
  refreshWorkspaceBootstrap,
  setBootstrapData,
  usersById,
}: UseWorkspaceRealtimeArgs): void {
  const queryClient = useQueryClient();
  const refs = useRef<WorkspaceRealtimeRefs>({
    agents,
    onConversationEvent,
    onFloatingConversationEvent,
    onAgentTurnNotificationEvent,
    onParticipantWorkEvent,
    onRefreshAgentState,
    onUpgradeStatusChange,
    queryClient,
    refreshWorkspaceAgents,
    refreshWorkspaceBootstrap,
    setBootstrapData,
    usersById,
  });
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingBootstrapRefreshRef = useRef(false);
  const pendingAgentsRefreshRef = useRef(false);
  const pendingTeamsRefreshRef = useRef(false);

  useEffect(() => {
    refs.current = {
      agents,
      onConversationEvent,
      onFloatingConversationEvent,
      onAgentTurnNotificationEvent,
      onParticipantWorkEvent,
      onRefreshAgentState,
      onUpgradeStatusChange,
      queryClient,
      refreshWorkspaceAgents,
      refreshWorkspaceBootstrap,
      setBootstrapData,
      usersById,
    };
  }, [
    agents,
    onConversationEvent,
    onFloatingConversationEvent,
    onAgentTurnNotificationEvent,
    onParticipantWorkEvent,
    onRefreshAgentState,
    onUpgradeStatusChange,
    queryClient,
    refreshWorkspaceAgents,
    refreshWorkspaceBootstrap,
    setBootstrapData,
    usersById,
  ]);

  useEffect(() => {
    function scheduleRefresh(options: { agents?: boolean; bootstrap?: boolean; teams?: boolean }) {
      pendingAgentsRefreshRef.current = pendingAgentsRefreshRef.current || Boolean(options.agents);
      pendingBootstrapRefreshRef.current = pendingBootstrapRefreshRef.current || Boolean(options.bootstrap);
      pendingTeamsRefreshRef.current = pendingTeamsRefreshRef.current || Boolean(options.teams);
      if (refreshTimerRef.current) {
        clearTimeout(refreshTimerRef.current);
      }
      refreshTimerRef.current = setTimeout(() => {
        refreshTimerRef.current = null;
        const shouldRefreshAgents = pendingAgentsRefreshRef.current;
        const shouldRefreshBootstrap = pendingBootstrapRefreshRef.current;
        const shouldRefreshTeams = pendingTeamsRefreshRef.current;
        pendingAgentsRefreshRef.current = false;
        pendingBootstrapRefreshRef.current = false;
        pendingTeamsRefreshRef.current = false;

        const current = refs.current;
        void Promise.all([
          shouldRefreshBootstrap ? current.refreshWorkspaceBootstrap() : Promise.resolve(null),
          shouldRefreshAgents ? current.refreshWorkspaceAgents({ silent: true }) : Promise.resolve([]),
          shouldRefreshTeams
            ? current.queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.teams() })
            : Promise.resolve(),
        ]);
      }, WORKSPACE_REALTIME_REFRESH_DEBOUNCE_MS);
    }

    function handleEvent(payload: IMServerEvent) {
      const current = refs.current;
      current.setBootstrapData((data) => applyIMEvent(data, payload));
      current.onConversationEvent?.(payload);
      current.onFloatingConversationEvent?.(payload);
      current.onAgentTurnNotificationEvent?.(payload);
      current.onParticipantWorkEvent?.(payload);

      const participantRosterEvent = isParticipantRosterEvent(payload);
      if (participantRosterEvent || isAgentRosterEvent(payload)) {
        scheduleRefresh({ agents: true, bootstrap: participantRosterEvent });
      }
      if (isTeamRosterEvent(payload)) {
        scheduleRefresh({ teams: true });
      }

      if (payload?.type === "message.created" && payload.message) {
        const senderID = String(payload.message.sender_id || "").trim();
        if (senderID) {
          const sender = resolveUserByLocalIdentity(senderID, current.usersById) ?? { id: senderID };
          const senderAgent = current.agents.find((agent) => agentMatchesUser(agent, sender));
          if (senderAgent?.id && !isAgentRunning(senderAgent)) {
            void current.onRefreshAgentState(String(senderAgent.id));
          }
        }
      }

      if (payload?.type === "upgrade.status_changed" && payload.upgrade) {
        current.onUpgradeStatusChange(payload.upgrade);
      }
    }

    const unsubscribe = subscribeIMEvents(handleEvent);

    return () => {
      unsubscribe();
      if (refreshTimerRef.current) {
        clearTimeout(refreshTimerRef.current);
        refreshTimerRef.current = null;
      }
      pendingAgentsRefreshRef.current = false;
      pendingBootstrapRefreshRef.current = false;
      pendingTeamsRefreshRef.current = false;
    };
  }, []);
}
