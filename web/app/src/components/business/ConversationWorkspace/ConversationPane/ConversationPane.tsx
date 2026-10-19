import { TaskConversationControls } from "../TaskConversationControls/TaskConversationControls";
import { WorkingTurnControls } from "@/components/business/ConversationPane/WorkingTurnControls";
import { workingParticipantForProgress } from "@/models/turnWorkingParticipant";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { errorMessage } from "@/api/client";
import {
  Conversation,
  AgentQuestionComposer,
  type ConversationWorkingParticipant,
  type ConversationPaneProps,
  useQuestionAnswerMode,
  useConversationDraftEditorSync,
} from "@/components/business/ConversationPane";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { ChatSidePanel, useChatTools } from "@/components/business/ChatSidePanel";
import { type CitationSelection } from "@/components/business/MessageContent";
import { AgentView, type AgentDetailPaneHandle } from "@/components/business/AgentDetails";
import { Button, DialogCloseButton, DialogContent, DialogRoot, DialogTitle } from "@/components/ui";
import { normalizeAuthProviderName } from "@/models/agents";
import { getConversationDescription, isDirectConversation, isOnDemandConversation } from "@/models/conversations";
import type { AgentDetailSidePanelProps } from "@/hooks/workspace/types";
import { RoomTaskDialog, RoomTaskReference } from "../RoomTaskDialog";
import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { ListTodo } from "lucide-react";
import { type IMMessage } from "@/models/conversations";
import { roomTaskParent, roomTaskMessageAnchors } from "@/models/roomTasks";
import { localizeAPIError } from "@/shared/i18n";
import { useRoomTasks } from "../useRoomTasks";
import {
  conversationActivityAgents,
  conversationActivityEntries,
  conversationWorkingParticipantsWithActivity,
} from "@/components/business/ConversationActivityPanel/conversationActivity";

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
  onNewTask,
  mentionCandidates,
  mentionIndex,
  onApplyMention,
  slashCandidates = [],
  slashIndex = 0,
  slashContinuation,
  slashPickerLoading = false,
  slashPickerOpen = false,
  onApplySlashCandidate = (_name) => {},
  threadSlashCandidates = [],
  threadSlashIndex = 0,
  threadSlashContinuation,
  threadSlashPickerLoading = false,
  threadSlashPickerOpen = false,
  onApplyThreadSlashCandidate = (_name) => {},
  onDismissThreadSlashPicker = () => {},
  onSetThreadSlashIndex = (_index) => {},
  managerProfile,
  composerDisabledReason = "",
  threadComposerDisabledReason = "",
  authStatuses,
  authBusyProvider,
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
  onRestoreThread,
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
  const tools = useChatTools({ conversation, currentUserID, agents, usersById, editorRef, t });
  const { openPreview, openActivity, openSources } = tools;
  const { openTab, closeTab, state: sideState } = tools.controller;
  const [clearMessagesDialogOpen, setClearMessagesDialogOpen] = useState(false);
  const [deleteRoomDialogOpen, setDeleteRoomDialogOpen] = useState(false);
  const [deleteRoomBusy, setDeleteRoomBusy] = useState(false);
  const [deleteRoomError, setDeleteRoomError] = useState("");
  const composerDisabled = Boolean(composerDisabledReason);
  const mainMessages = useMemo(
    () => conversation.messages.filter((message) => !message.relates_to),
    [conversation.messages],
  );
  const threadMessages = useMemo(
    () => (activeThreadView?.root ? [activeThreadView.root, ...(activeThreadView.replies ?? [])] : []),
    [activeThreadView],
  );
  const questionMode = useQuestionAnswerMode({
    messages: mainMessages,
    responderID: currentUserID,
    roomID: conversation.id,
    t,
  });
  const threadQuestionMode = useQuestionAnswerMode({
    messages: threadMessages,
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

  const messageControls = useMemo(() => {
    const byMessage = new Map<string, ConversationWorkingParticipant>();
    const moved = new Set<ConversationWorkingParticipant>();
    const threadVisible =
      activeThreadRootID &&
      !agentDetailPanelProps &&
      tools.controller.state.open &&
      tools.controller.state.activeID === "thread";
    const threadMessages =
      threadVisible && activeThreadView?.root ? [activeThreadView.root, ...(activeThreadView.replies ?? [])] : [];
    for (const message of [...visibleMessages, ...threadMessages]) {
      const participant = workingParticipantForProgress(message, conversation.id, workingParticipantsWithActivity);
      if (
        !message.id ||
        !participant ||
        !(participant.showContextUsage || participant.contextUsage || (participant.canStop && onStopWorkingTurn))
      )
        continue;
      byMessage.set(message.id, participant);
      moved.add(participant);
    }
    return {
      byMessage,
      composerParticipants: workingParticipantsWithActivity.filter((participant) => !moved.has(participant)),
    };
  }, [
    activeThreadRootID,
    activeThreadView,
    agentDetailPanelProps,
    tools.controller.state.open,
    tools.controller.state.activeID,
    conversation.id,
    onStopWorkingTurn,
    visibleMessages,
    workingParticipantsWithActivity,
  ]);
  const renderTurnControls = useCallback(
    (message: IMMessage) => {
      const participant = messageControls.byMessage.get(message.id || "");
      return participant
        ? {
            header: (
              <>
                <WorkingTurnControls participant={participant} t={t} onStop={onStopWorkingTurn} placement="message" />
              </>
            ),
          }
        : null;
    },
    [messageControls.byMessage, onStopWorkingTurn, t],
  );

  useConversationDraftEditorSync(editorRef, draftSegments);

  useEffect(() => {
    setClearMessagesDialogOpen(false);
    setDeleteRoomDialogOpen(false);
    setDeleteRoomBusy(false);
    setDeleteRoomError("");
  }, [conversation.id]);

  const handleOpenActivityPanel = useCallback(
    (participant?: ConversationWorkingParticipant) => {
      onToggleChannelTools(false);
      openActivity(participant?.activity?.entryID);
    },
    [onToggleChannelTools, openActivity],
  );

  const handlePreviewAttachment = useCallback(
    (request: DocumentPreviewRequest, originTabID?: "files") => {
      if (agentDetailPanelProps?.onClose(false) === false) return;
      onPreserveMessageAnchor(request.anchor);
      onToggleChannelTools(false);
      openPreview(request, originTabID);
    },
    [agentDetailPanelProps, onPreserveMessageAnchor, onToggleChannelTools, openPreview],
  );

  const previousThreadRoot = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (previousThreadRoot.current === activeThreadRootID) return;
    previousThreadRoot.current = activeThreadRootID;
    if (activeThreadRootID && !sideState.tabs.some((tab) => tab.kind === "thread" && tab.rootID === activeThreadRootID))
      openTab({ id: "thread", kind: "thread", rootID: activeThreadRootID });
  }, [activeThreadRootID, sideState.tabs, openTab]);

  const handleCloseThread = useCallback(() => {
    closeTab("thread");
    onCloseThread();
  }, [closeTab, onCloseThread]);

  const handleOpenCitation = useCallback(
    (selection: CitationSelection) => {
      if (agentDetailPanelProps?.onClose(false) === false) return;
      onPreserveMessageAnchor(selection.anchor);
      onToggleChannelTools(false);
      openSources(selection);
    },
    [agentDetailPanelProps, onPreserveMessageAnchor, onToggleChannelTools, openSources],
  );

  const handleOpenClearMessagesDialog = useCallback(() => {
    onToggleChannelTools(false);
    setClearMessagesDialogOpen(true);
  }, [onToggleChannelTools]);

  const handleOpenDeleteRoomDialog = useCallback(() => {
    onToggleChannelTools(false);
    setDeleteRoomError("");
    setDeleteRoomDialogOpen(true);
  }, [onToggleChannelTools]);

  const handleConfirmDeleteRoom = useCallback(async () => {
    setDeleteRoomBusy(true);
    setDeleteRoomError("");
    try {
      await onDeleteRoom(conversation.id);
      setDeleteRoomDialogOpen(false);
    } catch (err) {
      setDeleteRoomError(localizeAPIError(err, t, errorMessage(err, t("deleteRoomFailed"))));
    } finally {
      setDeleteRoomBusy(false);
    }
  }, [conversation.id, onDeleteRoom, t]);

  const threadPanel = activeThreadRootID ? (
    <Conversation.ThreadPanel
      renderTurnControls={renderTurnControls}
      agents={agents}
      thread={activeThreadView}
      loading={threadLoading}
      error={threadError}
      fileScope={tools.fileScope}
      draftSegments={threadDraftSegments}
      attachmentDrafts={threadAttachmentDrafts}
      disabled={Boolean(threadComposerDisabledReason)}
      usersById={usersById}
      locale={locale}
      theme={theme}
      showToolCalls={showToolCalls}
      t={t}
      onClose={handleCloseThread}
      onDraftChange={onThreadDraftChange}
      onSlashQueryChange={onThreadSlashQueryChange}
      onAddAttachments={onAddThreadAttachments}
      onRemoveAttachment={onRemoveThreadAttachment}
      onOpenAgentDetail={onOpenAgentDetail}
      threadSlashCandidates={threadSlashCandidates}
      threadSlashIndex={threadSlashIndex}
      threadSlashContinuation={threadSlashContinuation}
      threadSlashPickerLoading={threadSlashPickerLoading}
      threadSlashPickerOpen={threadSlashPickerOpen}
      onApplyThreadSlashCandidate={onApplyThreadSlashCandidate}
      onDismissThreadSlashPicker={onDismissThreadSlashPicker}
      onSetThreadSlashIndex={onSetThreadSlashIndex}
      mentionableUsers={conversationMembers}
      onPreviewUser={onPreviewUser}
      onPreviewAttachment={handlePreviewAttachment}
      onQuestionSelect={threadQuestionMode.select}
      onCitationSelect={handleOpenCitation}
      questionMode={threadQuestionMode}
      onSend={onSendThreadReply}
    />
  ) : null;
  const agentDetailPanel = agentDetailPanelProps ? <AgentDetailSidePanel {...agentDetailPanelProps} /> : null;

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
            {tools.toolbar}
          </>
        }
        inviteActionLabel={inviteActionLabel}
        selectedMessageCount={selectedMessageCount}
        selectedVisibleMessageCount={visibleMessages.length}
        showChannelTools={showChannelTools}
        showChannelToolsAction={false}
        showInviteAction={!conversation.is_direct}
        showMemberListAction={false}
        showToolCalls={showToolCalls}
        t={t}
        onClearMessages={handleOpenClearMessagesDialog}
        onDeleteRoom={handleOpenDeleteRoomDialog}
        onInviteAction={onInviteAction}
        onPreviewUser={onPreviewUser}
        onToggleChannelTools={onToggleChannelTools}
        onToggleToolCalls={onToggleToolCalls}
      />

      <Conversation.MessageList
        renderTurnControls={renderTurnControls}
        renderMessageFooter={taskFooter}
        agents={agents}
        conversation={conversation}
        currentUserID={currentUserID}
        emptyStateSlot={false}
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
        onCitationSelect={handleOpenCitation}
      />

      {questionMode.pending.length > 0 ? (
        <AgentQuestionComposer mode={questionMode} t={t} usersById={usersById} />
      ) : (
        <Conversation.Composer
          fileScope={tools.fileScope}
          authBusyProvider={authBusyProvider}
          authStatuses={authStatuses}
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
          slashContinuation={slashContinuation}
          slashPickerLoading={slashPickerLoading}
          slashPickerOpen={slashPickerOpen}
          t={t}
          toolbar={
            <TaskConversationControls
              conversation={conversation}
              participants={workingParticipantsWithActivity}
              agents={agents}
              t={t}
              onNewTask={onNewTask}
            />
          }
          hideWorkingContextUsage
          workingParticipants={messageControls.composerParticipants}
          onApplyMention={onApplyMention}
          onApplySlashCandidate={onApplySlashCandidate}
          onAddAttachments={onAddAttachments}
          onComposerCompositionEnd={onComposerCompositionEnd}
          onComposerCompositionStart={onComposerCompositionStart}
          onComposerKeyDown={onComposerKeyDown}
          onProviderLogin={onProviderLogin}
          onPreviewAttachment={handlePreviewAttachment}
          onRetrySend={onRetrySend}
          onSendMessage={onSendMessage}
          onStopSend={onStopSend}
          onUndoRemoveAttachment={onUndoRemoveAttachment}
          onStopWorkingTurn={onStopWorkingTurn}
          onRemoveAttachment={onRemoveAttachment}
          onSyncComposer={onSyncComposer}
          onWorkingAction={handleOpenActivityPanel}
        />
      )}
      {agentDetailPanel}
      <ChatSidePanel
        key={conversation.id}
        controller={tools.controller}
        fileScope={tools.fileScope}
        conversation={conversation}
        agents={agents}
        usersById={usersById}
        locale={locale}
        t={t}
        onReference={tools.onReference}
        onPreview={(item) => handlePreviewAttachment({ items: [item], index: 0 }, "files")}
        onSources={handleOpenCitation}
        onCollapse={tools.collapse}
        activeThreadRootID={activeThreadRootID}
        threadPanel={threadPanel}
        onThreadSelect={(rootID) => {
          if (onRestoreThread) {
            void onRestoreThread(rootID);
            return;
          }
          const message = conversation.messages.find((item) => item.id === rootID);
          if (message) void onOpenThread(message);
        }}
        onThreadClose={onCloseThread}
      />

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
          busy={deleteRoomBusy}
          cancelLabel={t("cancel")}
          closeLabel={t("close")}
          confirmLabel={t("deleteRoomConfirm")}
          description={t("deleteRoomConfirmBody")}
          error={deleteRoomError}
          open={deleteRoomDialogOpen}
          title={t("deleteRoom")}
          onConfirm={handleConfirmDeleteRoom}
          onOpenChange={(open) => {
            setDeleteRoomDialogOpen(open);
            if (!open) {
              setDeleteRoomError("");
            }
          }}
        />
      ) : null}
    </>
  );
}
