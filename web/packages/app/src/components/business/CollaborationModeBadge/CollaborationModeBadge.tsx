import { ListTodo, MessagesSquare } from "lucide-react";
import {
  RoomTypes,
  isDirectConversation,
  roomType,
  type IMConversation,
  type TranslateFn,
} from "@/models/conversations";
import styles from "./CollaborationModeBadge.module.css";

const modes = {
  [RoomTypes.autonomous]: {
    label: "autonomousCollaboration",
    shortLabel: "autonomousCollaborationTag",
    icon: MessagesSquare,
  },
  [RoomTypes.onDemand]: {
    label: "onDemandCollaboration",
    shortLabel: "onDemandCollaborationTag",
    icon: ListTodo,
  },
} as const;

export function CollaborationModeBadge({
  conversation,
  compact = false,
  t,
}: {
  conversation: IMConversation;
  compact?: boolean;
  t: TranslateFn;
}) {
  const type = roomType(conversation);
  if (isDirectConversation(conversation) || conversation.cloud_agent || type === RoomTypes.free) return null;

  const { label, shortLabel, icon: Icon } = modes[type];
  return (
    <span className={styles.badge} data-mode={type} data-compact={compact || undefined} title={t(label)}>
      {!compact ? <Icon size={12} aria-hidden="true" /> : null}
      <span>{t(compact ? shortLabel : label)}</span>
    </span>
  );
}
