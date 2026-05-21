import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { errorMessage } from "@/api/client";
import {
  createRoomRequest,
  deleteRoomRequest,
  fetchThreadRequest,
  inviteRoomUsersRequest,
  sendMessageRequest,
  startThreadRequest,
} from "@/api/im";
import {
  appendMessageToData,
  appendReplyToThreadView,
  applyIMEvent,
  applyThreadToData,
  conversationThreadViews,
  isDirectConversation,
  isThreadReply,
  isToolCallMessage,
  removeConversationFromData,
  THREAD_RELATION_TYPE,
  threadKey,
  threadMessageKey,
  threadViewKey,
  upsertConversationInData,
} from "@/models/conversations";
import {
  areComposerSegmentsEqual,
  getComposerMentionState,
  insertComposerLineBreak,
  isComposerKeyboardEventComposing,
  parseComposerSegments,
  placeCaretAtEnd,
  removeAdjacentMentionToken,
  renderComposerSegments,
  replaceMentionQueryWithToken,
  segmentsToPlainText,
  serializeComposerSegments,
  updateDrafts,
} from "@/models/composer";
import { WorkspacePaneTypes } from "@/models/routing";
import { normalizeAuthProviderName, providerNeedsAuth } from "@/models/agents";
import { localizeError } from "@/shared/i18n";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import { MESSAGE_LIST_BOTTOM_THRESHOLD } from "@/shared/constants/workspace";
import type { IMMessage, IMServerEvent, IMUser, ThreadView } from "@/models/conversations";
import type { UseConversationControllerArgs } from "./types";

type ComposerSegment =
  | {
      text: string;
      type: "text";
    }
  | {
      type: "mention";
      userId: string;
      userName: string;
    };

type ComposerMentionState = {
  endOffset: number;
  query: string;
  startOffset: number;
  textNode: Node;
};

type DraftsByConversationId = Record<string, ComposerSegment[]>;
type DraftsByThreadKey = Record<string, string>;

type OpenCreateRoomOptions = {
  description?: string;
  lockedMemberIDs?: string[];
  preselectedMemberIDs?: string[];
  title?: string;
};

export function useConversationController({
  activeConversationId,
  activePane,
  authBusyProvider,
  authStatuses,
  data,
  locale,
  managerProfile,
  managerProfileIncomplete,
  navigatePane,
  onMessageAction,
  onProviderLogin,
  onUpgradeStatusChange,
  rooms,
  selectComputer,
  selectConversation,
  setActiveConversationId,
  setBootstrapData,
  showToolCalls,
  setShowToolCalls,
  t,
  theme,
  messageActionBusy,
  messageActionError,
}: UseConversationControllerArgs) {
  const [draftsByConversationId, setDraftsByConversationId] = useState<DraftsByConversationId>({});
  const [threadDraftsByKey, setThreadDraftsByKey] = useState<DraftsByThreadKey>({});
  const [activeThreadRootID, setActiveThreadRootID] = useState("");
  const [activeThreadView, setActiveThreadView] = useState<ThreadView | null>(null);
  const [threadLoading, setThreadLoading] = useState(false);
  const [threadError, setThreadError] = useState("");
  const [composerMentionState, setComposerMentionState] = useState<ComposerMentionState | null>(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [showCreateRoom, setShowCreateRoom] = useState(false);
  const [showInvite, setShowInvite] = useState(false);
  const [showMemberList, setShowMemberList] = useState(false);
  const [showChannelTools, setShowChannelTools] = useState(false);
  const [roomTitle, setRoomTitle] = useState("");
  const [roomDescription, setRoomDescription] = useState("");
  const [roomMemberIDs, setRoomMemberIDs] = useState<string[]>([]);
  const [lockedRoomMemberIDs, setLockedRoomMemberIDs] = useState<string[]>([]);
  const [inviteUserIDs, setInviteUserIDs] = useState<string[]>([]);
  const [submitError, setSubmitError] = useState("");
  const [composerError, setComposerError] = useState("");
  const editorRef = useRef<HTMLElement | null>(null);
  const composerIsComposingRef = useRef(false);
  const composerJustEndedCompositionRef = useRef(false);
  const messageListRef = useRef<HTMLElement | null>(null);
  const memberMenuRef = useRef<HTMLElement | null>(null);
  const channelToolsRef = useRef<HTMLElement | null>(null);
  const shouldAutoScrollRef = useRef(true);
  const autoScrollConversationRef = useRef(activeConversationId);
  const activeThreadKeyRef = useRef("");

  const usersById = useMemo(() => {
    const result = new Map<string, IMUser>();
    data?.users.forEach((user) => result.set(user.id, user));
    return result;
  }, [data]);

  const activeConversation = useMemo(
    () => data?.rooms.find((item) => item.id === activeConversationId) ?? null,
    [data, activeConversationId],
  );
  const visibleMessages = useMemo(() => {
    if (!activeConversation) {
      return [];
    }
    return activeConversation.messages.filter((message) => {
      if (isThreadReply(message)) {
        return false;
      }
      return showToolCalls || !isToolCallMessage(message.content);
    });
  }, [activeConversation, showToolCalls]);
  const channels = useMemo(() => rooms.filter((room) => !isDirectConversation(room)), [rooms]);
  const directMessages = useMemo(() => rooms.filter((room) => isDirectConversation(room)), [rooms]);
  const threadGroups = useMemo(
    () =>
      rooms
        .map((room) => ({ conversation: room, threads: conversationThreadViews(room) }))
        .filter((group) => group.threads.length > 0),
    [rooms],
  );
  const threadCount = useMemo(
    () => threadGroups.reduce((count, group) => count + group.threads.length, 0),
    [threadGroups],
  );
  const roomCount = rooms.length;
  const selectedConversation = activePane.type === WorkspacePaneTypes.conversation ? activeConversation : null;
  const activeChannel =
    selectedConversation && !isDirectConversation(selectedConversation) ? selectedConversation : null;
  const selectedMessageCount = selectedConversation?.messages?.length ?? 0;
  const activeConversationMembers = activeConversation
    ? activeConversation.members.map((id) => usersById.get(id)).filter(Boolean)
    : [];
  const inviteCandidates = activeConversation
    ? data?.users.filter((user) => !activeConversation.members.includes(user.id)) || []
    : [];
  const inviteActionLabel =
    activeConversation && isDirectConversation(activeConversation) ? t("createRoomFromDM") : t("inviteMembers");

  const mentionCandidates = useMemo(() => {
    if (!data || !composerMentionState) {
      return [];
    }
    const allowed = new Set(activeConversation?.members ?? []);
    return data.users
      .filter((user) => allowed.has(user.id))
      .filter(
        (user) =>
          user.handle.toLowerCase().includes(composerMentionState.query.toLowerCase()) ||
          user.name.toLowerCase().includes(composerMentionState.query.toLowerCase()),
      )
      .slice(0, 5);
  }, [data, activeConversation, composerMentionState]);
  const mentionableUsersByHandle = useMemo(() => {
    const result = new Map<string, IMUser>();
    if (!data) {
      return result;
    }
    const allowed = new Set(activeConversation?.members ?? []);
    data.users
      .filter((user) => allowed.has(user.id))
      .forEach((user) => {
        const handle = String(user.handle ?? "")
          .trim()
          .toLowerCase();
        if (handle && !result.has(handle)) {
          result.set(handle, user);
        }
      });
    return result;
  }, [data, activeConversation]);

  const draftSegments = useMemo(
    () => draftsByConversationId[activeConversationId] ?? [],
    [draftsByConversationId, activeConversationId],
  );
  const draftText = useMemo(() => segmentsToPlainText(draftSegments), [draftSegments]);
  const activeThreadDraftKey = activeThreadRootID ? threadKey(activeConversationId, activeThreadRootID) : "";
  const activeThreadDraft = activeThreadDraftKey ? (threadDraftsByKey[activeThreadDraftKey] ?? "") : "";

  useEffect(() => {
    activeThreadKeyRef.current = activeThreadRootID ? threadKey(activeConversationId, activeThreadRootID) : "";
  }, [activeConversationId, activeThreadRootID]);

  useEffect(() => {
    const unsubscribe = subscribeIMEvents((payload: IMServerEvent) => {
      setBootstrapData((current) => applyIMEvent(current, payload));
      if ((payload?.type === "thread.created" || payload?.type === "thread.updated") && payload.thread) {
        if (threadViewKey(payload.thread) === activeThreadKeyRef.current) {
          setActiveThreadView(payload.thread);
        }
      }
      if (payload?.type === "message.created" && payload.message) {
        if (threadMessageKey(payload.room_id, payload.message) === activeThreadKeyRef.current) {
          setActiveThreadView((current) => appendReplyToThreadView(current, payload.message));
        }
      }
      if (payload?.type === "upgrade.status_changed" && payload.upgrade) {
        onUpgradeStatusChange(payload.upgrade);
      }
    });

    return () => {
      unsubscribe();
    };
  }, [onUpgradeStatusChange, setBootstrapData]);

  useEffect(() => {
    setMentionIndex(0);
  }, [activeConversationId, composerMentionState?.query, draftText]);

  useEffect(() => {
    if (!managerProfileIncomplete) {
      setComposerError("");
    }
  }, [managerProfileIncomplete]);

  useEffect(() => {
    if (!showCreateRoom) {
      setRoomTitle("");
      setRoomDescription("");
      setRoomMemberIDs([]);
      setLockedRoomMemberIDs([]);
      setSubmitError("");
    }
  }, [showCreateRoom]);

  useEffect(() => {
    if (!showInvite) {
      setInviteUserIDs([]);
      setSubmitError("");
    }
  }, [showInvite]);

  useEffect(() => {
    setShowMemberList(false);
    setShowChannelTools(false);
  }, [activeConversationId, activePane.type]);

  useEffect(() => {
    if (!showMemberList) {
      return undefined;
    }

    function handlePointerDown(event: MouseEvent) {
      const menu = memberMenuRef.current;
      if (!menu || !(event.target instanceof Node) || menu.contains(event.target)) {
        return;
      }
      setShowMemberList(false);
    }

    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [showMemberList]);

  useEffect(() => {
    if (!showChannelTools) {
      return undefined;
    }

    function handlePointerDown(event: MouseEvent) {
      const menu = channelToolsRef.current;
      if (!menu || !(event.target instanceof Node) || menu.contains(event.target)) {
        return;
      }
      setShowChannelTools(false);
    }

    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [showChannelTools]);

  useEffect(() => {
    if (!data) {
      return;
    }
    const fallbackConversationId = data.rooms[0]?.id ?? "";
    if (!activeConversationId) {
      if (fallbackConversationId) {
        setActiveConversationId(fallbackConversationId);
        if (activePane.type === WorkspacePaneTypes.conversation && !activePane.id) {
          navigatePane({ type: WorkspacePaneTypes.conversation, id: fallbackConversationId }, data.rooms, {
            replace: true,
          });
        }
      } else if (activePane.type === WorkspacePaneTypes.conversation && !activePane.id) {
        selectComputer({ replace: true });
      }
      return;
    }
    if (!data.rooms.some((room) => room.id === activeConversationId)) {
      const nextID = fallbackConversationId;
      if (nextID) {
        if (activePane.type === WorkspacePaneTypes.conversation) {
          selectConversation(nextID, { replace: true });
        } else {
          setActiveConversationId(nextID);
        }
      } else if (activePane.type === WorkspacePaneTypes.conversation) {
        setActiveConversationId("");
        selectComputer({ replace: true });
      } else {
        setActiveConversationId("");
      }
    }
  }, [
    data,
    activeConversationId,
    activePane.id,
    activePane.type,
    navigatePane,
    selectComputer,
    selectConversation,
    setActiveConversationId,
  ]);

  useEffect(() => {
    const el = messageListRef.current;
    if (!el) {
      return;
    }
    const updateAutoScrollState = () => {
      const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
      shouldAutoScrollRef.current = distanceFromBottom <= MESSAGE_LIST_BOTTOM_THRESHOLD;
    };
    updateAutoScrollState();
    el.addEventListener("scroll", updateAutoScrollState);
    return () => el.removeEventListener("scroll", updateAutoScrollState);
  }, [activeConversationId]);

  useLayoutEffect(() => {
    if (activePane.type !== WorkspacePaneTypes.conversation) {
      return;
    }
    const el = messageListRef.current;
    if (!el) {
      return;
    }
    autoScrollConversationRef.current = activeConversationId;
    el.scrollTop = el.scrollHeight;
    shouldAutoScrollRef.current = true;
  }, [activePane.type, activeConversationId]);

  useEffect(() => {
    const el = messageListRef.current;
    if (autoScrollConversationRef.current !== activeConversationId) {
      autoScrollConversationRef.current = activeConversationId;
      shouldAutoScrollRef.current = false;
      return;
    }
    if (!el || !shouldAutoScrollRef.current) {
      return;
    }
    el.scrollTop = el.scrollHeight;
  }, [visibleMessages.length, activeConversationId]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    if (!areComposerSegmentsEqual(parseComposerSegments(editor), draftSegments)) {
      renderComposerSegments(editor, draftSegments);
    }
    setComposerMentionState(null);
  }, [activeConversationId, draftSegments]);

  useEffect(() => {
    if (!activeConversationId || showCreateRoom || showInvite) {
      return;
    }
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    requestAnimationFrame(() => {
      if (editorRef.current !== editor) {
        return;
      }
      editor.focus();
      placeCaretAtEnd(editor);
    });
  }, [activeConversationId, showCreateRoom, showInvite]);

  async function sendMessage(): Promise<void> {
    if (managerProfileIncomplete) {
      setComposerError(t("profileIncomplete"));
      return;
    }
    const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
    if (providerNeedsAuth(managerProvider) && authStatuses[managerProvider]?.authenticated === false) {
      setComposerError(t("authRequired"));
      return;
    }
    if (!data || !activeConversation || !draftText.trim()) {
      return;
    }

    setComposerError("");
    try {
      const created = await sendMessageRequest({
        room_id: activeConversation.id,
        sender_id: data.current_user_id,
        content: serializeComposerSegments(draftSegments),
      });
      setBootstrapData((current) => appendMessageToData(current, activeConversation.id, created));
      clearComposer();
    } catch (_) {
      setComposerError(t("sendFailed"));
    }
  }

  async function openThreadInConversation(conversationID: string, message: IMMessage | null | undefined) {
    if (!conversationID || !message?.id) {
      return;
    }
    const rootID = isThreadReply(message) ? message.relates_to?.event_id : message.id;
    if (!rootID) {
      return;
    }

    if (conversationID !== activeConversationId) {
      selectConversation(conversationID);
    }
    setThreadError("");
    setThreadLoading(true);
    setActiveThreadRootID(rootID);
    try {
      const view = await startThreadRequest(conversationID, { root_message_id: rootID });
      setActiveThreadView(view);
      setBootstrapData((current) => applyThreadToData(current, conversationID, view));
    } catch (err) {
      setThreadError(errorMessage(err, t("threadLoadFailed")));
    } finally {
      setThreadLoading(false);
    }
  }

  async function openThread(message: IMMessage | null | undefined) {
    if (!activeConversation) {
      return;
    }
    await openThreadInConversation(activeConversation.id, message);
  }

  async function refreshThreadView(roomID: string, rootID: string) {
    const view = await fetchThreadRequest(roomID, rootID);
    setActiveThreadView(view);
    setBootstrapData((current) => applyThreadToData(current, roomID, view));
    return view;
  }

  async function sendThreadReply(): Promise<void> {
    if (managerProfileIncomplete) {
      setThreadError(t("profileIncomplete"));
      return;
    }
    const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
    if (providerNeedsAuth(managerProvider) && authStatuses[managerProvider]?.authenticated === false) {
      setThreadError(t("authRequired"));
      return;
    }
    const text = activeThreadDraft.trim();
    if (!data || !activeConversation || !activeThreadRootID || !text) {
      return;
    }

    setThreadError("");
    try {
      const created = await sendMessageRequest({
        room_id: activeConversation.id,
        sender_id: data.current_user_id,
        content: text,
        relates_to: {
          rel_type: THREAD_RELATION_TYPE,
          event_id: activeThreadRootID,
        },
      });
      setThreadDraftsByKey((current) => ({ ...current, [activeThreadDraftKey]: "" }));
      setActiveThreadView((current) => appendReplyToThreadView(current, created));
      setBootstrapData((current) => appendMessageToData(current, activeConversation.id, created));
      await refreshThreadView(activeConversation.id, activeThreadRootID);
    } catch (err) {
      setThreadError(errorMessage(err, t("sendFailed")));
    }
  }

  function closeThread() {
    setActiveThreadRootID("");
    setActiveThreadView(null);
    setThreadLoading(false);
    setThreadError("");
  }

  function selectConversationAndCloseThread(id: string) {
    closeThread();
    selectConversation(id);
  }

  async function createRoom(): Promise<void> {
    if (!data || !roomTitle.trim()) {
      return;
    }

    setSubmitError("");
    const memberIDs = roomMemberIDs.filter((id) => id && id !== data.current_user_id);
    try {
      const created = await createRoomRequest({
        title: roomTitle,
        description: roomDescription,
        creator_id: data.current_user_id,
        member_ids: memberIDs,
        locale,
      });
      setBootstrapData((current) => upsertConversationInData(current, created));
      selectConversation(created.id);
      setComposerError("");
      setShowCreateRoom(false);
    } catch (err) {
      setSubmitError(localizeError(err.message, t));
    }
  }

  function openCreateRoomModal(options: OpenCreateRoomOptions = {}) {
    if (!data) {
      return;
    }
    const lockedIDs = Array.from(new Set((options.lockedMemberIDs ?? [data.current_user_id]).filter(Boolean)));
    const selectedIDs = Array.from(new Set((options.preselectedMemberIDs ?? lockedIDs).filter(Boolean)));
    setRoomTitle(options.title ?? "");
    setRoomDescription(options.description ?? "");
    setRoomMemberIDs(selectedIDs);
    setLockedRoomMemberIDs(lockedIDs);
    setSubmitError("");
    setShowInvite(false);
    setShowCreateRoom(true);
  }

  function handleInviteAction() {
    if (!activeConversation) {
      return;
    }
    if (isDirectConversation(activeConversation)) {
      openCreateRoomModal({
        preselectedMemberIDs: activeConversation.members,
        lockedMemberIDs: activeConversation.members,
      });
      return;
    }
    setSubmitError("");
    setInviteUserIDs([]);
    setShowInvite(true);
  }

  async function inviteUsers(): Promise<void> {
    if (!data || !activeConversation || inviteUserIDs.length === 0) {
      return;
    }

    setSubmitError("");
    try {
      const updated = await inviteRoomUsersRequest({
        room_id: activeConversation.id,
        inviter_id: data.current_user_id,
        user_ids: inviteUserIDs,
        locale,
      });
      setBootstrapData((current) => upsertConversationInData(current, updated));
      setComposerError("");
      setShowInvite(false);
    } catch (err) {
      setSubmitError(localizeError(err.message, t));
    }
  }

  async function deleteRoom(roomID: string): Promise<void> {
    if (!data || !roomID) {
      return;
    }

    try {
      await deleteRoomRequest(roomID);
    } catch (err) {
      setComposerError(localizeError(err.message, t));
      return;
    }

    const remainingRooms = rooms.filter((item) => item.id !== roomID);
    setBootstrapData((current) => removeConversationFromData(current, roomID));
    setDraftsByConversationId((current) => {
      if (!current[roomID]) {
        return current;
      }
      const next = { ...current };
      delete next[roomID];
      return next;
    });
    setComposerError("");
    setSubmitError("");
    if (activeConversationId === roomID) {
      const nextID = remainingRooms[0]?.id ?? "";
      if (nextID) {
        selectConversation(nextID, { replace: true });
      } else {
        setActiveConversationId("");
        selectComputer({ replace: true });
      }
    }
  }

  function applyMention(user: IMUser | null | undefined) {
    const editor = editorRef.current;
    const state = getComposerMentionState(editor);
    if (!state) {
      return;
    }
    if (!replaceMentionQueryWithToken(editor, state, user)) {
      return;
    }
    syncComposerFromEditor();
  }

  function onComposerKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
    if (
      isComposerKeyboardEventComposing(event) ||
      composerIsComposingRef.current ||
      (event.key === "Enter" && composerJustEndedCompositionRef.current)
    ) {
      return;
    }

    if (mentionCandidates.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setMentionIndex((value) => (value + 1) % mentionCandidates.length);
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setMentionIndex((value) => (value - 1 + mentionCandidates.length) % mentionCandidates.length);
        return;
      }
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        applyMention(mentionCandidates[mentionIndex]);
        return;
      }
    }

    if (event.key === "Backspace" && removeAdjacentMentionToken(editorRef.current, "backward")) {
      event.preventDefault();
      syncComposerFromEditor();
      return;
    }

    if (event.key === "Delete" && removeAdjacentMentionToken(editorRef.current, "forward")) {
      event.preventDefault();
      syncComposerFromEditor();
      return;
    }

    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      sendMessage();
      return;
    }

    if (event.key === "Enter" && event.shiftKey) {
      event.preventDefault();
      insertComposerLineBreak(editorRef.current);
      syncComposerFromEditor();
    }
  }

  function onComposerCompositionStart() {
    composerIsComposingRef.current = true;
    composerJustEndedCompositionRef.current = false;
  }

  function onComposerCompositionEnd() {
    composerIsComposingRef.current = false;
    composerJustEndedCompositionRef.current = true;
    requestAnimationFrame(() => {
      composerJustEndedCompositionRef.current = false;
      syncComposerFromEditor();
    });
  }

  function syncComposerFromEditor() {
    const editor = editorRef.current;
    if (!editor || !activeConversationId) {
      return;
    }
    const segments = parseComposerSegments(editor) as ComposerSegment[];
    setDraftsByConversationId((current) => updateDrafts(current, activeConversationId, segments));
    setComposerMentionState(getComposerMentionState(editor) as ComposerMentionState | null);
  }

  function clearComposer() {
    const editor = editorRef.current;
    if (editor) {
      editor.innerHTML = "";
      editor.focus();
    }
    if (activeConversationId) {
      setDraftsByConversationId((current) => updateDrafts(current, activeConversationId, []));
    }
    setComposerMentionState(null);
  }

  function clearComposerError() {
    setComposerError("");
  }

  function closeConversationTools() {
    setShowMemberList(false);
    setShowChannelTools(false);
  }

  return {
    activeChannel,
    activeConversation,
    activeConversationMembers,
    activeThreadRootID,
    channels,
    closeConversationTools,
    directMessages,
    openThreadInConversation,
    roomCount,
    selectedConversation,
    selectedMessageCount,
    selectConversationAndCloseThread,
    threadCount,
    threadGroups,
    usersById,
    visibleMessages,
    clearComposerError,
    openCreateRoomModal,
    conversationViewProps: {
      conversation: selectedConversation,
      visibleMessages,
      currentUserID: data?.current_user_id,
      usersById,
      locale,
      t,
      theme,
      selectedMessageCount,
      conversationMembers: activeConversationMembers,
      showMemberList,
      onToggleMemberList: setShowMemberList,
      showChannelTools,
      onToggleChannelTools: setShowChannelTools,
      showToolCalls,
      onToggleToolCalls: setShowToolCalls,
      memberMenuRef,
      channelToolsRef,
      messageListRef,
      editorRef,
      onDeleteRoom: deleteRoom,
      inviteActionLabel,
      onInviteAction: handleInviteAction,
      mentionCandidates,
      mentionIndex,
      onApplyMention: applyMention,
      managerProfile,
      managerProfileIncomplete,
      authStatuses,
      authBusyProvider,
      onProviderLogin,
      draftSegments,
      draftText,
      mentionableUsersByHandle,
      onSyncComposer: syncComposerFromEditor,
      onComposerKeyDown,
      onComposerCompositionStart,
      onComposerCompositionEnd,
      onSendMessage: sendMessage,
      composerError,
      messageActionBusy,
      messageActionError,
      onMessageAction,
      activeThreadRootID,
      activeThreadView,
      threadLoading,
      threadError,
      threadDraft: activeThreadDraft,
      onOpenThread: openThread,
      onCloseThread: closeThread,
      onThreadDraftChange: (value: string) => {
        if (!activeThreadDraftKey) {
          return;
        }
        setThreadDraftsByKey((current) => ({ ...current, [activeThreadDraftKey]: value }));
      },
      onSendThreadReply: sendThreadReply,
    },
    createRoomModalProps:
      showCreateRoom && data
        ? {
            t,
            roomTitle,
            onRoomTitleChange: setRoomTitle,
            roomDescription,
            onRoomDescriptionChange: setRoomDescription,
            candidates: data.users,
            roomMemberIDs,
            lockedRoomMemberIDs,
            onRoomMemberIDsChange: setRoomMemberIDs,
            submitError,
            onClose: () => setShowCreateRoom(false),
            onCreate: createRoom,
          }
        : null,
    inviteMembersModalProps:
      showInvite && data
        ? {
            t,
            candidates: inviteCandidates,
            inviteUserIDs,
            onInviteUserIDsChange: setInviteUserIDs,
            submitError,
            onClose: () => setShowInvite(false),
            onInvite: inviteUsers,
          }
        : null,
  };
}
