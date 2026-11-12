import { useRef } from "react";
import {
  ContextUsageRing,
  ContextUsageDetails,
  type ConversationWorkingParticipant,
} from "@/components/business/ConversationPane";
import { agentRuntimeKind, isNotificationBotAgent, resolveAgentChannelUserID, type AgentLike } from "@/models/agents";
import { localIdentitiesMatch, type IMConversation, type TranslateFn } from "@/models/conversations";
import { contextUsageRatio, type ContextUsage } from "@/models/modelMetadata";
import styles from "./ConversationContextUsage.module.css";

export function ConversationContextUsage({
  conversation,
  agents,
  participants,
  t,
}: {
  conversation: IMConversation;
  agents: AgentLike[];
  participants: ConversationWorkingParticipant[];
  t: TranslateFn;
}) {
  const panel = useRef<HTMLUListElement>(null);
  const entries = agents.filter(
    (agent) =>
      !isNotificationBotAgent(agent) &&
      ["codex", "dsh"].includes(agentRuntimeKind(agent)) &&
      conversation.members.some((member) => localIdentitiesMatch(member, resolveAgentChannelUserID(agent))),
  );
  if (conversation.cloud_agent || !entries.length) return null;
  const usageFor = (agent: AgentLike): ContextUsage | undefined => {
    const id = resolveAgentChannelUserID(agent);
    const active = participants.find((p) => !p.threadRootID && localIdentitiesMatch(p.id, id))?.contextUsage;
    if (active) return active;
    const saved = Object.entries(conversation.context_usage ?? {}).find(([user]) =>
      localIdentitiesMatch(user, id),
    )?.[1];
    return saved ? { ...saved, compacting: false } : undefined;
  };
  if (conversation.is_direct) return <ContextUsageRing usage={usageFor(entries[0])} t={t} />;
  const highest = entries
    .map(usageFor)
    .reduce<
      ContextUsage | undefined
    >((selected, current) => ((contextUsageRatio(current) ?? -1) > (contextUsageRatio(selected) ?? -1) ? current : selected), undefined);
  const ratio = contextUsageRatio(highest);
  const label =
    ratio === null
      ? t("contextUsageGroup", { count: entries.length })
      : t("contextUsageGroupMaximum", { count: entries.length, percent: Math.round(ratio * 100) });
  return (
    <ContextUsageRing
      usage={highest}
      t={t}
      label={label}
      onKeyDown={(event) => {
        const direction =
          event.key === "ArrowDown" || event.key === "PageDown"
            ? 1
            : event.key === "ArrowUp" || event.key === "PageUp"
              ? -1
              : 0;
        if (direction) {
          event.preventDefault();
          panel.current?.scrollBy({ top: direction * (event.key.startsWith("Page") ? 240 : 48) });
        }
      }}
      content={
        <ul ref={panel} className={styles.panel} aria-label={t("contextUsageGroup", { count: entries.length })}>
          {entries.map((agent) => (
            <li key={agent.id}>
              <strong className={styles.name}>{agent.name || agent.id}</strong>
              <ContextUsageDetails usage={usageFor(agent)} t={t} compact />
            </li>
          ))}
        </ul>
      }
    />
  );
}
