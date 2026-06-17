import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { errorMessage } from "@/api/client";
import {
  clearRoomMessagesRequest,
  createRoomRequest,
  deleteRoomRequest,
  fetchThreadRequest,
  inviteRoomUsersRequest,
  sendMessageRequest,
  startThreadRequest,
} from "@/api/im";
import { fetchAgentSkills, fetchAgentSkillsFile } from "@/api/agents";
import {
  agentMatchesUser,
  appendMessageToData,
  appendReplyToThreadView,
  applyIMEvent,
  applyThreadToData,
  conversationThreadViews,
  isDirectConversation,
  isThreadReply,
  isToolCallMessage,
  removeConversationFromData,
  resolveConversationUser,
  THREAD_RELATION_TYPE,
  threadKey,
  threadMessageKey,
  threadViewKey,
  upsertConversationInData,
} from "@/models/conversations";
import {
  areComposerSegmentsEqual,
  type ComposerSegment,
  getMentionCandidates,
  getComposerMentionState,
  insertComposerLineBreak,
  isComposerKeyboardEventComposing,
  parseComposerSegments,
  placeCaretAtEnd,
  normalizeComposerSegmentsForDisplay,
  removeAdjacentMentionToken,
  renderComposerSegments,
  replaceComposerSlashWithSegments,
  replaceMentionQueryWithToken,
  segmentsToPlainText,
  serializeComposerSegments,
  updateDrafts,
} from "@/models/composer";
import { WorkspacePaneTypes } from "@/models/routing";
import { isAgentRunning, normalizeAuthProviderName, providerNeedsAuth } from "@/models/agents";
import { skillDescriptionFromMarkdown, skillOptionsFromWorkspace, type SlashSkillOption } from "@/models/slashCommands";
import { localizeError } from "@/shared/i18n";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import { MESSAGE_LIST_BOTTOM_THRESHOLD } from "@/shared/constants/workspace";
import type { IMMessage, IMServerEvent, IMUser, ThreadView } from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";
import type { UseConversationControllerArgs } from "./types";

const slashSkillOptionsCache = new Map<string, SlashSkillOption[]>();
const slashSkillOptionsRequests = new Map<string, Promise<SlashSkillOption[]>>();

export { skillDescriptionFromMarkdown } from "@/models/slashCommands";

type ComposerMentionState = {
  endOffset: number;
  query: string;
  startOffset: number;
  textNode: Node;
};

type DraftsByConversationId = Record<string, ComposerSegment[]>;
type DraftsByThreadKey = Record<string, ComposerSegment[]>;

type OpenCreateRoomOptions = {
  description?: string;
  lockedMemberIDs?: string[];
  preselectedMemberIDs?: string[];
  title?: string;
};

function clearThreadDraftsForConversation(current: DraftsByThreadKey, conversationID: string): DraftsByThreadKey {
  const prefix = `${conversationID}:`;
  let changed = false;
  const next: DraftsByThreadKey = {};
  for (const [key, value] of Object.entries(current)) {
    if (key.startsWith(prefix)) {
      changed = true;
      continue;
    }
    next[key] = value;
  }
  return changed ? next : current;
}

export function useConversationController({
  activeConversationId,
  activePane,
  agents,
  autoSelectFallbackConversation = true,
  authBusyProvider,
  authStatuses,
  data,
  locale,
  managerProfile,
  managerProfileIncomplete,
  navigatePane,
  onMessageAction,
  onProviderLogin,
  onRefreshAgentState,
  onUpgradeStatusChange,
  preferredFallbackConversationId = "",
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
  messageListActive = true,
}: UseConversationControllerArgs) {
  const [draftsByConversationId, setDraftsByConversationId] = useState<DraftsByConversationId>({});
  const [threadDraftsByKey, setThreadDraftsByKey] = useState<DraftsByThreadKey>({});
  const [activeThreadRootID, setActiveThreadRootID] = useState("");
  const [activeThreadView, setActiveThreadView] = useState<ThreadView | null>(null);
  const [threadLoading, setThreadLoading] = useState(false);
  const [threadError, setThreadError] = useState("");
  const [composerMentionState, setComposerMentionState] = useState<ComposerMentionState | null>(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [skillOptions, setSkillOptions] = useState<SlashSkillOption[]>([]);
  const [slashIndex, setSlashIndex] = useState(0);
  const [slashPickerLoading, setSlashPickerLoading] = useState(false);
  const [slashPickerDismissed, setSlashPickerDismissed] = useState(false);
  const [threadSlashPickerDismissed, setThreadSlashPickerDismissed] = useState(false);
  const [threadSlashIndex, setThreadSlashIndex] = useState(0);
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
  const editorRef = useRef<HTMLDivElement | null>(null);
  const composerIsComposingRef = useRef(false);
  const composerJustEndedCompositionRef = useRef(false);
  const messageListRef = useRef<HTMLElement | null>(null);
  const memberMenuRef = useRef<HTMLDivElement | null>(null);
  const channelToolsRef = useRef<HTMLDivElement | null>(null);
  const shouldAutoScrollRef = useRef(true);
  const autoScrollConversationRef = useRef(activeConversationId);
  const activeThreadKeyRef = useRef("");
  const agentsRef = useRef(agents);
  const usersByIdRef = useRef<Map<string, IMUser>>(new Map());
  const refreshAgentStateRef = useRef(onRefreshAgentState);

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
      return showToolCalls || !isToolCallMessage(message);
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
  const logAgent = useMemo(() => {
    if (!selectedConversation || !data?.current_user_id) {
      return null;
    }

    const directUser = resolveConversationUser(selectedConversation, data.current_user_id, usersById);
    const otherMembers = selectedConversation.members.filter((id) => id && id !== data.current_user_id);
    if (otherMembers.length !== 1) {
      return null;
    }
    const agentID = directUser?.id || otherMembers[0];
    const agent = agents.find((item) => item.id === agentID || agentMatchesUser(item, directUser));
    return agent ?? null;
  }, [agents, data?.current_user_id, selectedConversation, usersById]);
  const activeConversationAgentMembers = useMemo(() => {
    if (!selectedConversation) {
      return [];
    }

    return selectedConversation.members
      .filter((memberId) => memberId !== data?.current_user_id)
      .map((memberId) => ({
        memberId: memberId,
        user: usersById.get(memberId),
      }))
      .filter((entry) =>
        agents.some((agent) => agent.id === entry.memberId || (entry.user && agentMatchesUser(agent, entry.user))),
      )
      .map((entry) => entry.memberId);
  }, [agents, data?.current_user_id, selectedConversation, usersById]);
  const hasActiveConversationAgent = useMemo(() => {
    return activeConversationAgentMembers.length > 0;
  }, [activeConversationAgentMembers]);
  const activeConversationAgentId = useMemo(() => {
    if (logAgent?.id) {
      return logAgent.id;
    }
    if (activeConversationAgentMembers.length === 1) {
      return activeConversationAgentMembers[0];
    }
    return "";
  }, [activeConversationAgentMembers, logAgent?.id]);
  const activeConversationMembers = activeConversation
    ? activeConversation.members.map((id) => usersById.get(id)).filter((user): user is IMUser => Boolean(user))
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
    return getMentionCandidates(
      data.users.filter((user) => allowed.has(user.id)),
      composerMentionState.query,
    );
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
    () => normalizeComposerSegmentsForDisplay(draftsByConversationId[activeConversationId] ?? []),
    [draftsByConversationId, activeConversationId],
  );
  const draftText = useMemo(() => segmentsToPlainText(draftSegments), [draftSegments]);
  const slashPickerEnabled = Boolean((hasActiveConversationAgent || logAgent?.id) && !slashPickerDismissed);
  const slashPickerState = useMemo(
    () =>
      buildSlashPickerState({
        draftText,
        enabled: slashPickerEnabled,
        skillOptions,
      }),
    [draftText, slashPickerEnabled, skillOptions],
  );
  const slashPickerQuery = slashPickerState.query;
  const slashPickerActive = slashPickerState.active;
  const slashCandidates = slashPickerState.candidates;
  const activeThreadDraftKey = activeThreadRootID ? threadKey(activeConversationId, activeThreadRootID) : "";
  const activeThreadDraftSegments = useMemo(() => {
    if (!activeThreadDraftKey) {
      return [];
    }
    return activeThreadDraftKey ? (threadDraftsByKey[activeThreadDraftKey] ?? []) : [];
  }, [activeThreadDraftKey, threadDraftsByKey]);
  const activeThreadDraft = useMemo(() => segmentsToPlainText(activeThreadDraftSegments), [activeThreadDraftSegments]);
  const threadSlashPickerEnabled = Boolean((logAgent?.id || hasActiveConversationAgent) && activeThreadDraftKey);
  const threadSlashPickerState = useMemo(
    () =>
      buildSlashPickerState({
        draftText: activeThreadDraft,
        enabled: threadSlashPickerEnabled,
        skillOptions,
        disabled: threadSlashPickerDismissed,
      }),
    [activeThreadDraft, threadSlashPickerEnabled, threadSlashPickerDismissed, skillOptions],
  );
  const threadSlashPickerQuery = threadSlashPickerState.query;
  const threadSlashPickerActive = threadSlashPickerState.active;
  const threadSlashCandidates = threadSlashPickerState.candidates;
  const isAnySlashPickerNeeded = slashPickerActive || threadSlashPickerActive;

  useEffect(() => {
    activeThreadKeyRef.current = activeThreadRootID ? threadKey(activeConversationId, activeThreadRootID) : "";
  }, [activeConversationId, activeThreadRootID]);

  useEffect(() => {
    agentsRef.current = agents;
    usersByIdRef.current = usersById;
    refreshAgentStateRef.current = onRefreshAgentState;
  }, [agents, onRefreshAgentState, usersById]);

  useEffect(() => {
    const unsubscribe = subscribeIMEvents((payload: IMServerEvent) => {
      setBootstrapData((current) => applyIMEvent(current, payload));
      if ((payload?.type === "thread.created" || payload?.type === "thread.updated") && payload.thread) {
        if (threadViewKey(payload.thread) === activeThreadKeyRef.current) {
          setActiveThreadView(payload.thread);
        }
      }
      if (payload?.type === "message.created" && payload.message) {
        const senderID = String(payload.message.sender_id || "").trim();
        if (senderID) {
          const sender = usersByIdRef.current.get(senderID) ?? { id: senderID };
          const senderAgent = agentsRef.current.find((agent) => agentMatchesUser(agent, sender));
          if (senderAgent?.id && !isAgentRunning(senderAgent)) {
            void refreshAgentStateRef.current(String(senderAgent.id));
          }
        }
        if (threadMessageKey(payload.room_id, payload.message) === activeThreadKeyRef.current) {
          setActiveThreadView((current) => appendReplyToThreadView(current, payload.message) ?? null);
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
    setSlashPickerDismissed(false);
  }, [draftText]);

  useEffect(() => {
    setSkillOptions([]);
    setSlashIndex(0);
    setSlashPickerDismissed(false);
  }, [activeConversationId]);

  useEffect(() => {
    setThreadSlashIndex(0);
    setThreadSlashPickerDismissed(false);
  }, [activeThreadDraftKey]);

  useEffect(() => {
    setSlashIndex(0);
  }, [slashPickerQuery, skillOptions]);

  useEffect(() => {
    setThreadSlashIndex(0);
  }, [threadSlashPickerQuery, skillOptions]);

  useEffect(() => {
    if (!activeConversationAgentId) {
      setSkillOptions([]);
      setSlashPickerLoading(false);
      return;
    }

    const cached = slashSkillOptionsCache.get(activeConversationAgentId);
    if (cached) {
      setSkillOptions(cached);
      setSlashPickerLoading(false);
      return;
    }

    let cancelled = false;
    setSkillOptions([]);
    setSlashPickerLoading(false);
    loadSlashSkillOptions(activeConversationAgentId, (skills) => {
      if (cancelled) {
        return;
      }
      setSkillOptions(skills);
      setSlashPickerLoading(false);
    })
      .then((skills) => {
        if (cancelled) {
          return;
        }
        setSkillOptions(skills);
      })
      .catch(() => {
        if (!cancelled) {
          setSkillOptions([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setSlashPickerLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [activeConversationAgentId]);

  useEffect(() => {
    if (!isAnySlashPickerNeeded || !activeConversationAgentId || skillOptions.length > 0) {
      setSlashPickerLoading(false);
      return;
    }
    setSlashPickerLoading(slashSkillOptionsRequests.has(activeConversationAgentId));
  }, [activeConversationAgentId, isAnySlashPickerNeeded, skillOptions.length]);

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
    if (!autoSelectFallbackConversation) {
      return;
    }
    const preferredFallbackID = String(preferredFallbackConversationId || "").trim();
    const fallbackConversationId = data.rooms.some((room) => room.id === preferredFallbackID)
      ? preferredFallbackID
      : (data.rooms[0]?.id ?? "");
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
    autoSelectFallbackConversation,
    preferredFallbackConversationId,
  ]);

  useEffect(() => {
    if (!messageListActive) {
      return;
    }
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
  }, [activeConversationId, messageListActive]);

  useLayoutEffect(() => {
    if (!messageListActive) {
      return;
    }
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
  }, [activePane.type, activeConversationId, messageListActive]);

  useEffect(() => {
    if (!messageListActive) {
      return;
    }
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
  }, [visibleMessages.length, activeConversationId, messageListActive]);

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
    if (!data?.current_user_id || !activeConversation || !draftText.trim()) {
      return;
    }

    setComposerError("");
    const serializedDraft = serializeComposerSegments(draftSegments);
    const content = normalizeSlashShorthandForPayload(serializedDraft);
    try {
      const created = await sendMessageRequest({
        room_id: activeConversation.id,
        sender_id: data.current_user_id,
        content,
      });
      setBootstrapData((current) => appendMessageToData(current, activeConversation.id, created));
      clearComposer();
    } catch (err) {
      setComposerError(errorMessage(err, t("sendFailed")));
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
    if (!activeThreadDraft.trim()) {
      return;
    }
    const serializedDraft = serializeComposerSegments(activeThreadDraftSegments);
    const text = normalizeSlashShorthandForPayload(serializedDraft);
    if (!data?.current_user_id || !activeConversation || !activeThreadRootID || !text.trim()) {
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
      setThreadDraftsByKey((current) => updateDrafts(current, activeThreadDraftKey, []));
      setActiveThreadView((current) => appendReplyToThreadView(current, created) ?? null);
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
    if (!data?.current_user_id || !roomTitle.trim()) {
      return;
    }

    setSubmitError("");
    const memberIDs = roomMemberIDs.filter((id): id is string => Boolean(id && id !== data.current_user_id));
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
      setSubmitError(localizeError(errorMessage(err, ""), t));
    }
  }

  function openCreateRoomModal(options: OpenCreateRoomOptions = {}) {
    if (!data) {
      return;
    }
    const lockedIDs = Array.from(
      new Set((options.lockedMemberIDs ?? [data.current_user_id]).filter((id): id is string => Boolean(id))),
    );
    const selectedIDs = Array.from(
      new Set((options.preselectedMemberIDs ?? lockedIDs).filter((id): id is string => Boolean(id))),
    );
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
    if (!data?.current_user_id || !activeConversation || inviteUserIDs.length === 0) {
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
      setSubmitError(localizeError(errorMessage(err, ""), t));
    }
  }

  async function deleteRoom(roomID: string): Promise<void> {
    if (!data || !roomID) {
      return;
    }

    try {
      await deleteRoomRequest(roomID);
    } catch (err) {
      setComposerError(localizeError(errorMessage(err, ""), t));
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

  async function clearRoomMessages(roomID: string): Promise<void> {
    if (!data || !roomID) {
      return;
    }

    let clearedRoom;
    try {
      clearedRoom = await clearRoomMessagesRequest(roomID);
    } catch (err) {
      setComposerError(localizeError(errorMessage(err, ""), t));
      return;
    }

    setBootstrapData((current) => upsertConversationInData(current, clearedRoom));
    setThreadDraftsByKey((current) => clearThreadDraftsForConversation(current, roomID));
    setComposerError("");
    setSubmitError("");
    if (activeConversationId === roomID) {
      closeThread();
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

  function applySlashCandidate(name: string | null | undefined, editor?: HTMLElement | null) {
    const skillName = String(name || "").trim();
    if (!skillName || !activeConversationId) {
      return;
    }
    const nextText = slashCommandInputText(skillName);
    const nextSegments = normalizeComposerSegmentsForDisplay([{ type: "text", text: nextText }]);
    applySlashSuggestionToComposer(editor ?? editorRef.current, nextSegments, () =>
      setDraftsByConversationId((current) => updateDrafts(current, activeConversationId, nextSegments)),
    );
    setSlashIndex(0);
  }

  function applyThreadSlashCandidate(name: string | null | undefined, editor?: HTMLElement | null) {
    const skillName = String(name || "").trim();
    if (!skillName || !activeThreadDraftKey) {
      return;
    }
    const nextText = slashCommandInputText(skillName);
    const nextSegments = normalizeComposerSegmentsForDisplay([{ type: "text", text: nextText }]);
    applySlashSuggestionToComposer(editor, nextSegments, () => {
      setThreadDraftsByKey((current) => updateDrafts(current, activeThreadDraftKey, nextSegments));
    });
    setThreadSlashIndex(0);
  }

  function applySlashSuggestionToComposer(
    editor: HTMLElement | null | undefined,
    segments: ComposerSegment[],
    onCommit: () => void,
  ) {
    if (!editor) {
      onCommit();
      return;
    }
    if (!replaceComposerSlashWithSegments(editor, segments)) {
      renderComposerSegments(editor, segments);
      placeCaretAtEnd(editor);
    }
    editor.focus();
    onCommit();
  }

  function onComposerKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
    if (
      isComposerKeyboardEventComposing(event) ||
      composerIsComposingRef.current ||
      (event.key === "Enter" && composerJustEndedCompositionRef.current)
    ) {
      return;
    }

    if (slashPickerActive) {
      if (event.key === "ArrowDown" && slashCandidates.length > 0) {
        event.preventDefault();
        setSlashIndex((value) => (value + 1) % slashCandidates.length);
        return;
      }
      if (event.key === "ArrowUp" && slashCandidates.length > 0) {
        event.preventDefault();
        setSlashIndex((value) => (value - 1 + slashCandidates.length) % slashCandidates.length);
        return;
      }
      if (event.key === "Enter" && !event.shiftKey && slashCandidates.length > 0) {
        event.preventDefault();
        applySlashCandidate((slashCandidates[slashIndex] ?? slashCandidates[0])?.name, editorRef.current);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setSlashPickerDismissed(true);
        setSlashIndex(0);
        return;
      }
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
      logAgent,
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
      onClearRoomMessages: clearRoomMessages,
      inviteActionLabel,
      onInviteAction: handleInviteAction,
      mentionCandidates,
      mentionIndex,
      onApplyMention: applyMention,
      slashCandidates,
      slashIndex,
      slashPickerLoading,
      slashPickerOpen: slashPickerActive,
      onApplySlashCandidate: applySlashCandidate,
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
      threadDraftSegments: activeThreadDraftSegments,
      onOpenThread: openThread,
      onCloseThread: closeThread,
      onThreadDraftChange: (segments: ComposerSegment[]) => {
        if (!activeThreadDraftKey) {
          return;
        }
        setThreadDraftsByKey((current) => updateDrafts(current, activeThreadDraftKey, segments));
        setThreadSlashPickerDismissed(false);
        if (segmentsToPlainText(segments) !== activeThreadDraft) {
          setThreadSlashIndex(0);
        }
      },
      onSendThreadReply: sendThreadReply,
      threadSlashCandidates,
      threadSlashIndex,
      threadSlashPickerLoading: slashPickerLoading,
      threadSlashPickerOpen: threadSlashPickerActive,
      onApplyThreadSlashCandidate: applyThreadSlashCandidate,
      onDismissThreadSlashPicker: () => {
        setThreadSlashPickerDismissed(true);
        setThreadSlashIndex(0);
      },
      onSetThreadSlashIndex: setThreadSlashIndex,
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

function loadSlashSkillOptions(
  agentID: string,
  onInitial: (skills: SlashSkillOption[]) => void,
): Promise<SlashSkillOption[]> {
  const cached = slashSkillOptionsCache.get(agentID);
  if (cached) {
    return Promise.resolve(cached);
  }
  const pending = slashSkillOptionsRequests.get(agentID);
  if (pending) {
    return pending;
  }

  const request = fetchAgentSkills(agentID)
    .then(async (skillsListing) => {
      const skills = skillOptionsFromWorkspace(skillsListing.entries || []);
      slashSkillOptionsCache.set(agentID, skills);
      onInitial(skills);

      const enriched = await Promise.all(
        skills.map(async (skill) => {
          try {
            const file = await fetchAgentSkillsFile(agentID, `${skill.name}/SKILL.md`);
            return {
              ...skill,
              description: skillDescriptionFromMarkdown(file.content || "") || skill.description,
            };
          } catch {
            return skill;
          }
        }),
      );
      slashSkillOptionsCache.set(agentID, enriched);
      return enriched;
    })
    .finally(() => {
      slashSkillOptionsRequests.delete(agentID);
    });

  slashSkillOptionsRequests.set(agentID, request);
  return request;
}

const builtinSlashCommandNames = ["new"];

type SlashPickerStateInput = {
  draftText: string;
  disabled?: boolean;
  enabled: boolean;
  skillOptions: SlashSkillOption[];
};

type SlashPickerState = {
  query: string | null;
  active: boolean;
  candidates: SlashPickerCandidate[];
};

export function buildSlashPickerState(input: SlashPickerStateInput): SlashPickerState {
  const query = slashPickerQueryForDraft(input.draftText);
  const active = Boolean(input.enabled && query !== null && !input.disabled);
  if (!active) {
    return {
      query,
      active: false,
      candidates: [],
    };
  }

  return {
    query,
    active: true,
    candidates: [
      ...builtinSlashCommandNames
        .filter((name) => fuzzySkillMatch(name, query ?? ""))
        .map((name) => ({ description: slashCommandDescription(name), name, type: "command" as const })),
      ...input.skillOptions
        .filter((skill) => !builtinSlashCommandNames.includes(skill.name) && fuzzySkillMatch(skill.name, query ?? ""))
        .map((skill) => ({ description: skill.description, name: skill.name, type: "skill" as const })),
    ],
  };
}

function slashCommandDescription(name: string): string {
  if (name === "new") {
    return "Start a new conversation";
  }
  return "";
}

export function slashPickerQueryForDraft(draftText: string): string | null {
  const trimmed = draftText.trimStart();
  if (!trimmed.startsWith("/")) {
    return null;
  }
  const query = trimmed.slice(1);
  return /\s/.test(query) ? null : query.toLowerCase();
}

export function slashSkillCommandText(skillName: string): string {
  return slashSkillCommandTextWithBody(skillName, "");
}

function slashSkillCommandTextWithBody(skillName: string, body = ""): string {
  const skillArg = escapeXMLAttribute(String(skillName || "").trim());
  const base = `<slash-command name="use-skill" arg="${skillArg}"></slash-command>`;
  const normalizedBody = String(body ?? "").trim();
  if (!normalizedBody) {
    return base;
  }
  return `${base} ${normalizedBody}`;
}

export function slashCommandInputText(skillName: string): string {
  return `/${String(skillName || "").trim()} `;
}

export function normalizeSlashShorthandForPayload(text: string): string {
  const shorthand = parseSlashShorthandToPayload(text);
  if (!shorthand) {
    return text;
  }
  if (shorthand.name === "new") {
    return slashNewConversationCommandTextWithBody(shorthand.body);
  }
  return slashSkillCommandTextWithBody(shorthand.arg, shorthand.body);
}

function slashNewConversationCommandTextWithBody(body = ""): string {
  const base = '<slash-command name="new" arg="conversation"></slash-command>';
  const normalizedBody = String(body ?? "").trim();
  if (!normalizedBody) {
    return base;
  }
  return `${base} ${normalizedBody}`;
}

function parseSlashShorthandToPayload(text: string): { arg: string; body: string; name: "new" | "use-skill" } | null {
  const trimmed = text.trimStart();
  if (!trimmed.startsWith("/") || trimmed.startsWith("//")) {
    return null;
  }

  const afterSlash = trimmed.slice(1).replace(/^\s+/, "");
  if (!afterSlash) {
    return null;
  }
  let arg = afterSlash;
  let body = "";
  for (let index = 0; index < afterSlash.length; index++) {
    if (/\s/u.test(afterSlash[index])) {
      arg = afterSlash.slice(0, index);
      body = afterSlash.slice(index);
      break;
    }
  }
  if (!arg) {
    return null;
  }
  if (arg.toLowerCase() === "new") {
    return {
      arg: "conversation",
      body: body.trim(),
      name: "new",
    };
  }
  if (!isValidSkillSlug(arg)) {
    return null;
  }
  return {
    arg,
    body: body.trim(),
    name: "use-skill",
  };
}

function isValidSkillSlug(value: string): boolean {
  if (!value || value === "." || value === ".." || /[/\\]/u.test(value)) {
    return false;
  }
  return /^[A-Za-z0-9._-]+$/u.test(value);
}

function escapeXMLAttribute(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

function fuzzySkillMatch(name: string, query: string): boolean {
  if (!query) {
    return true;
  }
  const text = name.toLowerCase();
  let offset = 0;
  for (const char of query.toLowerCase()) {
    offset = text.indexOf(char, offset);
    if (offset < 0) {
      return false;
    }
    offset += 1;
  }
  return true;
}
