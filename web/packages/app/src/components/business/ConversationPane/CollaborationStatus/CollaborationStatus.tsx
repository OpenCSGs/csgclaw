import { useMemo, useSyncExternalStore } from "react";
import { CircleAlert, Square } from "lucide-react";
import { Button, IconButton, Tooltip } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { imEventsConnected, subscribeIMConnection } from "@/shared/realtime/imEvents";
import { avatarFallbackText } from "@/shared/avatar";
import { resolveAgentAvatarFallback, type AgentLike } from "@/models/agents";
import {
  resolveUserByLocalIdentity,
  userDisplayName,
  type IMMessage,
  type TranslateFn,
  type UsersById,
} from "@/models/conversations";
import { collaborationWork, collaborationWorkSummary, type CollaborationWork } from "@/models/collaborationWork";
import { progressActive } from "@/models/turnProgress";
import { resolveMessageAgent } from "../MessageTimeline";
import { ContextUsageRing } from "../ContextUsageRing";
import type { ConversationWorkingParticipant, VoidOrPromise } from "../types";
import styles from "./CollaborationStatus.module.css";

type Props = {
  roomID: string;
  messages: readonly IMMessage[];
  participants?: readonly ConversationWorkingParticipant[];
  agents?: AgentLike[];
  usersById: UsersById;
  t: TranslateFn;
  onOpenActivity?: (senderID?: string, turnID?: string, requestID?: string) => void;
  onStop?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
};
const noParticipants: readonly ConversationWorkingParticipant[] = [];
const noAgents: AgentLike[] = [];

export function CollaborationStatus({
  roomID,
  messages,
  participants = noParticipants,
  agents = noAgents,
  usersById,
  t,
  onOpenActivity,
  onStop,
}: Props) {
  const work = useMemo(() => collaborationWork(roomID, messages, participants), [roomID, messages, participants]);
  if (!work.length) return null;
  const activeCount = new Set(
    work.filter((item) => !item.progress || progressActive(item.progress)).map((item) => item.senderID),
  ).size;
  return (
    <section className={styles.status} aria-label={t("collaborationActivity")}>
      {activeCount > 0 ? (
        <div className={styles.heading} role="status">
          <span className={styles.pulse} aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          {t("collaborationWorkActive", { count: activeCount })}
        </div>
      ) : null}
      <div className={styles.members}>
        {work.map((item) => (
          <WorkMember
            key={item.id}
            work={item}
            agents={agents}
            usersById={usersById}
            t={t}
            onOpenActivity={onOpenActivity}
            onStop={onStop}
          />
        ))}
      </div>
    </section>
  );
}

type MemberProps = Pick<Props, "usersById" | "t" | "onOpenActivity" | "onStop"> & {
  work: CollaborationWork;
  agents: AgentLike[];
};
function WorkMember(props: MemberProps) {
  // Completed failures do not need live connection state.
  return !props.work.progress || progressActive(props.work.progress) ? (
    <LiveMember {...props} />
  ) : (
    <Member {...props} connected />
  );
}
function LiveMember(props: MemberProps) {
  const connected = useSyncExternalStore(subscribeIMConnection, imEventsConnected);
  return <Member {...props} connected={connected} />;
}
function Member({
  work,
  agents,
  usersById,
  t,
  onOpenActivity,
  onStop,
  connected,
}: MemberProps & { connected: boolean }) {
  const { progress, participant } = work;
  const active = !progress || progressActive(progress);
  const user = resolveUserByLocalIdentity(work.senderID, usersById);
  const agent = user ? resolveMessageAgent(agents, user, work.senderID) : undefined;
  const name = user ? userDisplayName(work.senderID, usersById) : participant?.name || work.senderID;
  const avatar = user?.avatar || agent?.avatar;
  const fallback = agent
    ? resolveAgentAvatarFallback(agent, usersById)
    : avatarFallbackText(avatar, name, work.senderID);
  const summary = collaborationWorkSummary(work);
  const label = t(active && !connected ? "progressStatus_reconnecting" : summary.label);
  const completedTools =
    progress?.items.filter(
      (item) => item.tool && ["completed", "complete", "success", "succeeded"].includes(item.tool.status),
    ).length || 0;
  const detail =
    summary.detail.replace(/\s+/g, " ").trim() ||
    (completedTools ? t("collaborationWorkSteps", { count: completedTools }) : t("collaborationWorkStarting"));
  const problem = !active || Boolean(participant?.stopError);
  const content = (
    <>
      <span className={styles.avatar} aria-hidden="true">
        <AgentAvatarContent avatar={avatar} fallback={fallback} />
      </span>
      <span className={styles.copy}>
        <span className={styles.identity}>
          <strong title={name}>{name}</strong>
          <span className={styles.action}>
            {problem ? <CircleAlert size={12} aria-hidden="true" /> : null}
            <span title={label}>{label}</span>
          </span>
        </span>
        <span className={styles.detail} title={detail}>
          {detail}
        </span>
      </span>
    </>
  );
  return (
    <div
      className={styles.member}
      data-problem={problem || undefined}
      data-work-turn-id={progress?.id}
      data-work-lease-id={progress ? undefined : participant?.leaseID || participant?.id}
    >
      {onOpenActivity ? (
        <Button
          variant="ghost"
          size="sm"
          className={styles.memberButton}
          aria-label={t("conversationWorkingOpenActivity", { name, detail: label })}
          aria-description={detail}
          onClick={() => onOpenActivity(work.senderID, progress?.id, participant?.requestID)}
        >
          {content}
        </Button>
      ) : (
        <div className={styles.memberContent}>{content}</div>
      )}
      {active && participant?.contextUsage ? (
        <span className={styles.usage} aria-label={t("progressControls", { name })}>
          <ContextUsageRing usage={participant.contextUsage} t={t} />
        </span>
      ) : null}
      {active && participant?.canStop && onStop ? (
        <Tooltip content={t("conversationWorkingStopAria", { name })}>
          <IconButton
            className={styles.stop}
            variant="tertiaryGray"
            size="sm"
            icon={<Square size={10} fill="currentColor" />}
            label={t("conversationWorkingStopAria", { name })}
            disabled={participant.stopping || participant.stopSending}
            onClick={() => void onStop(participant)}
          />
        </Tooltip>
      ) : null}
    </div>
  );
}
