import { SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import {
  agentModelID,
  agentProfileConfig,
  formatProviderLabel,
  isAgentIncomplete,
  isAgentRunning,
} from "@/models/agents";
import { providerNameForProviderID } from "@/models/modelProviders";
import { ComputerIcon, PlayIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";
import { AgentRuntimeSection } from "../AgentRuntimeSection";
import type { AgentRuntimeSectionProps } from "../AgentRuntimeSection";

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
    <section className="entity-pane computer-detail-pane">
      <section className="computer-overview-card">
        <header className="entity-header">
          <div className="entity-avatar">
            <ComputerIcon />
          </div>
          <div className="entity-heading">
            <div className="entity-title-row">
              <h1>{t("localComputer")}</h1>
              <span className="status-pill online">{t("online")}</span>
            </div>
            <p>{t("computerOverview")}</p>
          </div>
        </header>
        <div className="computer-summary-strip" aria-label={t("computerOverview")}>
          <div className="computer-summary-item">
            <span>{t("computerAgentsSection")}</span>
            <strong>{agents.length}</strong>
          </div>
          <div className="computer-summary-item">
            <span>{t("activeNow")}</span>
            <strong>{runningAgents.length}</strong>
          </div>
          <div className="computer-summary-item">
            <span>{t("channelsSection")}</span>
            <strong>{channels.length}</strong>
          </div>
          <div className="computer-summary-item">
            <span>{t("directMessagesSection")}</span>
            <strong>{directMessages.length}</strong>
          </div>
        </div>
      </section>
      {runtimeSectionProps ? <AgentRuntimeSection {...runtimeSectionProps} /> : null}
      <section className="computer-agent-panel">
        <div className="section-header-inline">
          <div className="section-label">{t("computerAgentsSection")}</div>
          <Button variant="primary" size="md" onClick={onCreateAgent}>
            {t("createAgent")}
          </Button>
        </div>
        <div className="entity-list computer-agent-list">
          {agents.length ? (
            agents.map((item) => (
              <div key={item.id} className="entity-list-row">
                <button className="entity-list-main-button" onClick={() => onSelectAgent(item)}>
                  <span className="entity-list-icon">
                    <AgentAvatarContent
                      avatar={item.avatar}
                      fallback={avatarFallbackText(item.avatar, item.name, item.id)}
                    />
                  </span>
                  <span className="entity-list-main">
                    <strong>{item.name}</strong>
                    <small>
                      {formatProviderLabel(
                        item.provider ||
                          agentProfileConfig(item)?.provider ||
                          providerNameForProviderID(agentProfileConfig(item)?.model_provider_id || ""),
                      )}{" "}
                      · {agentModelID(item)}
                    </small>
                  </span>
                  <span
                    className={`workspace-status-dot ${isAgentRunning(item) ? "online" : ""}`}
                    aria-hidden="true"
                  ></span>
                </button>
                {SHOW_AGENT_LIFECYCLE_ACTIONS ? (
                  <Button
                    className="agent-icon-button"
                    disabled={
                      actionBusyKeys.some((actionBusyKey) => actionBusyKey.startsWith(`${item.id}:`)) ||
                      isAgentIncomplete(item)
                    }
                    onClick={() => onStartAgent(item)}
                  >
                    <span aria-hidden="true">
                      <PlayIcon />
                    </span>
                  </Button>
                ) : null}
              </div>
            ))
          ) : (
            <div className="agent-empty">{t("noAgents")}</div>
          )}
        </div>
      </section>
    </section>
  );
}
