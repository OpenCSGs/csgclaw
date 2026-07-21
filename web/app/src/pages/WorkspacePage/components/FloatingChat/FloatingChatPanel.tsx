import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { fetchAgentLogsRequest } from "@/api/agents";
import { errorMessage } from "@/api/client";
import {
  Conversation,
  AgentQuestionComposer,
  type ConversationMessageListProps,
  type ConversationPaneProps,
  useQuestionAnswerMode,
  useConversationDraftEditorSync,
} from "@/components/business/ConversationPane";
import { DialogContent, DialogRoot } from "@/components/ui";
import { normalizeAuthProviderName } from "@/models/agents";
import {
  getConversationDescription,
  isDirectConversation,
  type IMMessage,
  type TranslateFn,
} from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import { FloatingChatPromptSuggestions } from "./FloatingChatPromptSuggestions";
import styles from "./FloatingChat.module.css";

export type FloatingChatPanelProps = {
  agentName: string;
  chatProps: ConversationPaneProps;
  headerAccessory?: ReactNode;
  onPickPrompt: (text: string) => void;
};

export function FloatingChatPanel({ agentName, chatProps, headerAccessory, onPickPrompt }: FloatingChatPanelProps) {
  const {
    activeThreadRootID,
    activeThreadView,
    agents = [],
    authBusyProvider,
    authStatuses,
    channelToolsRef,
    composerError,
    connectorBusyAction,
    connectorBusyProvider,
    connectorError,
    connectorPending,
    connectorStatus,
    conversation,
    conversationMembers,
    currentUserID = "",
    draftSegments,
    draftText,
    attachmentDrafts,
    editorRef,
    inviteActionLabel,
    locale,
    logAgent,
    gitlabConnectorStatus,
    managerProfile,
    managerProfileIncomplete,
    managerRuntimeUnavailable,
    memberMenuRef,
    mentionCandidates,
    mentionIndex,
    mentionableUsersByName,
    messageActionBusy,
    messageActionFeedback,
    messageListRef,
    onApplyMention,
    onApplySlashCandidate = (_name) => {},
    onApplyThreadSlashCandidate = (_name) => {},
    onAddAttachments,
    onAddThreadAttachments,
    onClearRoomMessages = (_id) => {},
    onCloseThread,
    onComposerCompositionEnd,
    onComposerCompositionStart,
    onComposerKeyDown,
    onConnectConnector,
    onDeleteRoom,
    onDisconnectConnector,
    onDisconnectGitLabConnector,
    onDismissThreadSlashPicker = () => {},
    onInviteAction,
    onMessageAction,
    onManageConnector,
    onOpenThread,
    onPreviewUser,
    onProviderLogin,
    onRemoveAttachment,
    onRemoveThreadAttachment,
    onSendMessage,
    onSendThreadReply,
    onSaveConnectorConfig,
    onSaveGitLabConnectorConfig,
    onSetThreadSlashIndex = (_index) => {},
    onSyncComposer,
    onThreadDraftChange,
    onThreadSlashQueryChange,
    onToggleChannelTools,
    onToggleMemberList,
    onToggleToolCalls,
    selectedMessageCount,
    showChannelTools,
    showMemberList,
    showToolCalls,
    slashCandidates = [],
    slashIndex = 0,
    slashPickerLoading = false,
    slashPickerOpen = false,
    t,
    theme,
    threadDraftSegments,
    threadAttachmentDrafts,
    threadError,
    threadLoading,
    threadSlashCandidates = [],
    threadSlashIndex = 0,
    threadSlashPickerLoading = false,
    threadSlashPickerOpen = false,
    usersById,
    visibleMessages,
    workingParticipants = [],
  } = chatProps;
  const description = getConversationDescription(conversation, currentUserID, usersById, locale, t);
  const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
  const [logModalOpen, setLogModalOpen] = useState(false);
  const [logContent, setLogContent] = useState("");
  const [logError, setLogError] = useState("");
  const [logLoading, setLogLoading] = useState(false);
  const [clearMessagesDialogOpen, setClearMessagesDialogOpen] = useState(false);
  const [deleteRoomDialogOpen, setDeleteRoomDialogOpen] = useState(false);
  const logAgentID = logAgent?.id || "";
  const logAgentName = logAgent?.name || conversation.title || "";
  const composerDisabledReason = managerRuntimeUnavailable ? t("managerCodexMissingWarning") : t("profileIncomplete");
  const composerDisabled = Boolean(managerRuntimeUnavailable || managerProfileIncomplete);
  const questionMode = useQuestionAnswerMode({
    messages: conversation.messages.filter((message) => !message.relates_to),
    responderID: currentUserID,
    roomID: conversation.id,
    t,
  });
  const threadQuestionMode = useQuestionAnswerMode({
    messages: activeThreadView?.root ? [activeThreadView.root, ...(activeThreadView.replies ?? [])] : [],
    responderID: currentUserID,
    roomID: conversation.id,
    t,
  });
  const floatingConversationMessages = conversation.messages.filter((message) => !isManagerBootstrapNotice(message));
  const floatingVisibleMessages = visibleMessages.filter((message) => !isManagerBootstrapNotice(message));
  const floatingConversation =
    floatingConversationMessages.length === conversation.messages.length
      ? conversation
      : { ...conversation, messages: floatingConversationMessages };
  const floatingComposerT = useCallback<TranslateFn>(
    (key, params) => (key === "inputPlaceholder" ? t("floatingChatInputPlaceholder") : t(key, params)),
    [t],
  );

  useConversationDraftEditorSync(editorRef, draftSegments);

  useEffect(() => {
    setLogModalOpen(false);
    setLogContent("");
    setLogError("");
    setLogLoading(false);
    setClearMessagesDialogOpen(false);
    setDeleteRoomDialogOpen(false);
  }, [conversation.id, logAgentID]);

  const refreshAgentLogs = useCallback(async () => {
    if (!logAgentID) {
      return;
    }
    setLogLoading(true);
    setLogError("");
    try {
      setLogContent(await fetchAgentLogsRequest(logAgentID, { lines: 400 }));
    } catch (err) {
      setLogError(errorMessage(err, t("agentLogsLoadFailed")));
    } finally {
      setLogLoading(false);
    }
  }, [logAgentID, t]);

  const handleOpenAgentLogs = useCallback(() => {
    setLogModalOpen(true);
    void refreshAgentLogs();
  }, [refreshAgentLogs]);

  const handleOpenClearMessagesDialog = useCallback(() => {
    onToggleChannelTools(false);
    setClearMessagesDialogOpen(true);
  }, [onToggleChannelTools]);

  const handleOpenDeleteRoomDialog = useCallback(() => {
    onToggleChannelTools(false);
    setDeleteRoomDialogOpen(true);
  }, [onToggleChannelTools]);

  const threadPanel = activeThreadRootID ? (
    <Conversation.ThreadPanel
      thread={activeThreadView}
      loading={threadLoading}
      error={threadError}
      draftSegments={threadDraftSegments}
      attachmentDrafts={threadAttachmentDrafts}
      disabled={composerDisabled}
      usersById={usersById}
      locale={locale}
      theme={theme}
      showToolCalls={showToolCalls}
      t={t}
      onClose={onCloseThread}
      onDraftChange={onThreadDraftChange}
      onSlashQueryChange={onThreadSlashQueryChange}
      onAddAttachments={onAddThreadAttachments}
      onRemoveAttachment={onRemoveThreadAttachment}
      threadSlashCandidates={threadSlashCandidates}
      threadSlashIndex={threadSlashIndex}
      threadSlashPickerLoading={threadSlashPickerLoading}
      threadSlashPickerOpen={threadSlashPickerOpen}
      onApplyThreadSlashCandidate={onApplyThreadSlashCandidate}
      onDismissThreadSlashPicker={onDismissThreadSlashPicker}
      onSetThreadSlashIndex={onSetThreadSlashIndex}
      mentionableUsers={conversationMembers}
      onPreviewUser={onPreviewUser}
      onQuestionSelect={threadQuestionMode.select}
      questionMode={threadQuestionMode}
      onSend={onSendThreadReply}
    />
  ) : null;

  return (
    <div className={classNames("chat-panel", styles.conversation)}>
      <Conversation.Header
        channelToolsRef={channelToolsRef}
        conversation={conversation}
        conversationMembers={conversationMembers}
        description={description}
        headerAccessory={headerAccessory}
        inviteActionLabel={inviteActionLabel}
        logAgent={logAgent}
        logModalOpen={logModalOpen}
        memberMenuRef={memberMenuRef}
        selectedMessageCount={selectedMessageCount}
        selectedVisibleMessageCount={floatingVisibleMessages.length}
        showChannelTools={showChannelTools}
        showInviteAction={false}
        showMemberList={showMemberList}
        showToolCalls={showToolCalls}
        t={t}
        onClearMessages={handleOpenClearMessagesDialog}
        onDeleteRoom={handleOpenDeleteRoomDialog}
        onInviteAction={onInviteAction}
        onOpenAgentLogs={handleOpenAgentLogs}
        onPreviewUser={onPreviewUser}
        onToggleChannelTools={onToggleChannelTools}
        onToggleMemberList={onToggleMemberList}
        onToggleToolCalls={onToggleToolCalls}
      />
      <FloatingChatMessageArea
        agents={agents}
        agentName={agentName}
        conversation={floatingConversation}
        currentUserID={currentUserID}
        locale={locale}
        messageActionBusy={messageActionBusy}
        messageActionFeedback={messageActionFeedback}
        messageListRef={messageListRef}
        t={t}
        theme={theme}
        usersById={usersById}
        visibleMessages={floatingVisibleMessages}
        onMessageAction={onMessageAction}
        onOpenThread={onOpenThread}
        onPickPrompt={onPickPrompt}
        onPreviewUser={onPreviewUser}
        onQuestionSelect={questionMode.select}
      />
      {questionMode.pending.length > 0 ? (
        <AgentQuestionComposer mode={questionMode} t={floatingComposerT} usersById={usersById} />
      ) : (
        <Conversation.Composer
          authBusyProvider={authBusyProvider}
          authStatuses={authStatuses}
          connectorStatus={connectorStatus}
          gitlabConnectorStatus={gitlabConnectorStatus}
          connectorBusyAction={connectorBusyAction}
          connectorBusyProvider={connectorBusyProvider}
          connectorError={connectorError}
          connectorPending={connectorPending}
          composerDisabled={composerDisabled}
          composerDisabledReason={composerDisabledReason}
          composerError={composerError}
          draftSegments={draftSegments}
          draftText={draftText}
          attachmentDrafts={attachmentDrafts}
          editorRef={editorRef}
          managerProfile={managerProfile}
          managerProvider={managerProvider}
          mentionCandidates={mentionCandidates}
          mentionIndex={mentionIndex}
          mentionableUsersByName={mentionableUsersByName}
          slashCandidates={slashCandidates}
          slashIndex={slashIndex}
          slashPickerLoading={slashPickerLoading}
          slashPickerOpen={slashPickerOpen}
          t={floatingComposerT}
          workingParticipants={workingParticipants}
          onApplyMention={onApplyMention}
          onApplySlashCandidate={onApplySlashCandidate}
          onAddAttachments={onAddAttachments}
          onComposerCompositionEnd={onComposerCompositionEnd}
          onComposerCompositionStart={onComposerCompositionStart}
          onComposerKeyDown={onComposerKeyDown}
          onConnectConnector={onConnectConnector}
          onDisconnectConnector={onDisconnectConnector}
          onDisconnectGitLabConnector={onDisconnectGitLabConnector}
          onManageConnector={onManageConnector}
          onProviderLogin={onProviderLogin}
          onSaveConnectorConfig={onSaveConnectorConfig}
          onSaveGitLabConnectorConfig={onSaveGitLabConnectorConfig}
          onSendMessage={onSendMessage}
          onRemoveAttachment={onRemoveAttachment}
          onSyncComposer={onSyncComposer}
        />
      )}
      <DialogRoot
        open={Boolean(activeThreadRootID)}
        onOpenChange={(open) => {
          if (!open) {
            onCloseThread();
          }
        }}
      >
        {threadPanel ? (
          <DialogContent className="thread-dialog-content" overlayClassName="thread-dialog-backdrop">
            {threadPanel}
          </DialogContent>
        ) : null}
      </DialogRoot>
      <Conversation.RoomDangerConfirmDialog
        cancelLabel={t("cancel")}
        closeLabel={t("close")}
        confirmLabel={t("clearRoomMessagesConfirm")}
        description={t("clearRoomMessagesAgentScopeHint")}
        open={clearMessagesDialogOpen}
        title={t("clearRoomMessages")}
        onConfirm={() => {
          setClearMessagesDialogOpen(false);
          onClearRoomMessages(conversation.id);
        }}
        onOpenChange={setClearMessagesDialogOpen}
      />
      {!isDirectConversation(conversation) ? (
        <Conversation.RoomDangerConfirmDialog
          cancelLabel={t("cancel")}
          closeLabel={t("close")}
          confirmLabel={t("deleteRoomConfirm")}
          description={t("deleteRoomConfirmBody")}
          open={deleteRoomDialogOpen}
          title={t("deleteRoom")}
          onConfirm={() => {
            setDeleteRoomDialogOpen(false);
            onDeleteRoom(conversation.id);
          }}
          onOpenChange={setDeleteRoomDialogOpen}
        />
      ) : null}
      {logModalOpen && logAgent ? (
        <Conversation.AgentLogsDialog
          agentName={logAgentName}
          content={logContent}
          error={logError}
          loading={logLoading}
          t={t}
          onClose={() => setLogModalOpen(false)}
          onRefresh={refreshAgentLogs}
        />
      ) : null}
    </div>
  );
}

type FloatingChatMessageAreaProps = Omit<ConversationMessageListProps, "emptyStateSlot"> & {
  agentName: string;
  onPickPrompt: (text: string) => void;
};

function FloatingChatMessageArea({
  agentName,
  messageListRef,
  onPickPrompt,
  t,
  ...messageListProps
}: FloatingChatMessageAreaProps) {
  if (messageListProps.conversation.messages.length === 0) {
    return (
      <section ref={messageListRef} className="messages">
        <FloatingChatPromptSuggestions agentName={agentName} t={t} onPickPrompt={onPickPrompt} />
      </section>
    );
  }

  return <Conversation.MessageList {...messageListProps} messageListRef={messageListRef} t={t} />;
}

function isManagerBootstrapNotice(message: IMMessage): boolean {
  if (message.event) {
    return false;
  }
  const content = message.content.trim();
  return (
    content === "Bootstrap room created for admin and manager." ||
    content === "Bootstrap room created for Admin and Manager."
  );
}
