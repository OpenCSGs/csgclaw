import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { PlayIcon } from "@/components/business/Icons";
import { Button, IconButton, StatusDot } from "@/components/ui";
import {
  agentModelID,
  agentProfileConfig,
  canStartAgent,
  formatProviderLabel,
  isAgentIncomplete,
  isAgentRunning,
} from "@/models/agents";
import type { AgentLike } from "@/models/agents";
import { providerNameForProviderID } from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";
import { avatarFallbackText } from "@/shared/avatar";
import styles from "./ComputerAgentList.module.css";

type ComputerAgentListProps = {
  agents: AgentLike[];
  actionBusyKeys: readonly string[];
  onCreateAgent: () => void | Promise<void>;
  onSelectAgent: (agent: AgentLike) => void;
  onStartAgent: (agent: AgentLike) => void | Promise<void>;
  t: TranslateFn;
};

export function ComputerAgentList({
  agents,
  actionBusyKeys,
  onCreateAgent,
  onSelectAgent,
  onStartAgent,
  t,
}: ComputerAgentListProps) {
  return (
    <section className={`computer-agent-panel ${styles["computer-agent-panel"]}`}>
      <div className={`section-header-inline ${styles["section-header-inline"]}`}>
        <div className={`section-label ${styles["section-label"]}`}>{t("computerAgentsSection")}</div>
        <Button variant="primary" size="md" onClick={onCreateAgent}>
          {t("createAgent")}
        </Button>
      </div>
      <div className={`entity-list ${styles["entity-list"]}`}>
        {agents.length ? (
          agents.map((item) => (
            <div key={item.id} className={`entity-list-row ${styles["entity-list-row"]}`}>
              <button
                type="button"
                className={`entity-list-main-button ${styles["entity-list-main-button"]}`}
                onClick={() => onSelectAgent(item)}
              >
                <span className={`entity-list-icon ${styles["entity-list-icon"]}`}>
                  <AgentAvatarContent
                    avatar={item.avatar}
                    fallback={avatarFallbackText(item.avatar, item.name, item.id)}
                  />
                </span>
                <span className={`entity-list-main ${styles["entity-list-main"]}`}>
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
                <StatusDot online={isAgentRunning(item)} />
              </button>
              {canStartAgent(item) ? (
                <IconButton
                  icon={<PlayIcon />}
                  iconSize={16}
                  size="sm"
                  markClassName="icon-button-mark"
                  className={styles["agent-action"]}
                  label={t("agentStart")}
                  title={t("agentStart")}
                  disabled={
                    actionBusyKeys.some((actionBusyKey) => actionBusyKey.startsWith(`${item.id}:`)) ||
                    isAgentIncomplete(item)
                  }
                  onClick={() => onStartAgent(item)}
                />
              ) : null}
            </div>
          ))
        ) : (
          <div className="agent-empty px-2.5 py-2 text-[13px] text-[var(--muted)]">{t("noAgents")}</div>
        )}
      </div>
    </section>
  );
}
