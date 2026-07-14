import type { Dispatch, KeyboardEvent as ReactKeyboardEvent, RefObject, SetStateAction } from "react";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";
import type { AgentDetailSidePanelProps } from "@/hooks/workspace/types";
import type { AgentLike, AgentProfileLike } from "@/models/agents";
import type { AttachmentDraft } from "@/models/attachments";
import type { ComposerMentionUser, ComposerSegment } from "@/models/composer";
import type { ConnectorConfigDraft, ConnectorStatus } from "@/models/connectors";
import type {
  IMConversation,
  IMMessage,
  IMUser,
  LocaleCode,
  ThreadView,
  TranslateFn,
  UsersById,
} from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";
import type { ThemeMode } from "@/shared/theme/theme";
import type { MessageAction, MessageActionError, MessageLike } from "@/components/business/MessageContent/types";

export type BooleanStateSetter = Dispatch<SetStateAction<boolean>>;
export type MentionPickerUser = ComposerMentionUser & Pick<IMUser, "avatar" | "role">;
export type VoidOrPromise = void | Promise<void>;

export type ConversationWorkingParticipant = {
  id: string;
  name: string;
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
  connectorError?: string;
  connectorPending?: boolean;
  connectorStatus?: ConnectorStatus;
  conversation: IMConversation;
  conversationMembers: IMUser[];
  currentUserID?: string;
  draftSegments: ComposerSegment[];
  draftText: string;
  attachmentDrafts?: AttachmentDraft[];
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
  messageActionError: MessageActionError;
  messageListRef: RefObject<HTMLElement | null>;
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
  onManageConnector?: () => VoidOrPromise;
  onDeleteRoom: (id: string) => VoidOrPromise;
  onDismissThreadSlashPicker?: () => void;
  onInviteAction: () => void;
  onMessageAction: (action: MessageAction, message?: MessageLike | null) => VoidOrPromise;
  onOpenAgentDetail?: (agent: AgentLike, anchor: HTMLElement) => VoidOrPromise;
  onOpenThread: (message: IMMessage) => VoidOrPromise;
  onRemoveMember?: (memberID: string) => VoidOrPromise;
  onRemoveAttachment?: (id: string) => void;
  onRemoveThreadAttachment?: (id: string) => void;
  onCancelProfilePreviewClose?: () => void;
  onCloseProfilePreview?: () => void;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onSaveConnectorConfig?: (draft: ConnectorConfigDraft) => VoidOrPromise;
  onSendMessage: () => VoidOrPromise;
  onSendThreadReply: () => VoidOrPromise;
  onSetThreadSlashIndex?: (index: number) => void;
  onSyncComposer: () => void;
  onThreadDraftChange: (segments: ComposerSegment[]) => void;
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
