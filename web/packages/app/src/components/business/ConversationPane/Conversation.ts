import { ConversationComposer } from "./ConversationComposer";
import { ConversationAgentLogsDialog, ConversationRoomDangerConfirmDialog } from "./ConversationDialogs";
import { ConversationHeader } from "./ConversationHeader";
import { ConversationMessageList } from "./ConversationMessageList";
import { ConversationThreadPanel } from "./ConversationThreadPanel";
import { MentionPicker } from "./MentionPicker";
import { MessageTimeDivider, MessageTimestamp } from "./MessageTime";
import { SlashPicker } from "./SlashPicker";

export const Conversation = {
  AgentLogsDialog: ConversationAgentLogsDialog,
  Composer: ConversationComposer,
  Header: ConversationHeader,
  MentionPicker,
  MessageList: ConversationMessageList,
  MessageTimeDivider,
  MessageTimestamp,
  RoomDangerConfirmDialog: ConversationRoomDangerConfirmDialog,
  SlashPicker,
  ThreadPanel: ConversationThreadPanel,
} as const;
