import type { TaskConfiguration } from "@/models/taskConfiguration";
import { useOptionalWorkspaceControllerContext } from "@/hooks/workspace";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui";
import { ContextUsageRing, type ConversationWorkingParticipant } from "@/components/business/ConversationPane";
import { resolveAgentChannelUserID, type AgentLike } from "@/models/agents";
import { isEventMessage, isToolCallMessage, type IMConversation, type TranslateFn } from "@/models/conversations";
import styles from "./TaskConversationControls.module.css";
import { TaskConfigurationToolbar } from "@/components/business/TaskConfigurationToolbar";

export function TaskConversationControls({
  conversation,
  participants,
  agents,
  t,
  onNewTask,
}: {
  conversation: IMConversation;
  participants: ConversationWorkingParticipant[];
  agents: AgentLike[];
  t: TranslateFn;
  onNewTask?: (agentID?: string, configuration?: TaskConfiguration) => void;
}) {
  const controller = useOptionalWorkspaceControllerContext();
  const entry = controller?.taskEntry;
  const agent = conversation.is_direct
    ? agents.find((item) => conversation.members.includes(resolveAgentChannelUserID(item)))
    : undefined;
  const hasChatMessages = conversation.messages.some(
    (message) => !isEventMessage(message) && !isToolCallMessage(message),
  );
  return (
    <div className={styles.controls}>
      <TaskConfigurationToolbar
        key={conversation.id}
        t={t}
        agent={agent}
        showModel={Boolean(agent)}
        configuration={conversation.configuration}
        directoryLocked
        disabled={entry?.isWorking(conversation.id) || entry?.busyID === conversation.id}
        onChange={entry ? (value) => entry.patch(conversation.id, { configuration: value }) : undefined}
      />
      {onNewTask && hasChatMessages ? (
        <Button
          variant="tertiaryGray"
          size="sm"
          aria-label={t("taskEntryNew")}
          onClick={() => onNewTask(agent?.id || undefined, conversation.configuration)}
        >
          <Plus size={15} />
          {t("taskEntryNew")}
        </Button>
      ) : null}
      {participants
        .filter((participant) => participant.showContextUsage || participant.contextUsage)
        .map((participant) => (
          <span
            key={participant.leaseID || participant.id || participant.name}
            className={styles.usage}
            title={participant.name}
          >
            <small>{conversation.is_direct ? "" : participant.name}</small>
            <ContextUsageRing usage={participant.contextUsage} t={t} />
          </span>
        ))}
    </div>
  );
}
