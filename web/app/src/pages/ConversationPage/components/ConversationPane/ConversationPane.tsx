import { X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import { fetchAgentLogsRequest } from "@/api/agents";
import { errorMessage } from "@/api/client";
import {
  Conversation,
  type ConversationPaneProps,
  useConversationDraftEditorSync,
} from "@/components/business/ConversationPane";
import { AgentView } from "@/pages/AgentPage/components";
import { IconButton } from "@/components/ui";
import { normalizeAuthProviderName } from "@/models/agents";
import { getConversationDescription, isDirectConversation } from "@/models/conversations";
import type { AgentDetailSidePanelProps } from "@/hooks/workspace/types";

const AGENT_DETAIL_PANEL_DEFAULT_WIDTH = 760;
const AGENT_DETAIL_PANEL_MIN_WIDTH = 520;
const AGENT_DETAIL_PANEL_MAX_WIDTH = 1120;
const AGENT_DETAIL_PANEL_MIN_MAIN_WIDTH = 360;
const AGENT_DETAIL_PANEL_KEYBOARD_STEP = 24;
const AGENT_DETAIL_PANEL_KEYBOARD_LARGE_STEP = 80;

function clampAgentDetailPanelWidth(width: number, containerWidth = 0): number {
  const maxByContainer =
    containerWidth > 0
      ? Math.max(AGENT_DETAIL_PANEL_MIN_WIDTH, containerWidth - AGENT_DETAIL_PANEL_MIN_MAIN_WIDTH)
      : AGENT_DETAIL_PANEL_MAX_WIDTH;
  const maxWidth = Math.min(AGENT_DETAIL_PANEL_MAX_WIDTH, maxByContainer);
  if (!Number.isFinite(width)) {
    return AGENT_DETAIL_PANEL_DEFAULT_WIDTH;
  }
  return Math.min(maxWidth, Math.max(AGENT_DETAIL_PANEL_MIN_WIDTH, Math.round(width)));
}

function AgentDetailSidePanel({
  onClose,
  onResize,
  width = AGENT_DETAIL_PANEL_DEFAULT_WIDTH,
  ...props
}: AgentDetailSidePanelProps) {
  const panelRef = useRef<HTMLElement | null>(null);
  const dragRef = useRef<{ containerWidth: number; panelRight: number; pointerID: number } | null>(null);
  const [resizing, setResizing] = useState(false);

  const resolveContainerWidth = useCallback(() => {
    const panel = panelRef.current;
    const parent = panel?.closest(".chat-panel");
    const parentWidth = parent instanceof HTMLElement ? parent.getBoundingClientRect().width : 0;
    return parentWidth > 0 ? parentWidth : window.innerWidth;
  }, []);

  const resizeTo = useCallback(
    (nextWidth: number, containerWidth = resolveContainerWidth()) => {
      onResize?.(clampAgentDetailPanelWidth(nextWidth, containerWidth));
    },
    [onResize, resolveContainerWidth],
  );

  useEffect(() => {
    if (!resizing) {
      return undefined;
    }
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    return () => {
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
    };
  }, [resizing]);

  function handleResizePointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    const panel = panelRef.current;
    if (!panel) {
      return;
    }
    const panelRect = panel.getBoundingClientRect();
    const containerWidth = resolveContainerWidth();
    dragRef.current = {
      containerWidth,
      panelRight: panelRect.right,
      pointerID: event.pointerId,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
    event.preventDefault();
    setResizing(true);
  }

  function handleResizePointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }
    resizeTo(drag.panelRight - event.clientX, drag.containerWidth);
  }

  function finishResize(event: ReactPointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }
    dragRef.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    setResizing(false);
  }

  function handleResizeKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    const step = event.shiftKey ? AGENT_DETAIL_PANEL_KEYBOARD_LARGE_STEP : AGENT_DETAIL_PANEL_KEYBOARD_STEP;
    if (event.key === "ArrowLeft") {
      event.preventDefault();
      resizeTo(width + step);
      return;
    }
    if (event.key === "ArrowRight") {
      event.preventDefault();
      resizeTo(width - step);
      return;
    }
    if (event.key === "Home") {
      event.preventDefault();
      resizeTo(AGENT_DETAIL_PANEL_MIN_WIDTH);
      return;
    }
    if (event.key === "End") {
      event.preventDefault();
      resizeTo(AGENT_DETAIL_PANEL_MAX_WIDTH);
    }
  }

  return (
    <aside
      ref={panelRef}
      className={`agent-detail-side-panel ${resizing ? "is-resizing" : ""}`.trim()}
      aria-label={props.t("agentDetailPanel")}
    >
      <div
        className="agent-detail-resize-handle"
        role="separator"
        aria-label={props.t("resizeAgentDetailPanel")}
        aria-orientation="vertical"
        aria-valuemin={AGENT_DETAIL_PANEL_MIN_WIDTH}
        aria-valuemax={AGENT_DETAIL_PANEL_MAX_WIDTH}
        aria-valuenow={Math.round(width)}
        tabIndex={0}
        onKeyDown={handleResizeKeyDown}
        onPointerCancel={finishResize}
        onPointerDown={handleResizePointerDown}
        onPointerMove={handleResizePointerMove}
        onPointerUp={finishResize}
      />
      <div className="agent-detail-side-panel-bar">
        <span className="thread-panel-kicker">{props.t("agentDetailPanel")}</span>
        <IconButton
          className="modal-close"
          icon={<X size={20} strokeWidth={2} />}
          label={props.t("close")}
          markClassName="modal-close-icon"
          onClick={onClose}
          variant="tertiaryGray"
        />
      </div>
      <div className="agent-detail-side-panel-body">
        <AgentView {...props} />
      </div>
    </aside>
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
  mentionableUsersByName,
  onSyncComposer,
  onComposerKeyDown,
  onComposerCompositionStart,
  onComposerCompositionEnd,
  onSendMessage,
  composerError,
  messageActionBusy,
  messageActionError,
  onMessageAction,
  onCancelProfilePreviewClose,
  onCloseProfilePreview,
  onOpenAgentDetail,
  activeThreadRootID,
  activeThreadView,
  threadLoading,
  threadError,
  threadDraftSegments,
  onOpenThread,
  onCloseThread,
  onThreadDraftChange,
  onSendThreadReply,
  agentDetailPanelProps,
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
      disabled={composerDisabled}
      usersById={usersById}
      locale={locale}
      theme={theme}
      showToolCalls={showToolCalls}
      t={t}
      onClose={onCloseThread}
      onDraftChange={onThreadDraftChange}
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
  const sidePanel = agentDetailPanel ?? threadPanel;

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
        workingParticipants={workingParticipants}
        onApplyMention={onApplyMention}
        onApplySlashCandidate={onApplySlashCandidate}
        onComposerCompositionEnd={onComposerCompositionEnd}
        onComposerCompositionStart={onComposerCompositionStart}
        onComposerKeyDown={onComposerKeyDown}
        onConnectConnector={onConnectConnector}
        onDisconnectConnector={onDisconnectConnector}
        onManageConnector={onManageConnector}
        onProviderLogin={onProviderLogin}
        onSaveConnectorConfig={onSaveConnectorConfig}
        onSendMessage={onSendMessage}
        onSyncComposer={onSyncComposer}
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
