import type { Dispatch, KeyboardEvent as ReactKeyboardEvent, RefObject, SetStateAction } from "react";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";
import type { AgentDetailSidePanelProps } from "@/hooks/workspace/types";
import type { AgentLike, AgentProfileLike } from "@/models/agents";
import type { AttachmentDraft } from "@/models/attachments";
import type { ComposerMentionUser, ComposerSegment } from "@/models/composer";
import type { ConnectorConfigDraft, ConnectorStatus, GitLabConnectorConfigDraft } from "@/models/connectors";
import type {
  IMConversation,
  IMMessage,
  IMUser,
  LocaleCode,
  ParticipantWorkStage,
  ThreadView,
  TranslateFn,
  UsersById,
} from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";
import type { ThemeMode } from "@/shared/theme/theme";
import type { MessageAction, MessageActionFeedback, MessageLike } from "@/components/business/MessageContent/types";

export type BooleanStateSetter = Dispatch<SetStateAction<boolean>>;
export type MentionPickerUser = ComposerMentionUser & Pick<IMUser, "avatar" | "role">;
export type VoidOrPromise = void | Promise<void>;

export const ConversationWorkingActions = {
  editing: "editing",
  generatingReply: "generating_reply",
  preparingReply: "preparing_reply",
  reading: "reading",
  replying: "replying",
  running: "running",
  searching: "searching",
  thinking: "thinking",
  usingTool: "using_tool",
  waiting: "waiting",
} as const;

export type ComposerSendStatus = "failed" | "idle" | "sending";

export type ConversationWorkingAction = (typeof ConversationWorkingActions)[keyof typeof ConversationWorkingActions];

export type ConversationWorkingParticipant = {
  activity?: {
    action: ConversationWorkingAction;
    entryID?: string;
    summary?: string;
    toolName?: string;
    updatedAt?: string;
  };
  activityAfter?: string;
  canStop?: boolean;
  id: string;
  leaseID?: string;
  name: string;
  participantID?: string;
  requestID?: string;
  roomID?: string;
  stopError?: string;
  stopSending?: boolean;
  stopping?: boolean;
  thinkingText?: string;
  thinkingTruncated?: boolean;
  workStage?: ParticipantWorkStage;
};

export type ConversationPaneProps = {
  activeThreadRootID?: string;
  activeThreadView?: ThreadView | null;
  agents?: AgentLike[];
  authBusyProvider: string;
  authStatuses: CLIProxyAuthStatusMap;
  channelToolsRef: RefObject<HTMLDivElement | null>;
  composerError: string;
  connectorBusyAction?: string;
  connectorBusyProvider?: string;
  connectorError?: string;
  connectorPending?: boolean;
  connectorStatus?: ConnectorStatus;
  gitlabConnectorStatus?: ConnectorStatus;
  conversation: IMConversation;
  conversationMembers: IMUser[];
  currentUserID?: string;
  draftSegments: ComposerSegment[];
  draftText: string;
  attachmentDrafts?: AttachmentDraft[];
  removedAttachmentCount?: number;
  removedAttachmentName?: string;
  sendError?: string;
  sendProgress?: number;
  sendStatus?: ComposerSendStatus;
  editorRef: RefObject<HTMLDivElement | null>;
  inviteActionLabel: string;
  locale: LocaleCode;
  logAgent?: AgentLike | null;
  managerProfile?: AgentProfileLike | null;
  managerProfileIncomplete?: boolean | null;
  managerRuntimeUnavailable?: boolean | null;
  memberMenuRef: RefObject<HTMLDivElement | null>;
  mentionCandidates: MentionPickerUser[];
  mentionIndex: number;
  mentionableUsersByName: Map<string, ComposerMentionUser>;
  messageActionBusy: string;
  messageActionFeedback: MessageActionFeedback;
  messageListRef: RefObject<HTMLElement | null>;
  notifyAllAgentsBusy?: boolean;
  notifyAllAgentsError?: string;
  memberActionBusyID?: string;
  memberActionError?: string;
  onApplyMention: (user: MentionPickerUser) => void;
  onApplySlashCandidate?: (name: string) => void;
  onApplyThreadSlashCandidate?: (name: string) => void;
  onAddAttachments?: (files: File[]) => void;
  onAddThreadAttachments?: (files: File[]) => void;
  onClearRoomMessages?: (id: string) => VoidOrPromise;
  onClearMemberActionError?: () => void;
  onCloseThread: () => void;
  onComposerCompositionEnd: () => void;
  onComposerCompositionStart: () => void;
  onComposerKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void;
  onConnectConnector?: () => VoidOrPromise;
  onDisconnectConnector?: () => VoidOrPromise;
  onDisconnectGitLabConnector?: () => VoidOrPromise;
  onManageConnector?: () => VoidOrPromise;
  onDeleteRoom: (id: string) => VoidOrPromise;
  onDismissThreadSlashPicker?: () => void;
  onInviteAction: () => void;
  onMessageAction: (action: MessageAction, message?: MessageLike | null) => VoidOrPromise;
  onNotifyAllAgentsChange?: (enabled: boolean) => VoidOrPromise;
  onOpenAgentDetail?: (agent: AgentLike, anchor: HTMLElement) => VoidOrPromise;
  onOpenThread: (message: IMMessage) => VoidOrPromise;
  onRemoveMember?: (memberID: string) => VoidOrPromise;
  onRemoveAttachment?: (id: string) => void;
  onRemoveThreadAttachment?: (id: string) => void;
  onCancelProfilePreviewClose?: () => void;
  onCloseProfilePreview?: () => void;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onShowProfilePreview?: (user: IMUser, anchor: HTMLElement) => void;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onRetrySend?: () => VoidOrPromise;
  onSaveConnectorConfig?: (draft: ConnectorConfigDraft) => VoidOrPromise;
  onSaveGitLabConnectorConfig?: (draft: GitLabConnectorConfigDraft) => VoidOrPromise;
  onSendMessage: () => VoidOrPromise;
  onStopSend?: () => void;
  onUndoRemoveAttachment?: () => void;
  onStopWorkingTurn?: (participant: ConversationWorkingParticipant) => VoidOrPromise;
  onSendThreadReply: () => VoidOrPromise;
  onSetThreadSlashIndex?: (index: number) => void;
  onSyncComposer: () => void;
  onThreadDraftChange: (segments: ComposerSegment[]) => void;
  onThreadSlashQueryChange?: (query: string | null) => void;
  onToggleChannelTools: BooleanStateSetter;
  onToggleMemberList: BooleanStateSetter;
  onToggleToolCalls: BooleanStateSetter;
  selectedMessageCount: number;
  showChannelTools: boolean;
  showMemberList: boolean;
  showToolCalls: boolean;
  slashCandidates?: SlashPickerCandidate[];
  slashIndex?: number;
  slashPickerLoading?: boolean;
  slashPickerOpen?: boolean;
  t: TranslateFn;
  theme: ThemeMode;
  threadDraftSegments: ComposerSegment[];
  threadAttachmentDrafts?: AttachmentDraft[];
  threadError: string;
  threadLoading: boolean;
  threadSlashCandidates?: SlashPickerCandidate[];
  threadSlashIndex?: number;
  threadSlashPickerLoading?: boolean;
  threadSlashPickerOpen?: boolean;
  usersById: UsersById;
  visibleMessages: IMMessage[];
  workingParticipants?: ConversationWorkingParticipant[];
  agentDetailPanelProps?: AgentDetailSidePanelProps | null;
};
