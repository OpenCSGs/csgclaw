import { SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import {
  agentModelID,
  formatProviderLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentRunning,
  isNotificationBotAgent,
} from "@/models/agents";
import { AgentIcon, PlayIcon, StopIcon, TrashIcon, WrenchIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";

export function AgentSection({
  title,
  manager,
  workers,
  t,
  activeRoom,
  busyKey,
  error,
  onCreate,
  onEdit,
  onStart,
  onStop,
  onRecreate,
  onDelete,
  onInvite,
}) {
  const items = [manager, ...workers].filter(Boolean);
  return (
    <section className="agent-section">
      <div className="agent-section-head">
        <div>
          <div className="section-label">
            {title} {items.length}
          </div>
        </div>
        <Button className="agent-add-button" aria-label={t("createAgent")} title={t("createAgent")} onClick={onCreate}>
          <span aria-hidden="true">
            <AgentIcon />
          </span>
        </Button>
      </div>
      <div className="agent-list">
        {items.length ? (
          items.map((item) => (
            <AgentRow
              key={item.id}
              item={item}
              t={t}
              activeRoom={activeRoom}
              busyKey={busyKey}
              onEdit={onEdit}
              onStart={onStart}
              onStop={onStop}
              onRecreate={onRecreate}
              onDelete={onDelete}
              onInvite={onInvite}
            />
          ))
        ) : (
          <div className="agent-empty">{t("noAgents")}</div>
        )}
      </div>
      {error ? <div className="form-error agent-error">{error}</div> : null}
    </section>
  );
}

export function AgentRow({ item, t, activeRoom, busyKey, onEdit, onStart, onStop, onRecreate, onDelete, onInvite }) {
  const isManager = item.role === "manager" || item.id === "u-manager";
  const isNotification = isNotificationBotAgent(item);
  const running = isAgentRunning(item);
  const incomplete = isAgentIncomplete(item);
  const restartNeeded = isAgentRestartNeeded(item);
  const busyPrefix = `${item.id}:`;
  return (
    <div className={`agent-row ${isManager ? "manager" : ""} ${incomplete ? "incomplete" : ""}`.trim()}>
      <div className="agent-avatar" aria-hidden="true">
        <AgentIcon />
      </div>
      <div className="agent-row-main">
        <div className="agent-row-top">
          <span className="agent-name truncate">{item.name}</span>
          <span className={`agent-status ${running ? "running" : ""}`}>{item.status || "unknown"}</span>
        </div>
        <div className="agent-meta truncate">
          {formatProviderLabel(item.provider || item.agent_profile?.provider)} · {agentModelID(item)}
        </div>
        <div className="agent-badges">
          <span className={`agent-badge ${incomplete ? "warn" : ""}`}>
            {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
          </span>
          {restartNeeded ? <span className="agent-badge warn">{t("profileRestartRequired")}</span> : null}
        </div>
      </div>
      <div className="agent-actions">
        <Button
          className="agent-icon-button"
          aria-label={t("editProfile")}
          title={t("editProfile")}
          onClick={() => onEdit(item)}
        >
          <span aria-hidden="true">
            <WrenchIcon />
          </span>
        </Button>
        {SHOW_AGENT_LIFECYCLE_ACTIONS ? (
          <>
            <Button
              className="agent-icon-button"
              aria-label={running ? t("agentStop") : t("agentStart")}
              title={running ? t("agentStop") : t("agentStart")}
              disabled={busyKey.startsWith(busyPrefix) || incomplete}
              onClick={() => (running ? onStop(item) : onStart(item))}
            >
              <span aria-hidden="true">{running ? <StopIcon /> : <PlayIcon />}</span>
            </Button>
          </>
        ) : null}
        {!isNotification ? (
          <Button
            variant="outlineDanger"
            className="agent-action-text danger"
            disabled={busyKey.startsWith(busyPrefix) || incomplete}
            onClick={() => onRecreate(item)}
          >
            {t("agentRecreate")}
          </Button>
        ) : null}
        {SHOW_AGENT_LIFECYCLE_ACTIONS && activeRoom && !isManager ? (
          <Button
            className="agent-action-text"
            disabled={busyKey.startsWith(busyPrefix)}
            onClick={() => onInvite(item)}
          >
            {t("inviteToRoom")}
          </Button>
        ) : null}
        {!isManager ? (
          <Button
            variant="outlineDanger"
            className="agent-icon-button danger"
            aria-label={t("agentDelete")}
            title={t("agentDelete")}
            disabled={busyKey.startsWith(busyPrefix)}
            onClick={() => onDelete(item)}
          >
            <span aria-hidden="true">
              <TrashIcon />
            </span>
          </Button>
        ) : null}
      </div>
    </div>
  );
}
