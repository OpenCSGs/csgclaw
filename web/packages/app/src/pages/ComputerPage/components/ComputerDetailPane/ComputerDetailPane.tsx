import { ComputerAgentList } from "./ComputerAgentList";
import { EntityAvatar, EntityHeader, EntityPane } from "@/components/business/EntityDetail";
import computerDetailPaneStyles from "./ComputerDetailPane.module.css";
import { isAgentRunning } from "@/models/agents";
import { ComputerIcon } from "@/components/business/Icons";
import { StatusBadge } from "@/components/ui";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import { AgentRuntimeSection } from "@/components/business/AgentRuntimeSection";
import type { AgentRuntimeSectionProps } from "@/components/business/AgentRuntimeSection";

type VoidOrPromise = void | Promise<void>;

export type ComputerDetailPaneProps = {
  agents?: AgentLike[];
  busyKey?: string;
  busyKeys?: readonly string[];
  channels?: IMConversation[];
  directMessages?: IMConversation[];
  onCreateAgent?: () => VoidOrPromise;
  onSelectAgent?: (item: AgentLike) => void;
  onStartAgent?: (item: AgentLike) => VoidOrPromise;
  runtimeSectionProps?: AgentRuntimeSectionProps;
  t?: TranslateFn;
};

export function ComputerDetailPane({
  agents = [],
  channels = [],
  directMessages = [],
  busyKey = "",
  busyKeys = [],
  onSelectAgent = () => {},
  onCreateAgent = () => {},
  onStartAgent = () => {},
  runtimeSectionProps,
  t = (key) => key,
}: ComputerDetailPaneProps) {
  const runningAgents = agents.filter(isAgentRunning);
  const actionBusyKeys = busyKeys.length ? busyKeys : busyKey ? [busyKey] : [];
  return (
    <EntityPane className={`computer-detail-pane ${computerDetailPaneStyles["computer-detail-pane"]}`}>
      <section className={`computer-overview-card ${computerDetailPaneStyles["computer-overview-card"]}`}>
        <EntityHeader
          avatar={
            <EntityAvatar>
              <ComputerIcon />
            </EntityAvatar>
          }
          title={
            <>
              <h1>{t("localComputer")}</h1>
              <StatusBadge tone="positive" className={"online"}>
                {t("online")}
              </StatusBadge>
            </>
          }
        >
          <p>{t("computerOverview")}</p>
        </EntityHeader>
        <div
          className={`computer-summary-strip ${computerDetailPaneStyles["computer-summary-strip"]}`}
          aria-label={t("computerOverview")}
        >
          <div className={`computer-summary-item ${computerDetailPaneStyles["computer-summary-item"]}`}>
            <span>{t("computerAgentsSection")}</span>
            <strong>{agents.length}</strong>
          </div>
          <div className={`computer-summary-item ${computerDetailPaneStyles["computer-summary-item"]}`}>
            <span>{t("activeNow")}</span>
            <strong>{runningAgents.length}</strong>
          </div>
          <div className={`computer-summary-item ${computerDetailPaneStyles["computer-summary-item"]}`}>
            <span>{t("channelsSection")}</span>
            <strong>{channels.length}</strong>
          </div>
          <div className={`computer-summary-item ${computerDetailPaneStyles["computer-summary-item"]}`}>
            <span>{t("directMessagesSection")}</span>
            <strong>{directMessages.length}</strong>
          </div>
        </div>
      </section>
      {runtimeSectionProps ? <AgentRuntimeSection {...runtimeSectionProps} /> : null}
      <ComputerAgentList
        agents={agents}
        actionBusyKeys={actionBusyKeys}
        onCreateAgent={onCreateAgent}
        onSelectAgent={onSelectAgent}
        onStartAgent={onStartAgent}
        t={t}
      />
    </EntityPane>
  );
}
