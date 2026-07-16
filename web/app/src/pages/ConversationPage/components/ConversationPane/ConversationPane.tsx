import { useCallback, useEffect, useState } from "react";
import { fetchAgentLogsRequest } from "@/api/agents";
import { errorMessage } from "@/api/client";
import {
  Conversation,
  type ConversationPaneProps,
  useConversationDraftEditorSync,
} from "@/components/business/ConversationPane";
import { AgentView } from "@/pages/AgentPage/components";
import { Button, DialogCloseButton, DialogContent, DialogRoot, DialogTitle } from "@/components/ui";
import { normalizeAuthProviderName } from "@/models/agents";
import { getConversationDescription, isDirectConversation } from "@/models/conversations";
import type { AgentDetailSidePanelProps } from "@/hooks/workspace/types";
import { CONVERSATION_ACTIVITY_ACTION_SEEN_STORAGE_KEY } from "@/shared/storage/keys";
import { ConversationActivityPanel } from "../ConversationActivityPanel";

function readConversationActivityActionSeen(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return window.localStorage.getItem(CONVERSATION_ACTIVITY_ACTION_SEEN_STORAGE_KEY) === "seen";
  } catch {
    return false;
  }
}

function writeConversationActivityActionSeen() {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(CONVERSATION_ACTIVITY_ACTION_SEEN_STORAGE_KEY, "seen");
  } catch {
    // Browsers can deny localStorage access; the action still works for this session.
  }
}

function AgentDetailSidePanel({ onClose, onOpenDM, ...props }: AgentDetailSidePanelProps) {
  const handleOpenDM = useCallback(
    async (...args: Parameters<typeof onOpenDM>) => {
      if (onClose(false) === false) {
        return;
      }
      await onOpenDM(...args);
    },
    [onClose, onOpenDM],
  );

  return (
    <DialogRoot open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent
        aria-describedby={undefined}
        aria-modal="true"
        className="agent-detail-side-panel"
        overlayClassName="agent-detail-drawer-backdrop"
      >
        <div className="agent-detail-side-panel-bar">
          <DialogCloseButton
            className="agent-detail-side-panel-close"
            label={props.t("close")}
            variant="tertiaryGray"
          />
          <DialogTitle className="agent-detail-side-panel-title">{props.t("agentDetailPanel")}</DialogTitle>
        </div>
        <div className="agent-detail-side-panel-body">
          <AgentView {...props} onOpenDM={handleOpenDM} />
        </div>
      </DialogContent>
    </DialogRoot>
  );
}

export function ConversationPane({
  conversation,
  visibleMessages,
  currentUserID = "",
  usersById,
  agents = [],
  locale,
  t,
  theme,
  workingParticipants = [],
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
  managerRuntimeUnavailable,
  authStatuses,
  authBusyProvider,
  connectorStatus,
  connectorBusyAction,
  connectorError,
  connectorPending,
  onSaveConnectorConfig,
  onConnectConnector,
  onDisconnectConnector,
  onManageConnector,
  onProviderLogin,
  draftSegments,
  draftText,
  attachmentDrafts,
  mentionableUsersByName,
  onSyncComposer,
  onComposerKeyDown,
  onComposerCompositionStart,
  onComposerCompositionEnd,
  onSendMessage,
  onAddAttachments,
  onRemoveAttachment,
  composerError,
  messageActionBusy,
  messageActionFeedback,
  onMessageAction,
  onCancelProfilePreviewClose,
  onCloseProfilePreview,
  onOpenAgentDetail,
  activeThreadRootID,
  activeThreadView,
  threadLoading,
  threadError,
  threadDraftSegments,
  threadAttachmentDrafts,
  onOpenThread,
  onCloseThread,
  onThreadDraftChange,
  onSendThreadReply,
  onAddThreadAttachments,
  onRemoveThreadAttachment,
  agentDetailPanelProps,
}: ConversationPaneProps) {
  const description = getConversationDescription(conversation, currentUserID, usersById, locale, t);
  const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
  const [logModalOpen, setLogModalOpen] = useState(false);
  const [logContent, setLogContent] = useState("");
  const [logError, setLogError] = useState("");
  const [logLoading, setLogLoading] = useState(false);
  const [activityPanelOpen, setActivityPanelOpen] = useState(false);
  const [activityActionSeen, setActivityActionSeen] = useState(readConversationActivityActionSeen);
  const [clearMessagesDialogOpen, setClearMessagesDialogOpen] = useState(false);
  const [deleteRoomDialogOpen, setDeleteRoomDialogOpen] = useState(false);
  const logAgentID = logAgent?.id || "";
  const logAgentName = logAgent?.name || conversation.title || "";
  const composerDisabledReason = managerRuntimeUnavailable ? t("managerCodexMissingWarning") : t("profileIncomplete");
  const composerDisabled = Boolean(managerRuntimeUnavailable || managerProfileIncomplete);

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

  const markActivityActionSeen = useCallback(() => {
    setActivityActionSeen(true);
    writeConversationActivityActionSeen();
  }, []);

  const handleToggleActivityPanel = useCallback(() => {
    if (!activityPanelOpen) {
      markActivityActionSeen();
      onCloseThread();
      onToggleChannelTools(false);
    }
    setActivityPanelOpen((open) => !open);
  }, [activityPanelOpen, markActivityActionSeen, onCloseThread, onToggleChannelTools]);

  const handleOpenActivityPanel = useCallback(() => {
    markActivityActionSeen();
    onCloseThread();
    onToggleChannelTools(false);
    setActivityPanelOpen(true);
  }, [markActivityActionSeen, onCloseThread, onToggleChannelTools]);

  useEffect(() => {
    if (activeThreadRootID) {
      setActivityPanelOpen(false);
    }
  }, [activeThreadRootID]);

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
      agents={agents}
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
      onAddAttachments={onAddThreadAttachments}
      onRemoveAttachment={onRemoveThreadAttachment}
      onCancelProfilePreviewClose={onCancelProfilePreviewClose}
      onCloseProfilePreview={onCloseProfilePreview}
      onOpenAgentDetail={onOpenAgentDetail}
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
  const agentDetailPanel = agentDetailPanelProps ? <AgentDetailSidePanel {...agentDetailPanelProps} /> : null;
  const activityPanel = activityPanelOpen ? (
    <ConversationActivityPanel
      key={conversation.id}
      agents={agents}
      conversation={conversation}
      locale={locale}
      t={t}
      usersById={usersById}
      onClose={() => setActivityPanelOpen(false)}
    />
  ) : null;
  const sidePanel = agentDetailPanel ?? activityPanel ?? threadPanel;

  return (
    <>
      <Conversation.Header
        channelToolsRef={channelToolsRef}
        conversation={conversation}
        conversationMembers={conversationMembers}
        description={description}
        headerAccessory={
          <Button
            className="icon-button activity-record-button"
            active={activityPanelOpen}
            iconOnly
            size="lg"
            variant="secondaryGray"
            aria-label={t("conversationActivityOpen")}
            aria-pressed={activityPanelOpen}
            data-tooltip={t("conversationActivityOpen")}
            data-tooltip-side="bottom"
            onClick={handleToggleActivityPanel}
          >
            <span className="icon-button-mark" aria-hidden="true">
              <ActivityWaveIcon />
            </span>
          </Button>
        }
        inviteActionLabel={inviteActionLabel}
        logAgent={logAgent}
        logModalOpen={logModalOpen}
        selectedMessageCount={selectedMessageCount}
        selectedVisibleMessageCount={visibleMessages.length}
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
        messageActionFeedback={messageActionFeedback}
        messageListRef={messageListRef}
        t={t}
        theme={theme}
        usersById={usersById}
        visibleMessages={visibleMessages}
        onMessageAction={onMessageAction}
        onCancelProfilePreviewClose={onCancelProfilePreviewClose}
        onCloseProfilePreview={onCloseProfilePreview}
        onOpenAgentDetail={onOpenAgentDetail}
        onOpenThread={onOpenThread}
        onPreviewUser={onPreviewUser}
      />

      <Conversation.Composer
        authBusyProvider={authBusyProvider}
        authStatuses={authStatuses}
        connectorStatus={connectorStatus}
        connectorBusyAction={connectorBusyAction}
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
        t={t}
        workingActionAttention={!activityActionSeen}
        workingActionLabel={t("conversationActivityView")}
        workingParticipants={workingParticipants}
        onApplyMention={onApplyMention}
        onApplySlashCandidate={onApplySlashCandidate}
        onAddAttachments={onAddAttachments}
        onComposerCompositionEnd={onComposerCompositionEnd}
        onComposerCompositionStart={onComposerCompositionStart}
        onComposerKeyDown={onComposerKeyDown}
        onConnectConnector={onConnectConnector}
        onDisconnectConnector={onDisconnectConnector}
        onManageConnector={onManageConnector}
        onProviderLogin={onProviderLogin}
        onSaveConnectorConfig={onSaveConnectorConfig}
        onSendMessage={onSendMessage}
        onRemoveAttachment={onRemoveAttachment}
        onSyncComposer={onSyncComposer}
        onWorkingAction={handleOpenActivityPanel}
      />
      {sidePanel}
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

function ActivityWaveIcon() {
  return (
    <svg aria-hidden="true" fill="none" focusable="false" viewBox="0 0 24 24">
      <path
        d="M4 12h3.4l2-4 3.4 8 2.1-4H20"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.9"
      />
    </svg>
  );
}
