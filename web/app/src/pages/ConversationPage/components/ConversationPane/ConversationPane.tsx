import { useCallback, useEffect, useState } from "react";
import { fetchAgentLogsRequest } from "@/api/agents";
import { errorMessage } from "@/api/client";
import {
  Conversation,
  type ConversationPaneProps,
  useConversationDraftEditorSync,
} from "@/components/business/ConversationPane";
import { normalizeAuthProviderName } from "@/models/agents";
import { getConversationDescription, isDirectConversation } from "@/models/conversations";

export function ConversationPane({
  conversation,
  visibleMessages,
  currentUserID = "",
  usersById,
  agents = [],
  locale,
  t,
  theme,
  selectedMessageCount,
  logAgent,
  conversationMembers,
  showChannelTools,
  onToggleChannelTools,
  showToolCalls,
  onToggleToolCalls,
  channelToolsRef,
  messageListRef,
  editorRef,
  onPreviewUser,
  onDeleteRoom,
  onClearRoomMessages = (_id) => {},
  inviteActionLabel,
  onInviteAction,
  mentionCandidates,
  mentionIndex,
  onApplyMention,
  slashCandidates = [],
  slashIndex = 0,
  slashPickerLoading = false,
  slashPickerOpen = false,
  onApplySlashCandidate = (_name) => {},
  threadSlashCandidates = [],
  threadSlashIndex = 0,
  threadSlashPickerLoading = false,
  threadSlashPickerOpen = false,
  onApplyThreadSlashCandidate = (_name) => {},
  onDismissThreadSlashPicker = () => {},
  onSetThreadSlashIndex = (_index) => {},
  managerProfile,
  managerProfileIncomplete,
  authStatuses,
  authBusyProvider,
  onProviderLogin,
  draftSegments,
  draftText,
  mentionableUsersByHandle,
  onSyncComposer,
  onComposerKeyDown,
  onComposerCompositionStart,
  onComposerCompositionEnd,
  onSendMessage,
  composerError,
  messageActionBusy,
  messageActionError,
  onMessageAction,
  activeThreadRootID,
  activeThreadView,
  threadLoading,
  threadError,
  threadDraftSegments,
  onOpenThread,
  onCloseThread,
  onThreadDraftChange,
  onSendThreadReply,
}: ConversationPaneProps) {
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
  const composerDisabled = Boolean(managerProfileIncomplete);

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
      disabled={composerDisabled}
      usersById={usersById}
      locale={locale}
      theme={theme}
      showToolCalls={showToolCalls}
      t={t}
      onClose={onCloseThread}
      onDraftChange={onThreadDraftChange}
      threadSlashCandidates={threadSlashCandidates}
      threadSlashIndex={threadSlashIndex}
      threadSlashPickerLoading={threadSlashPickerLoading}
      threadSlashPickerOpen={threadSlashPickerOpen}
      onApplyThreadSlashCandidate={onApplyThreadSlashCandidate}
      onDismissThreadSlashPicker={onDismissThreadSlashPicker}
      onSetThreadSlashIndex={onSetThreadSlashIndex}
      mentionableUsers={conversationMembers}
      onPreviewUser={onPreviewUser}
      onSend={onSendThreadReply}
    />
  ) : null;

  return (
    <>
      <Conversation.Header
        channelToolsRef={channelToolsRef}
        conversation={conversation}
        conversationMembers={conversationMembers}
        description={description}
        inviteActionLabel={inviteActionLabel}
        logAgent={logAgent}
        logModalOpen={logModalOpen}
        selectedMessageCount={selectedMessageCount}
        showChannelTools={showChannelTools}
        showInviteAction={true}
        showMemberListAction={false}
        showToolCalls={showToolCalls}
        t={t}
        onClearMessages={handleOpenClearMessagesDialog}
        onDeleteRoom={handleOpenDeleteRoomDialog}
        onInviteAction={onInviteAction}
        onOpenAgentLogs={handleOpenAgentLogs}
        onPreviewUser={onPreviewUser}
        onToggleChannelTools={onToggleChannelTools}
        onToggleToolCalls={onToggleToolCalls}
      />

      <Conversation.MessageList
        agents={agents}
        conversation={conversation}
        currentUserID={currentUserID}
        emptyStateSlot={<></>}
        locale={locale}
        messageActionBusy={messageActionBusy}
        messageActionError={messageActionError}
        messageListRef={messageListRef}
        t={t}
        theme={theme}
        usersById={usersById}
        visibleMessages={visibleMessages}
        onMessageAction={onMessageAction}
        onOpenThread={onOpenThread}
        onPreviewUser={onPreviewUser}
      />

      <Conversation.Composer
        authBusyProvider={authBusyProvider}
        authStatuses={authStatuses}
        composerDisabled={composerDisabled}
        composerError={composerError}
        draftSegments={draftSegments}
        draftText={draftText}
        editorRef={editorRef}
        managerProfile={managerProfile}
        managerProvider={managerProvider}
        mentionCandidates={mentionCandidates}
        mentionIndex={mentionIndex}
        mentionableUsersByHandle={mentionableUsersByHandle}
        slashCandidates={slashCandidates}
        slashIndex={slashIndex}
        slashPickerLoading={slashPickerLoading}
        slashPickerOpen={slashPickerOpen}
        t={t}
        onApplyMention={onApplyMention}
        onApplySlashCandidate={onApplySlashCandidate}
        onComposerCompositionEnd={onComposerCompositionEnd}
        onComposerCompositionStart={onComposerCompositionStart}
        onComposerKeyDown={onComposerKeyDown}
        onProviderLogin={onProviderLogin}
        onSendMessage={onSendMessage}
        onSyncComposer={onSyncComposer}
      />
      {threadPanel}
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
    </>
  );
}
