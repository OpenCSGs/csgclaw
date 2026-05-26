import { SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import { agentModelID, formatProviderLabel, isAgentIncomplete, isAgentRunning } from "@/models/agents";
import { AgentIcon, ComputerIcon, PlayIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";

export function ComputerDetailPane({
  t,
  agents,
  channels,
  directMessages,
  busyKey,
  onSelectAgent,
  onCreateAgent,
  onStartAgent,
}) {
  const runningAgents = agents.filter(isAgentRunning);
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
                    <AgentIcon />
                  </span>
                  <span className="entity-list-main">
                    <strong>{item.name}</strong>
                    <small>
                      {formatProviderLabel(item.provider || item.agent_profile?.provider)} · {agentModelID(item)}
                    </small>
                  </span>
                  <span className={`workspace-status-dot ${isAgentRunning(item) ? "online" : ""}`}></span>
                </button>
                {SHOW_AGENT_LIFECYCLE_ACTIONS ? (
                  <Button
                    className="agent-icon-button"
                    disabled={busyKey.startsWith(`${item.id}:`) || isAgentIncomplete(item)}
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
