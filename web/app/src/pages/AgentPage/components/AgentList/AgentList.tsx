import { SHOW_AGENT_LIFECYCLE_ACTIONS } from "@/shared/constants/agents";
import {
  agentProfileConfig,
  agentRuntimeState,
  agentStatusLabel,
  agentModelID,
  formatProviderLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentUpgradeNeeded,
  isAgentRunning,
  isNotificationBotAgent,
} from "@/models/agents";
import { AgentIcon, PlayIcon, StopIcon, TrashIcon, WrenchIcon } from "@/components/ui/Icons";
import { Button, Tooltip } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { providerNameForProviderID } from "@/models/modelProviders";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, TranslateFn } from "@/models/conversations";

type VoidOrPromise = void | Promise<void>;
type AgentActionHandler = (item: AgentLike) => VoidOrPromise;

export type AgentSectionProps = {
  activeRoom?: IMConversation | null;
  busyKey?: string;
  error?: string;
  manager?: AgentLike | null;
  onCreate: () => VoidOrPromise;
  onDelete: AgentActionHandler;
  onEdit: AgentActionHandler;
  onInvite: AgentActionHandler;
  onRecreate: AgentActionHandler;
  onStart: AgentActionHandler;
  onStop: AgentActionHandler;
  onUpgrade?: AgentActionHandler;
  t: TranslateFn;
  title: string;
  workers?: AgentLike[];
};

export function AgentSection({
  title,
  manager,
  workers = [],
  t,
  activeRoom = null,
  busyKey = "",
  error = "",
  onCreate,
  onEdit,
  onStart,
  onStop,
  onRecreate,
  onUpgrade,
  onDelete,
  onInvite,
}: AgentSectionProps) {
  const items = [manager, ...workers].filter((item): item is AgentLike => Boolean(item));
  return (
    <section className="agent-section">
      <div className="agent-section-head">
        <div>
          <div className="section-label">
            {title} {items.length}
          </div>
        </div>
        <Tooltip content={t("createAgent")}>
          <Button className="agent-add-button" aria-label={t("createAgent")} onClick={onCreate}>
            <span aria-hidden="true">
              <AgentIcon />
            </span>
          </Button>
        </Tooltip>
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
              onUpgrade={onUpgrade}
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

export type AgentRowProps = {
  activeRoom?: IMConversation | null;
  busyKey?: string;
  item: AgentLike;
  onDelete: AgentActionHandler;
  onEdit: AgentActionHandler;
  onInvite: AgentActionHandler;
  onRecreate: AgentActionHandler;
  onStart: AgentActionHandler;
  onStop: AgentActionHandler;
  onUpgrade?: AgentActionHandler;
  t: TranslateFn;
};

export function AgentRow({
  item,
  t,
  activeRoom = null,
  busyKey = "",
  onEdit,
  onStart,
  onStop,
  onRecreate,
  onUpgrade,
  onDelete,
  onInvite,
}: AgentRowProps) {
  const isManager = item.role === "manager" || item.id === "u-manager";
  const isNotification = isNotificationBotAgent(item);
  const running = isAgentRunning(item);
  const incomplete = isAgentIncomplete(item);
  const restartNeeded = isAgentRestartNeeded(item);
  const upgradeNeeded = isAgentUpgradeNeeded(item);
  const profile = agentProfileConfig(item);
  const provider = item.provider || profile?.provider || providerNameForProviderID(profile?.model_provider_id || "");
  const busyPrefix = `${item.id}:`;
  return (
    <div className={`agent-row ${isManager ? "manager" : ""} ${incomplete ? "incomplete" : ""}`.trim()}>
      <div className="agent-avatar" aria-hidden="true">
        <AgentAvatarContent avatar={item.avatar} fallback={avatarFallbackText(item.avatar, item.name, item.id)} />
      </div>
      <div className="agent-row-main">
        <div className="agent-row-top">
          <span className="agent-name truncate">{item.name}</span>
          <span className={`agent-status ${running ? "running" : ""}`}>
            {agentStatusLabel(agentRuntimeState(item), t)}
          </span>
        </div>
        <div className="agent-meta truncate">
          {formatProviderLabel(provider)} · {agentModelID(item)}
        </div>
        <div className="agent-badges">
          <span className={`agent-badge ${incomplete ? "warn" : "ready"}`}>
            {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
          </span>
          {upgradeNeeded ? <span className="agent-badge warn">{t("profileUpgradeRequired")}</span> : null}
          {restartNeeded ? <span className="agent-badge warn">{t("profileRestartRequired")}</span> : null}
        </div>
      </div>
      <div className="agent-actions">
        <Tooltip content={t("editProfile")}>
          <Button className="agent-icon-button" aria-label={t("editProfile")} onClick={() => onEdit(item)}>
            <span aria-hidden="true">
              <WrenchIcon />
            </span>
          </Button>
        </Tooltip>
        {SHOW_AGENT_LIFECYCLE_ACTIONS ? (
          <>
            <Tooltip content={running ? t("agentStop") : t("agentStart")}>
              <span>
                <Button
                  className="agent-icon-button"
                  aria-label={running ? t("agentStop") : t("agentStart")}
                  disabled={busyKey.startsWith(busyPrefix) || incomplete}
                  onClick={() => (running ? onStop(item) : onStart(item))}
                >
                  <span aria-hidden="true">{running ? <StopIcon /> : <PlayIcon />}</span>
                </Button>
              </span>
            </Tooltip>
          </>
        ) : null}
        {!isNotification ? (
          <>
            {onUpgrade && upgradeNeeded ? (
              <Button
                className="agent-action-text"
                disabled={busyKey.startsWith(busyPrefix) || incomplete}
                onClick={() => onUpgrade(item)}
              >
                {t("agentUpgrade")}
              </Button>
            ) : null}
            <Button
              variant="outlineDanger"
              className="agent-action-text danger"
              disabled={busyKey.startsWith(busyPrefix) || incomplete}
              onClick={() => onRecreate(item)}
            >
              {t("agentRecreate")}
            </Button>
          </>
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
          <Tooltip content={t("agentDelete")}>
            <span>
              <Button
                variant="outlineDanger"
                className="agent-icon-button danger"
                aria-label={t("agentDelete")}
                disabled={busyKey.startsWith(busyPrefix)}
                onClick={() => onDelete(item)}
              >
                <span aria-hidden="true">
                  <TrashIcon />
                </span>
              </Button>
            </span>
          </Tooltip>
        ) : null}
      </div>
    </div>
  );
}
