import { useEffect, useState } from "react";
import { CircleGauge, LoaderCircle, X } from "lucide-react";
import { Button, Tooltip, PopoverRoot, PopoverTrigger, PopoverContent, PopoverClose } from "@/components/ui";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane";
import { compactRoomContext } from "@/api/contextCompaction";
import { localizeAPIError } from "@/shared/i18n";
import { agentRuntimeKind, isNotificationBotAgent, resolveAgentChannelUserID, type AgentLike } from "@/models/agents";
import { localIdentitiesMatch, type IMConversation, type TranslateFn } from "@/models/conversations";
import { contextUsageRatio, formatContextAmount, latestContextUsage, type ContextUsage } from "@/models/modelMetadata";
import {
  conversationCompacting,
  manualCompactionEligible,
  messageCompaction,
  type CompactionOperation,
} from "@/models/contextCompaction";
import styles from "./ConversationContextUsage.module.css";
import { createUUID } from "@/shared/lib/uuid";
import { useHoverContextPanel } from "./useHoverContextPanel";
import { ContextCompactIcon } from "./ContextCompactIcon";

export function ConversationContextUsage({
  conversation,
  agents,
  participants,
  t,
  onCompactingChange,
}: {
  conversation: IMConversation;
  agents: AgentLike[];
  participants: ConversationWorkingParticipant[];
  t: TranslateFn;
  onCompactingChange?: (busy: boolean) => void;
}) {
  const hoverPanel = useHoverContextPanel();
  const [sending, setSending] = useState(false);
  const [pending, setPending] = useState<Pick<CompactionOperation, "id" | "epoch">>();
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const entries = agents.filter(
    (agent) =>
      Boolean(agent.id) &&
      !isNotificationBotAgent(agent) &&
      ["codex", "dsh"].includes(agentRuntimeKind(agent)) &&
      conversation.members.some((member) => localIdentitiesMatch(member, resolveAgentChannelUserID(agent))),
  );
  const usageFor = (agent: AgentLike): ContextUsage | undefined => {
    const id = resolveAgentChannelUserID(agent);
    const active = participants.find(
      (p) =>
        !p.threadRootID &&
        localIdentitiesMatch(p.id, id) &&
        (p.contextUsageEpoch ?? 0) === (conversation.context_usage_epoch ?? 0),
    )?.contextUsage;
    const saved = Object.entries(conversation.context_usage ?? {}).find(([user]) =>
      localIdentitiesMatch(user, id),
    )?.[1];
    return latestContextUsage(active, saved);
  };
  const running =
    conversationCompacting(conversation) ||
    Boolean(
      pending &&
      pending.epoch === (conversation.context_usage_epoch ?? 0) &&
      !conversation.messages.some((message) => {
        const operation = messageCompaction(message);
        return operation?.id === pending.id && operation.epoch === pending.epoch;
      }),
    );
  const ratio = Math.max(-1, ...entries.map((agent) => contextUsageRatio(usageFor(agent)) ?? -1));
  const percent = Math.round(ratio * 100);
  const label =
    ratio < 0
      ? t("contextCompactContext")
      : t(conversation.is_direct ? "contextUsagePercent" : "contextCompactGroupPercent", { percent });
  useEffect(() => {
    onCompactingChange?.(Boolean(running || sending));
  }, [running, sending, onCompactingChange]);
  useEffect(() => {
    setPending(undefined);
    setNotice("");
    setError("");
  }, [conversation.context_usage_epoch]);
  if (conversation.cloud_agent || !entries.length) return null;
  const submit = async (targets: AgentLike[]) => {
    setError("");
    setNotice("");
    if (targets.length === 1 && !manualCompactionEligible(usageFor(targets[0]))) {
      setNotice(
        t(
          usageFor(targets[0])?.used_tokens == null
            ? "contextCompactStatus_unknown"
            : "contextCompactStatus_unnecessary",
        ),
      );
      return;
    }
    setSending(true);
    try {
      const operation = await compactRoomContext(
        conversation.id,
        targets.map((agent) => agent.id!),
        createUUID(),
      );
      if (operation.running) setPending({ id: operation.id, epoch: operation.epoch });
    } catch (cause) {
      setError(localizeAPIError(cause, t));
    } finally {
      setSending(false);
    }
  };
  const action = (all: boolean, disabled: boolean, busy: boolean, click: () => void) => {
    const text = t(all ? "contextCompactAllContext" : "contextCompactAction");
    return (
      <Tooltip content={text} delayDuration={0}>
        <span className={styles.actionHint}>
          <Button variant="tertiaryGray" size="sm" iconOnly aria-label={text} disabled={disabled} onClick={click}>
            {busy ? <LoaderCircle size={16} className={styles.spinner} /> : <ContextCompactIcon all={all} />}
          </Button>
        </span>
      </Tooltip>
    );
  };
  return (
    <div className={styles.root}>
      <PopoverRoot open={hoverPanel.open} onOpenChange={hoverPanel.onOpenChange}>
        <PopoverTrigger asChild>
          <Button {...hoverPanel.triggerProps} variant="tertiaryGray" size="sm" aria-label={label}>
            <CircleGauge size={16} />
            <span>
              {ratio < 0
                ? t("contextCompactContext")
                : t(conversation.is_direct ? "contextCompactPercent" : "contextCompactGroupShort", { percent })}
            </span>
          </Button>
        </PopoverTrigger>
        <PopoverContent
          {...hoverPanel.panelProps}
          side="top"
          align="end"
          className={styles.panel}
          aria-label={t("contextCompactContext")}
        >
          <div className={styles.header}>
            <strong>{t("contextCompactContext")}</strong>
            {!conversation.is_direct ? action(true, sending || running, running, () => void submit(entries)) : null}
            <PopoverClose asChild>
              <Button variant="tertiaryGray" size="sm" iconOnly aria-label={t("contextCompactClose")}>
                <X size={16} />
              </Button>
            </PopoverClose>
          </div>
          <ul className={styles.list}>
            {entries.map((agent) => {
              const usage = usageFor(agent);
              const value = contextUsageRatio(usage);
              const busy = participants.some(
                (p) =>
                  localIdentitiesMatch(p.id, resolveAgentChannelUserID(agent)) && Boolean(p.leaseID || p.requestID),
              );
              const amount = `${usage?.used_tokens == null ? "-" : formatContextAmount(usage.used_tokens)} / ${usage ? formatContextAmount(usage.context_window) : "-"}`;
              return (
                <li key={agent.id}>
                  <div className={styles.row}>
                    <div className={styles.name}>
                      <strong>{agent.name || agent.id}</strong>
                      {usage?.model_id ? <span className={styles.model}> ({usage.model_id})</span> : null}
                    </div>
                    <span className={styles.amount} aria-label={t("contextCompactAmountLabel", { amount })}>
                      {amount}
                      {value == null ? "" : ` (${Math.round(value * 100)}%)`}
                    </span>
                  </div>
                  <div className={styles.meterRow}>
                    <progress
                      className={styles.meter}
                      max={1}
                      value={value == null ? 0 : Math.min(1, value)}
                      aria-label={t("contextCompactContext")}
                    />
                    {action(
                      false,
                      sending || running || busy || Boolean(usage?.compacting),
                      Boolean(usage?.compacting),
                      () => void submit([agent]),
                    )}
                  </div>
                  {busy ? <span className={styles.muted}>{t("contextCompactStatus_busy")}</span> : null}
                </li>
              );
            })}
          </ul>
          {notice ? (
            <p role="status" className={styles.notice}>
              {notice}
            </p>
          ) : null}
          {error ? (
            <p role="alert" className={styles.error}>
              {error}
            </p>
          ) : null}
        </PopoverContent>
      </PopoverRoot>
    </div>
  );
}
