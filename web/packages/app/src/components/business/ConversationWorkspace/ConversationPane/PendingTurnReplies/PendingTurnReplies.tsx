import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { WorkingTurnStatus } from "@/components/business/ConversationPane/WorkingTurnStatus";
import type { ConversationWorkingParticipant, VoidOrPromise } from "@/components/business/ConversationPane";
import { resolveMessageAgent } from "@/components/business/ConversationPane/MessageTimeline";
import { resolveAgentAvatarFallback, type AgentLike } from "@/models/agents";
import { resolveUserByLocalIdentity, type TranslateFn, type UsersById } from "@/models/conversations";
import { avatarFallbackText } from "@/shared/avatar";
import styles from "./PendingTurnReplies.module.css";

// A work lease can arrive before its transcript message. Keep that interval
// in the message timeline, then let the matching progress message replace it.
export function PendingTurnReplies({
  participants,
  agents,
  usersById,
  t,
  onAction,
  onStop,
}: {
  participants: readonly ConversationWorkingParticipant[];
  agents: AgentLike[];
  usersById: UsersById;
  t: TranslateFn;
  onAction?: (participant?: ConversationWorkingParticipant) => void;
  onStop?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
}) {
  return participants.map((participant) => {
    const user = resolveUserByLocalIdentity(participant.id, usersById) ?? {
      id: participant.id,
      name: participant.name,
    };
    const agent = resolveMessageAgent(agents, user, participant.id);
    const avatar = user?.avatar || agent?.avatar;
    const fallback = agent
      ? resolveAgentAvatarFallback(agent, usersById)
      : avatarFallbackText(avatar, participant.name, participant.id);
    return (
      <div
        className="message-row"
        data-work-lease-id={participant.leaseID || participant.id}
        key={participant.leaseID || participant.id}
      >
        <span className={`avatar ${styles.avatar}`} aria-hidden="true">
          <AgentAvatarContent avatar={avatar} fallback={fallback} />
          <span className="message-avatar-status online" />
        </span>
        <div className="message-card">
          <div className="message-meta">
            <span className="message-author">{participant.name}</span>
          </div>
          <div className={styles.status} role="status" aria-live="polite">
            <WorkingTurnStatus
              participant={participant}
              showName={false}
              hideContextUsage
              t={t}
              onAction={onAction}
              onStop={onStop}
            />
          </div>
        </div>
      </div>
    );
  });
}
