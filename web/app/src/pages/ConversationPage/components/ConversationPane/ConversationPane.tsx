import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchAgentLogsRequest } from "@/api/agents";
import { errorMessage } from "@/api/client";
import {
  Conversation,
  AgentQuestionComposer,
  type ConversationWorkingParticipant,
  type ConversationPaneProps,
  useQuestionAnswerMode,
  useConversationDraftEditorSync,
} from "@/components/business/ConversationPane";
import { DocumentPreviewPanel, type DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { AgentView, type AgentDetailPaneHandle } from "@/pages/AgentPage/components";
import { Button, DialogCloseButton, DialogContent, DialogRoot, DialogTitle } from "@/components/ui";
import { normalizeAuthProviderName } from "@/models/agents";
import { getConversationDescription, isDirectConversation, isOnDemandConversation } from "@/models/conversations";
import type { AgentDetailSidePanelProps } from "@/hooks/workspace/types";
import { ConversationActivityPanel } from "../ConversationActivityPanel";
import { RoomTaskDialog, RoomTaskReference } from "../RoomTaskDialog";
import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { ListTodo } from "lucide-react";
import { type IMMessage } from "@/models/conversations";
import { roomTaskParent, roomTaskMessageAnchors } from "@/models/roomTasks";
import { useRoomTasks } from "../../useRoomTasks";
import {
  conversationActivityAgents,
  conversationActivityEntries,
  conversationWorkingParticipantsWithActivity,
} from "../ConversationActivityPanel/conversationActivity";

function hasBlockingAgentDetailChangesAfterMetadataCommit(
  props: Pick<AgentDetailSidePanelProps, "draft" | "hasUnsavedChanges" | "savedDraft">,
  committedFields: Array<"description" | "name">,
): boolean {
  if (!props.hasUnsavedChanges) {
    return false;
  }
  if (!committedFields.length || !props.draft || !props.savedDraft) {
    return true;
  }
  const normalizedDraft = { ...props.draft };
  committedFields.forEach((field) => {
    normalizedDraft[field] = props.savedDraft?.[field] ?? "";
  });
  return JSON.stringify(normalizedDraft) !== JSON.stringify(props.savedDraft);
}

function AgentDetailSidePanel({ onClose, onOpenDM, ...props }: AgentDetailSidePanelProps) {
  const [dialogPortalContainer, setDialogPortalContainer] = useState<HTMLDivElement | null>(null);
  const detailPaneRef = useRef<AgentDetailPaneHandle | null>(null);
  const initialFocusRef = useRef<HTMLButtonElement | null>(null);
  const requestClose = useCallback(
    (restoreFocus = true) => {
      const committedFields = detailPaneRef.current?.commitActiveMetadataEdit() ?? [];
      const skipUnsavedCheck = !hasBlockingAgentDetailChangesAfterMetadataCommit(props, committedFields);
      return onClose(restoreFocus, { skipUnsavedCheck });
    },
    [onClose, props],
  );
  const handleOpenDM = useCallback(
    async (...args: Parameters<typeof onOpenDM>) => {
      if (requestClose(false) === false) {
        return;
      }
      await onOpenDM(...args);
    },
    [onOpenDM, requestClose],
  );

  return (
    <DialogRoot open onOpenChange={(open) => (!open ? requestClose() : undefined)}>
      <DialogContent
        ref={setDialogPortalContainer}
        aria-describedby={undefined}
        aria-modal="true"
        className="agent-detail-side-panel"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          initialFocusRef.current?.focus({ preventScroll: true });
        }}
        onEscapeKeyDown={(event) => {
          const canceledFields = detailPaneRef.current?.cancelActiveMetadataEdit() ?? [];
          if (canceledFields.length) {
            event.preventDefault();
          }
        }}
        overlayClassName="agent-detail-drawer-backdrop"
      >
        <div className="agent-detail-side-panel-bar">
          <DialogCloseButton
            ref={initialFocusRef}
            className="icon-button agent-detail-side-panel-close"
            label={props.t("close")}
            title=""
          />
          <DialogTitle className="agent-detail-side-panel-title">{props.t("agentDetailPanel")}</DialogTitle>
        </div>
        <div className="agent-detail-side-panel-body">
          <AgentView
            ref={detailPaneRef}
            {...props}
            dialogPortalContainer={dialogPortalContainer}
            onOpenDM={handleOpenDM}
          />
        </div>
      </DialogContent>
    </DialogRoot>
  );
}

type RoomTaskSlots = { taskHeader?: ReactNode; taskDialog?: ReactNode; taskFooter?: (message: IMMessage) => ReactNode };
export function ConversationPane(props: ConversationPaneProps) {
  if (isOnDemandConversation(props.conversation) && !props.conversation.is_direct) {
    return <RoomTaskConversation key={props.conversation.id} {...props} />;
  }
  return <ConversationPaneContent {...props} />;
}
function RoomTaskConversation(props: ConversationPaneProps) {
  const { conversation, t } = props;
  const query = useRoomTasks(conversation);
  const [params, setParams] = useSearchParams();
  const selected = params.get("task") ?? "";
  const trigger = useRef<HTMLElement | null>(null);
  const tasks = query.data ?? [];
  const anchors = roomTaskMessageAnchors(conversation.messages ?? [], conversation.manager_id ?? "");
  const open = (id: string, anchor?: HTMLElement) => {
    if (anchor) trigger.current = anchor;
    void query.refetch();
    setParams((current) => {
      const next = new URLSearchParams(current);
      next.set("task", id);
      return next;
    });
  };
  const close = () =>
    setParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.delete("task");
        return next;
      },
      { replace: true },
    );
  const footer = (message: IMMessage) => {
    const id = anchors.get(message.id ?? "");
    if (!id) return null;
    const parent = roomTaskParent(tasks, id);
    return (
      <RoomTaskReference
        compact
        taskID={parent?.id ?? id}
        task={parent}
        tasks={tasks}
        t={t}
        onOpen={(anchor) => open(parent?.id ?? id, anchor)}
      />
    );
  };
  return (
    <ConversationPaneContent
      {...props}
      taskFooter={footer}
      taskHeader={
        <Button
          size="lg"
          variant="secondaryGray"
          iconOnly
          aria-label={t("roomTasksOpen")}
          onClick={(event) => open("list", event.currentTarget)}
        >
          <ListTodo size={20} />
        </Button>
      }
      taskDialog={
        <RoomTaskDialog
          roomID={conversation.id}
          roomTitle={conversation.title ?? conversation.id}
          taskID={selected}
          tasks={tasks}
          loading={query.isLoading}
          error={query.isError ? errorMessage(query.error, t("tasksLoadFailed")) : ""}
          onClose={close}
          onRestoreFocus={() => trigger.current?.focus({ preventScroll: true })}
          t={t}
        />
      }
    />
  );
}
function ConversationPaneContent({
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
  gitlabConnectorStatus,
  connectorBusyAction,
  connectorBusyProvider,
  connectorError,
  connectorPending,
  onSaveConnectorConfig,
  onSaveGitLabConnectorConfig,
  onConnectConnector,
  onDisconnectConnector,
  onDisconnectGitLabConnector,
  onManageConnector,
  onProviderLogin,
  draftSegments,
  draftText,
  attachmentDrafts,
  removedAttachmentCount,
  removedAttachmentName,
  sendError,
  sendProgress,
  sendStatus,
  mentionableUsersByName,
  onSyncComposer,
  onComposerKeyDown,
  onComposerCompositionStart,
  onComposerCompositionEnd,
  onSendMessage,
  onRetrySend,
  onStopSend,
  onUndoRemoveAttachment,
  onAddAttachments,
  onRemoveAttachment,
  composerError,
  messageActionBusy,
  messageActionFeedback,
  onMessageAction,
  onPreserveMessageAnchor = () => {},
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
  onThreadSlashQueryChange,
  onSendThreadReply,
  onStopWorkingTurn,
  onAddThreadAttachments,
  onRemoveThreadAttachment,
  agentDetailPanelProps,
  taskHeader,
  taskDialog,
  taskFooter,
}: ConversationPaneProps & RoomTaskSlots) {
  const description = getConversationDescription(conversation, currentUserID, usersById, locale, t);
  const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
  const [logModalOpen, setLogModalOpen] = useState(false);
  const [logContent, setLogContent] = useState("");
  const [logError, setLogError] = useState("");
  const [logLoading, setLogLoading] = useState(false);
  const [activityPanelOpen, setActivityPanelOpen] = useState(false);
  const [focusedActivityEntryID, setFocusedActivityEntryID] = useState<string | null>(null);
  const [clearMessagesDialogOpen, setClearMessagesDialogOpen] = useState(false);
  const [deleteRoomDialogOpen, setDeleteRoomDialogOpen] = useState(false);
  const [documentPreview, setDocumentPreview] = useState<DocumentPreviewRequest | null>(null);
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
  const activityAgents = useMemo(() => conversationActivityAgents(conversation, agents), [agents, conversation]);
  const activityEntries = useMemo(
    () => conversationActivityEntries(conversation.messages, activityAgents, conversation.members, usersById),
    [activityAgents, conversation.members, conversation.messages, usersById],
  );
  const workingParticipantsWithActivity = useMemo(
    () => conversationWorkingParticipantsWithActivity(workingParticipants, activityAgents, activityEntries),
    [activityAgents, activityEntries, workingParticipants],
  );

  useConversationDraftEditorSync(editorRef, draftSegments);

  useEffect(() => {
    setLogModalOpen(false);
    setLogContent("");
    setLogError("");
    setLogLoading(false);
    setClearMessagesDialogOpen(false);
    setDeleteRoomDialogOpen(false);
    setDocumentPreview(null);
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

  const handleToggleActivityPanel = useCallback(() => {
    if (!activityPanelOpen) {
      onCloseThread();
      onToggleChannelTools(false);
      setDocumentPreview(null);
    }
    setActivityPanelOpen((open) => !open);
  }, [activityPanelOpen, onCloseThread, onToggleChannelTools]);

  const handleOpenActivityPanel = useCallback(
    (participant?: ConversationWorkingParticipant) => {
      setFocusedActivityEntryID(participant?.activity?.entryID || null);
      onCloseThread();
      onToggleChannelTools(false);
      setDocumentPreview(null);
      setActivityPanelOpen(true);
    },
    [onCloseThread, onToggleChannelTools],
  );

  const handlePreviewAttachment = useCallback(
    (request: DocumentPreviewRequest) => {
      if (agentDetailPanelProps?.onClose(false) === false) {
        return;
      }
      onPreserveMessageAnchor(request.anchor);
      onCloseThread();
      setActivityPanelOpen(false);
      setDocumentPreview(request);
    },
    [agentDetailPanelProps, onCloseThread, onPreserveMessageAnchor],
  );

  useEffect(() => {
    if (activeThreadRootID) {
      setActivityPanelOpen(false);
      setDocumentPreview(null);
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
      onSlashQueryChange={onThreadSlashQueryChange}
      onAddAttachments={onAddThreadAttachments}
      onRemoveAttachment={onRemoveThreadAttachment}
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
      onPreviewAttachment={handlePreviewAttachment}
      onQuestionSelect={threadQuestionMode.select}
      questionMode={threadQuestionMode}
      onSend={onSendThreadReply}
    />
  ) : null;
  const agentDetailPanel = agentDetailPanelProps ? <AgentDetailSidePanel {...agentDetailPanelProps} /> : null;
  const activityPanel = activityPanelOpen ? (
    <ConversationActivityPanel
      key={conversation.id}
      agents={agents}
      conversation={conversation}
      initialEntryID={focusedActivityEntryID}
      locale={locale}
      t={t}
      usersById={usersById}
      onClose={() => setActivityPanelOpen(false)}
    />
  ) : null;
  const documentPreviewPanel = documentPreview ? (
    <DocumentPreviewPanel
      {...documentPreview}
      t={t}
      onClose={() => setDocumentPreview(null)}
      onIndexChange={(index) => setDocumentPreview((current) => (current ? { ...current, index } : current))}
    />
  ) : null;
  const sidePanel = agentDetailPanel ?? documentPreviewPanel ?? activityPanel ?? threadPanel;

  return (
    <>
      {taskDialog}
      <Conversation.Header
        channelToolsRef={channelToolsRef}
        conversation={conversation}
        conversationMembers={conversationMembers}
        description={description}
        headerAccessory={
          <>
            {taskHeader}
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
          </>
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
        renderMessageFooter={taskFooter}
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
        onOpenAgentDetail={onOpenAgentDetail}
        onOpenThread={onOpenThread}
        onPreviewUser={onPreviewUser}
        onPreviewAttachment={handlePreviewAttachment}
        onQuestionSelect={questionMode.select}
      />

      {questionMode.pending.length > 0 ? (
        <AgentQuestionComposer mode={questionMode} t={t} usersById={usersById} />
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
          removedAttachmentCount={removedAttachmentCount}
          removedAttachmentName={removedAttachmentName}
          sendError={sendError}
          sendProgress={sendProgress}
          sendStatus={sendStatus}
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
          workingParticipants={workingParticipantsWithActivity}
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
          onPreviewAttachment={handlePreviewAttachment}
          onRetrySend={onRetrySend}
          onSaveConnectorConfig={onSaveConnectorConfig}
          onSaveGitLabConnectorConfig={onSaveGitLabConnectorConfig}
          onSendMessage={onSendMessage}
          onStopSend={onStopSend}
          onUndoRemoveAttachment={onUndoRemoveAttachment}
          onStopWorkingTurn={onStopWorkingTurn}
          onRemoveAttachment={onRemoveAttachment}
          onSyncComposer={onSyncComposer}
          onWorkingAction={handleOpenActivityPanel}
        />
      )}
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
