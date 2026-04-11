import React, { useEffect, useMemo, useRef, useState } from "https://esm.sh/react@18.3.1";
import { createRoot } from "https://esm.sh/react-dom@18.3.1/client";
import htm from "https://esm.sh/htm@3.1.1";
import { marked } from "https://esm.sh/marked@13.0.2";
import DOMPurify from "https://esm.sh/dompurify@3.1.6";
import mermaid from "https://esm.sh/mermaid@11.4.1";

const html = htm.bind(React.createElement);
const LOCALE_STORAGE_KEY = "csgclaw.im.locale";
const TOOL_CALLS_STORAGE_KEY = "csgclaw.im.showToolCalls";
const SIDEBAR_COLLAPSED_STORAGE_KEY = "csgclaw.im.sidebarCollapsed";

marked.setOptions({
  gfm: true,
  breaks: true,
});

mermaid.initialize({
  startOnLoad: false,
  securityLevel: "strict",
  theme: "neutral",
});

const messages = {
  zh: {
    pageTitle: "CSGClaw IM",
    loading: "正在加载 IM 工作区...",
    loadingFailed: "加载失败，请稍后重试。",
    emptyConversation: "请选择一个房间",
    conversationSection: "房间",
    yourView: "你的视图",
    activeNow: "当前在线",
    totalThreads: "房间总数",
    teamMembers: "团队成员",
    membersTitle: "成员",
    conversationOverview: "房间概览",
    sendFailed: "消息发送失败，请重试。",
    roomCreatedToast: "Room 已创建",
    inviteSentToast: "邀请已发送",
    noMessages: "还没有消息，发一条开始吧。",
    noVisibleMessages: "工具调用已隐藏，当前没有可显示的消息。",
    createRoom: "创建房间",
    deleteRoom: "删除房间",
    conversationLabel: "房间",
    participants: "成员",
    mentionBadge: "@ 提及",
    inviteMembers: "邀请成员",
    inputPlaceholder: "输入消息，使用 @ 选择成员",
    send: "发送",
    composerTip: "Enter 发送，Shift + Enter 换行。支持多人房间、双人房间和 @ 提及。",
    createRoomTitle: "创建房间",
    createRoomSubtitle: "为一个新主题建立房间，并预先邀请成员。",
    close: "关闭",
    roomName: "标题",
    roomNamePlaceholder: "例如：Launch War Room",
    roomDescription: "说明",
    roomDescriptionPlaceholder: "简单说明这个房间的用途",
    initialMembers: "初始成员",
    cancel: "取消",
    create: "创建",
    inviteTitle: "添加成员",
    inviteSubtitle: "将更多成员加入当前房间。",
    inviteCandidates: "可选成员",
    noInviteCandidates: "当前没有可新增的成员。",
    sendInvite: "发送邀请",
    languageSwitcher: "切换语言",
    languageOptionZh: "简体中文",
    languageOptionEn: "English",
    toggleToolCallsShow: "显示工具调用",
    toggleToolCallsHide: "隐藏工具调用",
    collapseSidebar: "收起侧边栏",
    expandSidebar: "展开侧边栏",
    online: "在线",
    offline: "离线",
    justNow: "刚刚",
    minutesAgo: "{count} 分钟前",
    roles: {
      admin: "管理员",
      manager: "经理",
      worker: "成员",
    },
    errors: {
      "title is required": "标题不能为空",
      "creator_id is required": "缺少创建者",
      "creator not found": "创建者不存在",
      "user not found": "用户不存在",
      "room_id is required": "缺少房间 ID",
      "room not found": "房间不存在",
      "inviter_id is required": "缺少邀请者",
      "inviter not found": "邀请者不存在",
      "inviter is not a room member": "邀请者不在当前房间中",
      "user_ids is required": "请选择至少一位成员",
      "no new users to invite": "没有可新增的成员",
    },
  },
  en: {
    pageTitle: "CSGClaw IM",
    loading: "Loading IM workspace...",
    loadingFailed: "Failed to load the workspace. Please try again.",
    emptyConversation: "Select a room",
    conversationSection: "Rooms",
    yourView: "Your view",
    activeNow: "Active now",
    totalThreads: "Threads",
    teamMembers: "Members",
    membersTitle: "Members",
    conversationOverview: "Room overview",
    sendFailed: "Failed to send the message. Please retry.",
    roomCreatedToast: "Room created",
    inviteSentToast: "Invite sent",
    noMessages: "No messages yet. Start this room.",
    noVisibleMessages: "Tool calls are hidden, and there are no visible messages in this room.",
    createRoom: "New Room",
    deleteRoom: "Delete Room",
    conversationLabel: "Room",
    participants: "participants",
    mentionBadge: "@ mention",
    inviteMembers: "Invite Members",
    inputPlaceholder: "Type a message and use @ to mention members",
    send: "Send",
    composerTip: "Press Enter to send and Shift + Enter for a new line. Supports group chats, 1:1 rooms, and @ mentions.",
    createRoomTitle: "New Room",
    createRoomSubtitle: "Create a new room and invite members in advance.",
    close: "Close",
    roomName: "Title",
    roomNamePlaceholder: "For example: Launch War Room",
    roomDescription: "Details",
    roomDescriptionPlaceholder: "Briefly describe what this room is for",
    initialMembers: "Initial Members",
    cancel: "Cancel",
    create: "Create",
    inviteTitle: "Add Members",
    inviteSubtitle: "Add more members to the current room.",
    inviteCandidates: "Available Members",
    noInviteCandidates: "There are no additional members to invite.",
    sendInvite: "Send Invite",
    languageSwitcher: "Switch language",
    languageOptionZh: "简体中文",
    languageOptionEn: "English",
    toggleToolCallsShow: "Show tool calls",
    toggleToolCallsHide: "Hide tool calls",
    collapseSidebar: "Collapse sidebar",
    expandSidebar: "Expand sidebar",
    online: "online",
    offline: "offline",
    justNow: "just now",
    minutesAgo: "{count} min ago",
    roles: {
      admin: "admin",
      manager: "manager",
      worker: "worker",
    },
    errors: {
      "title is required": "Title is required",
      "creator_id is required": "Creator is required",
      "creator not found": "Creator not found",
      "user not found": "User not found",
      "room_id is required": "Room ID is required",
      "room not found": "Room not found",
      "inviter_id is required": "Inviter is required",
      "inviter not found": "Inviter not found",
      "inviter is not a room member": "Inviter is not a room member",
      "user_ids is required": "Select at least one member",
      "no new users to invite": "There are no new users to invite",
    },
  },
};

function GlobeIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M12 3.25a8.75 8.75 0 1 0 0 17.5a8.75 8.75 0 0 0 0-17.5Zm5.99 7.97h-2.56a14.57 14.57 0 0 0-1.13-4.01a7.28 7.28 0 0 1 3.69 4.01Zm-5.24-4.47c.52.76 1.16 2.28 1.51 4.47h-4.52c.35-2.19.99-3.71 1.51-4.47c.22-.32.42-.5.5-.5s.28.18.5.5Zm-4.05.46a14.57 14.57 0 0 0-1.13 4.01H4.01A7.28 7.28 0 0 1 7.7 7.21Zm-4.19 5.51h2.81c.03 1.48.24 2.88.57 4.01H5.37a7.22 7.22 0 0 1-.86-4.01Zm3.89 0h4.72c-.04 1.4-.24 2.79-.62 4.01H9.02a17.18 17.18 0 0 1-.62-4.01Zm.87 5.51h3.46c-.27.69-.59 1.3-.95 1.83c-.29.42-.54.69-.68.69s-.39-.27-.68-.69a9.65 9.65 0 0 1-.95-1.83Zm4.95-1.5c.33-1.13.54-2.53.57-4.01h2.81a7.22 7.22 0 0 1-.86 4.01h-2.52Z"
        fill="currentColor"
      />
    </svg>
  `;
}

function MessageContent({ content }) {
  const containerRef = useRef(null);
  const structured = useMemo(() => parseStructuredMessage(content), [content]);
  const markup = useMemo(() => renderMarkdown(content), [content]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }

    const mermaidBlocks = container.querySelectorAll("pre > code.language-mermaid");
    mermaidBlocks.forEach((code, index) => {
      const pre = code.parentElement;
      if (!pre || pre.dataset.enhanced === "true") {
        return;
      }
      const wrapper = document.createElement("div");
      wrapper.className = "mermaid";
      wrapper.textContent = code.textContent ?? "";
      wrapper.dataset.blockId = `${Date.now()}-${index}`;
      pre.replaceWith(wrapper);
    });

    const diagrams = container.querySelectorAll(".mermaid");
    if (diagrams.length > 0) {
      mermaid.run({ nodes: diagrams });
    }
  }, [markup]);

  if (structured) {
    return html`<${StructuredMessageCard} data=${structured} />`;
  }

  return html`<div ref=${containerRef} className="message-content" dangerouslySetInnerHTML=${{ __html: markup }} />`;
}

function StructuredMessageCard({ data }) {
  return html`
    <div className="structured-message">
      <div className="structured-message-header">
        <div>
          <div className="structured-message-title">${data.title}</div>
          ${data.subtitle ? html`<div className="structured-message-subtitle">${data.subtitle}</div>` : null}
        </div>
        ${data.badge ? html`<span className="structured-message-badge">${data.badge}</span>` : null}
      </div>
      ${data.summary ? html`<div className="structured-message-summary">${data.summary}</div>` : null}
      ${data.code
        ? html`
            <details className="structured-message-details">
              <summary>${data.codeSummary}</summary>
              <pre className="structured-message-code"><code>${data.code}</code></pre>
            </details>
          `
        : null}
      ${data.payload
        ? html`
            <details className="structured-message-details">
              <summary>${data.payloadSummary}</summary>
              <pre className="structured-message-json"><code>${data.payload}</code></pre>
            </details>
          `
        : null}
    </div>
  `;
}

function AddUserIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M15 19c0-2.761-2.239-5-5-5s-5 2.239-5 5"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.8"
      />
      <circle
        cx="10"
        cy="7.5"
        r="3.5"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
      />
      <path
        d="M18 8v6M15 11h6"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-width="1.8"
      />
    </svg>
  `;
}

function UsersIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M9 11a4 4 0 1 0 0-8a4 4 0 0 0 0 8Zm7 1a3 3 0 1 0 0-6a3 3 0 0 0 0 6Zm-7 2c-3.314 0-6 1.79-6 4c0 .552.448 1 1 1h10a1 1 0 0 0 1-1c0-2.21-2.686-4-6-4Zm7 1c-.758 0-1.483.11-2.147.312c1.16.87 1.956 2.035 2.118 3.358A1 1 0 0 0 16.964 19H20a1 1 0 0 0 1-1c0-1.657-2.239-3-5-3Z"
        fill="currentColor"
      />
    </svg>
  `;
}

function WrenchIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M14.71 6.29a4 4 0 0 0-5.32 5.94l-4.1 4.1a1.5 1.5 0 1 0 2.12 2.12l4.1-4.1a4 4 0 0 0 5.94-5.32l-2.24 2.24a1 1 0 0 1-1.42 0l-1.38-1.38a1 1 0 0 1 0-1.42Z"
        fill="currentColor"
      />
    </svg>
  `;
}

function SidebarToggleIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <rect
        x="3.75"
        y="5.25"
        width="16.5"
        height="13.5"
        rx="2.25"
        fill="none"
        stroke="currentColor"
        stroke-width="1.6"
      />
      <path d="M8.5 5.75v12.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
    </svg>
  `;
}

function RoomPlusIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="10" fill="#e6ebf3" />
      <path d="M12 7.5v9M7.5 12h9" fill="none" stroke="#586274" stroke-linecap="round" stroke-width="1.9" />
    </svg>
  `;
}

function TrashIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M9.5 4.75h5a1.5 1.5 0 0 1 1.5 1.5v.5h3"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-width="1.8"
      />
      <path
        d="M5 6.75h14"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-width="1.8"
      />
      <path
        d="M8 9.5v6.75M12 9.5v6.75M16 9.5v6.75"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-width="1.8"
      />
      <path
        d="M7.25 6.75l.63 10.11A2 2 0 0 0 9.87 18.75h4.26a2 2 0 0 0 1.99-1.89L16.75 6.75"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.8"
      />
    </svg>
  `;
}

function RoomsIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="10" fill="#e6ebf3" />
      <path
        d="M9.25 7.75c-2.35 0-4.25 1.64-4.25 3.67c0 1.01.47 1.93 1.23 2.59L5.5 16.25l2.91-.46c.27.04.55.05.84.05c2.35 0 4.25-1.64 4.25-3.67S11.6 7.75 9.25 7.75Zm5.3 2.92c2.04.21 3.65 1.65 3.65 3.42c0 .88-.4 1.69-1.08 2.29l.58 1.88l-2.35-.43c-.25.03-.52.04-.8.04c-1.75 0-3.25-.78-4-1.95"
        fill="none"
        stroke="#1f2937"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.7"
      />
    </svg>
  `;
}

function App() {
  const [locale, setLocale] = useState(() => detectInitialLocale());
  const [showToolCalls, setShowToolCalls] = useState(() => {
    const value = window.localStorage.getItem(TOOL_CALLS_STORAGE_KEY);
    return value === "true";
  });
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(() => {
    const value = window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY);
    return value === "true";
  });
  const [data, setData] = useState(null);
  const [activeConversationId, setActiveConversationId] = useState("");
  const [draft, setDraft] = useState("");
  const [composerSelectionStart, setComposerSelectionStart] = useState(0);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [showCreateRoom, setShowCreateRoom] = useState(false);
  const [showInvite, setShowInvite] = useState(false);
  const [showMemberList, setShowMemberList] = useState(false);
  const [roomTitle, setRoomTitle] = useState("");
  const [roomDescription, setRoomDescription] = useState("");
  const [roomMemberIDs, setRoomMemberIDs] = useState([]);
  const [inviteUserIDs, setInviteUserIDs] = useState([]);
  const [submitError, setSubmitError] = useState("");
  const [composerError, setComposerError] = useState("");
  const [loadingError, setLoadingError] = useState("");
  const textareaRef = useRef(null);
  const composerHighlightRef = useRef(null);
  const messageListRef = useRef(null);

  useEffect(() => {
    fetch("/api/v1/bootstrap")
      .then((resp) => resp.json())
      .then((payload) => {
        setData(normalizeIMData(payload));
        setLoadingError("");
        setInviteUserIDs([]);
        if (payload.rooms.length > 0) {
          setActiveConversationId(payload.rooms[0].id);
        }
      })
      .catch(() => setLoadingError(messages[locale].loadingFailed));
  }, []);

  useEffect(() => {
    const source = new EventSource("/api/v1/events");

    source.onmessage = (event) => {
      const payload = JSON.parse(event.data);
      setData((current) => applyIMEvent(current, payload));
    };

    return () => source.close();
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    document.title = messages[locale].pageTitle;
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  }, [locale]);

  useEffect(() => {
    window.localStorage.setItem(TOOL_CALLS_STORAGE_KEY, String(showToolCalls));
  }, [showToolCalls]);

  useEffect(() => {
    window.localStorage.setItem(SIDEBAR_COLLAPSED_STORAGE_KEY, String(isSidebarCollapsed));
  }, [isSidebarCollapsed]);

  const t = useMemo(() => createTranslator(locale), [locale]);

  const usersById = useMemo(() => {
    const result = new Map();
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
    if (showToolCalls) {
      return activeConversation.messages;
    }
    return activeConversation.messages.filter((message) => !isToolCallMessage(message.content));
  }, [activeConversation, showToolCalls]);

  const rooms = useMemo(
    () => data?.rooms ?? [],
    [data],
  );
  const roomCount = rooms.length;

  const mentionState = useMemo(() => getMentionState(draft, composerSelectionStart), [draft, composerSelectionStart]);
  const mentionCandidates = useMemo(() => {
    if (!data || !mentionState) {
      return [];
    }
    const allowed = new Set(activeConversation?.participants ?? []);
    return data.users
      .filter((user) => allowed.has(user.id))
      .filter((user) => user.handle.toLowerCase().includes(mentionState.query.toLowerCase()) || user.name.toLowerCase().includes(mentionState.query.toLowerCase()))
      .slice(0, 5);
  }, [data, activeConversation, mentionState]);

  useEffect(() => {
    setMentionIndex(0);
  }, [activeConversationId, draft]);

  useEffect(() => {
    setComposerSelectionStart(0);
  }, [activeConversationId]);

  useEffect(() => {
    if (!showCreateRoom) {
      setRoomTitle("");
      setRoomDescription("");
      setRoomMemberIDs([]);
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
  }, [activeConversationId]);

  useEffect(() => {
    const el = textareaRef.current;
    if (!el) {
      return;
    }
    el.style.height = "0px";
    el.style.height = `${Math.min(el.scrollHeight, 168)}px`;
    syncComposerScroll(el);
  }, [draft]);

  useEffect(() => {
    const el = messageListRef.current;
    if (!el) {
      return;
    }
    el.scrollTop = el.scrollHeight;
  }, [activeConversationId, visibleMessages.length]);

  async function sendMessage() {
    if (!data || !activeConversation || !draft.trim()) {
      return;
    }

    setComposerError("");
    const resp = await fetch("/api/v1/messages", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        room_id: activeConversation.id,
        sender_id: data.current_user_id,
        content: draft,
      }),
    });
    if (!resp.ok) {
      setComposerError(t("sendFailed"));
      return;
    }
    const created = await resp.json();
    setData((current) => appendMessageToData(current, activeConversation.id, created));
    setDraft("");
  }

  async function createRoom() {
    if (!data || !roomTitle.trim()) {
      return;
    }

    setSubmitError("");
    const resp = await fetch("/api/v1/rooms", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        title: roomTitle,
        description: roomDescription,
        creator_id: data.current_user_id,
        participant_ids: roomMemberIDs,
        locale,
      }),
    });
    if (!resp.ok) {
      setSubmitError(localizeError(await resp.text(), t));
      return;
    }

    const created = await resp.json();
    setData((current) => upsertConversationInData(current, created));
    setActiveConversationId(created.id);
    setComposerError("");
    setShowCreateRoom(false);
  }

  async function inviteUsers() {
    if (!data || !activeConversation || inviteUserIDs.length === 0) {
      return;
    }

    setSubmitError("");
    const resp = await fetch("/api/v1/rooms/invite", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        room_id: activeConversation.id,
        inviter_id: data.current_user_id,
        user_ids: inviteUserIDs,
        locale,
      }),
    });
    if (!resp.ok) {
      setSubmitError(localizeError(await resp.text(), t));
      return;
    }

    const updated = await resp.json();
    setData((current) => upsertConversationInData(current, updated));
    setComposerError("");
    setShowInvite(false);
  }

  async function deleteRoom(roomID) {
    if (!data || !roomID) {
      return;
    }

    const resp = await fetch(`/api/v1/rooms/${roomID}`, {
      method: "DELETE",
    });
    if (!resp.ok) {
      setComposerError(localizeError(await resp.text(), t));
      return;
    }

    const remainingRooms = rooms.filter((item) => item.id !== roomID);
    setData((current) => removeConversationFromData(current, roomID));
    setComposerError("");
    setSubmitError("");
    if (activeConversationId === roomID) {
      setActiveConversationId(remainingRooms[0]?.id ?? "");
    }
  }

  function applyMention(user) {
    const state = getMentionState(draft, composerSelectionStart);
    if (!state) {
      return;
    }
    const next = `${draft.slice(0, state.start)}@${user.handle} ${draft.slice(state.end)}`;
    const pos = state.start + user.handle.length + 2;
    setDraft(next);
    setComposerSelectionStart(pos);
    requestAnimationFrame(() => {
      const el = textareaRef.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(pos, pos);
    });
  }

  function syncComposerSelection(target) {
    setComposerSelectionStart(target.selectionStart ?? target.value.length);
  }

  function syncComposerScroll(target) {
    const highlight = composerHighlightRef.current;
    if (!highlight || !target) {
      return;
    }
    highlight.scrollTop = target.scrollTop;
    highlight.scrollLeft = target.scrollLeft;
  }

  function onComposerKeyDown(event) {
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

    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      sendMessage();
    }
  }

  if (!data) {
    return html`<div className="empty-state">${loadingError || t("loading")}</div>`;
  }

  const createRoomCandidates = data.users.filter((user) => user.id !== data.current_user_id);
  const inviteCandidates = activeConversation
    ? data.users.filter((user) => !activeConversation.participants.includes(user.id))
    : [];
  const activeConversationMembers = activeConversation
    ? activeConversation.participants.map((id) => usersById.get(id)).filter(Boolean)
    : [];
  return html`
    <${React.Fragment}>
      <div className=${`app-shell ${isSidebarCollapsed ? "sidebar-collapsed" : ""}`}>
        <aside className=${`sidebar ${isSidebarCollapsed ? "collapsed" : ""}`}>
          <div className="sidebar-header">
            <div className="sidebar-brand-row">
              <div className="sidebar-brand">CSGClaw</div>
              <div className="language-switch sidebar-language-switch" role="group" aria-label=${t("languageSwitcher")}>
                <span className="language-switch-icon" aria-hidden="true"><${GlobeIcon} /></span>
                <div className=${`language-switch-track ${locale === "en" ? "is-en" : "is-zh"}`}>
                  <span className="language-switch-thumb" aria-hidden="true"></span>
                  <button
                    className=${`language-toggle ${locale === "zh" ? "active" : ""}`}
                    aria-pressed=${locale === "zh"}
                    title=${t("languageOptionZh")}
                    onClick=${() => setLocale("zh")}
                  >
                    中
                  </button>
                  <button
                    className=${`language-toggle ${locale === "en" ? "active" : ""}`}
                    aria-pressed=${locale === "en"}
                    title=${t("languageOptionEn")}
                    onClick=${() => setLocale("en")}
                  >
                    EN
                  </button>
                </div>
              </div>
              <button
                className="sidebar-toggle-button"
                aria-label=${t("collapseSidebar")}
                aria-pressed=${false}
                title=${t("collapseSidebar")}
                onClick=${() => setIsSidebarCollapsed(true)}
              >
                <span className="sidebar-toggle-mark"><${SidebarToggleIcon} /></span>
              </button>
            </div>
            <div className="sidebar-header-row">
              <nav className="sidebar-nav" aria-label=${t("conversationSection")}>
                <button
                  className="sidebar-nav-button"
                  aria-label=${t("createRoom")}
                  title=${t("createRoom")}
                  onClick=${() => setShowCreateRoom(true)}
                >
                  <span className="sidebar-nav-icon" aria-hidden="true"><${RoomPlusIcon} /></span>
                  <span className="sidebar-nav-label">${t("createRoom")}</span>
                </button>
                <button
                  className="sidebar-nav-button active"
                  aria-current="page"
                  aria-label=${`${t("conversationSection")} (${roomCount})`}
                  title=${t("conversationSection")}
                  onClick=${() => setIsSidebarCollapsed(false)}
                >
                  <span className="sidebar-nav-icon" aria-hidden="true"><${RoomsIcon} /></span>
                  <span className="sidebar-nav-label">${t("conversationSection")}</span>
                  <span className="sidebar-nav-count" aria-hidden="true">${roomCount}</span>
                </button>
              </nav>
            </div>
          </div>
          <div className="conversation-list">
            <${ConversationSection}
              title=${t("conversationSection")}
              items=${rooms}
              activeConversationId=${activeConversationId}
              currentUserID=${data.current_user_id}
              usersById=${usersById}
              locale=${locale}
              t=${t}
              onSelect=${setActiveConversationId}
              onDelete=${deleteRoom}
            />
          </div>
        </aside>

        ${isSidebarCollapsed
          ? html`
              <div className="sidebar-rail">
                <button
                  className="sidebar-expand-button"
                  aria-label=${t("expandSidebar")}
                  aria-pressed=${true}
                  title=${t("expandSidebar")}
                  onClick=${() => setIsSidebarCollapsed(false)}
                >
                  <span className="sidebar-toggle-mark"><${SidebarToggleIcon} /></span>
                </button>
                <nav className="sidebar-rail-nav" aria-label=${t("conversationSection")}>
                  <button
                    className="sidebar-rail-button"
                    aria-label=${t("createRoom")}
                    title=${t("createRoom")}
                    onClick=${() => setShowCreateRoom(true)}
                  >
                    <span className="sidebar-rail-icon" aria-hidden="true"><${RoomPlusIcon} /></span>
                  </button>
                  <button
                    className="sidebar-rail-button active"
                    aria-current="page"
                    aria-label=${t("conversationSection")}
                    title=${t("conversationSection")}
                    onClick=${() => setIsSidebarCollapsed(false)}
                  >
                    <span className="sidebar-rail-icon" aria-hidden="true"><${RoomsIcon} /></span>
                  </button>
                </nav>
              </div>
            `
          : null}

        <main className="chat-panel">
          ${activeConversation
            ? html`
                <header className="chat-header">
                  <div className="chat-header-main">
                    <div className="chat-title-bar">
                      <div className="chat-title-row">
                        <div className="chat-title-group">
                          <div className="chat-title truncate">${activeConversation.title}</div>
                          <div className="header-menu">
                            <button
                              className=${`member-badge-button ${showMemberList ? "active" : ""}`}
                              aria-label=${t("membersTitle")}
                              aria-pressed=${showMemberList}
                              title=${t("membersTitle")}
                              onClick=${() => setShowMemberList((value) => !value)}
                            >
                              <span className="icon-button-mark" aria-hidden="true"><${UsersIcon} /></span>
                              <span className="member-badge-count">${activeConversationMembers.length}</span>
                            </button>
                            ${showMemberList
                              ? html`
                                  <div className="header-popover members-popover">
                                    <div className="header-popover-title">${t("membersTitle")}</div>
                                    <div className="members-popover-list">
                                      ${activeConversationMembers.map((user) => html`
                                        <div key=${user.id} className="member-row">
                                          <div className="avatar" style=${{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}>${user.avatar}</div>
                                          <div className="member-row-main">
                                            <div className="member-row-name">${user.name}</div>
                                            <div className="member-row-meta">@${user.handle} · ${localizeRole(user.role, t)}</div>
                                          </div>
                                        </div>
                                      `)}
                                    </div>
                                  </div>
                                `
                              : null}
                          </div>
                        </div>
                      </div>
                      <div className="chat-title-actions">
                        <button
                          className=${`icon-button ${showToolCalls ? "active" : ""}`}
                          aria-label=${showToolCalls ? t("toggleToolCallsHide") : t("toggleToolCallsShow")}
                          aria-pressed=${showToolCalls}
                          title=${showToolCalls ? t("toggleToolCallsHide") : t("toggleToolCallsShow")}
                          onClick=${() => setShowToolCalls((value) => !value)}
                        >
                          <span className="icon-button-mark"><${WrenchIcon} /></span>
                        </button>
                        <button
                          className="icon-button"
                          aria-label=${t("inviteMembers")}
                          title=${t("inviteMembers")}
                          onClick=${() => setShowInvite(true)}
                        >
                          <span className="icon-button-mark"><${AddUserIcon} /></span>
                        </button>
                      </div>
                    </div>
                    ${getConversationDescription(activeConversation, data.current_user_id, usersById, locale, t)
                      ? html`<div className="chat-subtitle">${getConversationDescription(activeConversation, data.current_user_id, usersById, locale, t)}</div>`
                      : null}
                  </div>
                </header>

                <section ref=${messageListRef} className="messages">
                  ${activeConversation.messages.length === 0
                    ? html`<div className="messages-empty">${t("noMessages")}</div>`
                    : visibleMessages.length === 0
                      ? html`<div className="messages-empty">${t("noVisibleMessages")}</div>`
                      : null}
                  ${visibleMessages.map((message) => {
                    if (isEventMessage(message)) {
                      return html`
                        <div key=${message.id} className="message-event-row">
                          <div className="message-event-text">${formatEventMessage(message, usersById, locale)}</div>
                        </div>
                      `;
                    }
                    const user = usersById.get(message.sender_id);
                    const own = message.sender_id === data.current_user_id;
                    return html`
                      <div key=${message.id} className=${`message-row ${own ? "own" : ""}`}>
                        <div className="avatar" style=${{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}>${user.avatar}</div>
                        <div className="message-card">
                          <div className="message-meta">
                            <span className="message-author">${user.name}</span>
                            <span>${formatTime(message.created_at, locale)}</span>
                          </div>
                          <div className="message-bubble"><${MessageContent} content=${message.content} /></div>
                        </div>
                      </div>
                    `;
                  })}
                </section>

                <footer className="composer">
                  ${mentionCandidates.length > 0
                    ? html`
                        <div className="mention-picker">
                          ${mentionCandidates.map((user, index) => html`
                            <button
                              key=${user.id}
                              className=${`mention-option ${index === mentionIndex ? "active" : ""}`}
                              onMouseDown=${(event) => {
                                event.preventDefault();
                                applyMention(user);
                              }}
                            >
                              <div className="avatar" style=${{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}>${user.avatar}</div>
                              <div>
                                <div className="message-author">${user.name}</div>
                                <div className="conversation-preview">@${user.handle} · ${localizeRole(user.role, t)}</div>
                              </div>
                            </button>
                          `)}
                        </div>
                      `
                    : null}
                  <div className="composer-box">
                    <div className="composer-input-wrap">
                      <div ref=${composerHighlightRef} className="composer-highlight" aria-hidden="true">
                        ${renderComposerHighlight(draft)}
                      </div>
                      <textarea
                        ref=${textareaRef}
                        value=${draft}
                        placeholder=${t("inputPlaceholder")}
                        onInput=${(event) => {
                          setDraft(event.target.value);
                          syncComposerSelection(event.target);
                          syncComposerScroll(event.target);
                        }}
                        onClick=${(event) => syncComposerSelection(event.target)}
                        onKeyDown=${onComposerKeyDown}
                        onKeyUp=${(event) => syncComposerSelection(event.target)}
                        onScroll=${(event) => syncComposerScroll(event.target)}
                        onSelect=${(event) => syncComposerSelection(event.target)}
                      />
                      <button
                        type="button"
                        className="composer-send-button"
                        aria-label=${t("send")}
                        title=${t("send")}
                        disabled=${!draft.trim()}
                        onClick=${sendMessage}
                      >
                        <span className="composer-send-main" aria-hidden="true">
                          <svg viewBox="0 0 24 24" focusable="false">
                            <path
                              d="M 4.22 3.12 L 19.78 10.88 Q 22 12 19.78 13.12 L 4.22 20.88 Q 2 22 2 19.5 L 2 16.5 Q 2 14 4.4 13.32 L 7.56 12.41 Q 9 12 7.56 11.59 L 4.4 10.67 Q 2 10 2 7.5 L 2 4.5 Q 2 2 4.22 3.12 Z"
                            />
                          </svg>
                        </span>
                      </button>
                    </div>
                  </div>
                  ${composerError ? html`<div className="form-error composer-error">${composerError}</div>` : null}
                  <div className="composer-tip">${t("composerTip")}</div>
                </footer>
              `
            : html`<div className="empty-state">${t("emptyConversation")}</div>`}
        </main>
      </div>

      ${showCreateRoom
        ? html`
            <div className="modal-backdrop" onClick=${() => setShowCreateRoom(false)}>
              <div className="modal-card" onClick=${(event) => event.stopPropagation()}>
                <div className="modal-header">
                  <div>
                    <div className="modal-title">${t("createRoomTitle")}</div>
                    <div className="modal-subtitle">${t("createRoomSubtitle")}</div>
                  </div>
                  <button className="modal-close" onClick=${() => setShowCreateRoom(false)}>${t("close")}</button>
                </div>
                <label className="field">
                  <span>${t("roomName")}</span>
                  <input value=${roomTitle} onInput=${(event) => setRoomTitle(event.target.value)} placeholder=${t("roomNamePlaceholder")} />
                </label>
                <label className="field">
                  <span>${t("roomDescription")}</span>
                  <textarea value=${roomDescription} onInput=${(event) => setRoomDescription(event.target.value)} placeholder=${t("roomDescriptionPlaceholder")} />
                </label>
                <div className="field">
                  <span>${t("initialMembers")}</span>
                  <div className="selection-list">
                    ${createRoomCandidates.map((user) => html`
                      <label key=${user.id} className="selection-item">
                        <input
                          type="checkbox"
                          checked=${roomMemberIDs.includes(user.id)}
                          onChange=${() => setRoomMemberIDs((current) => toggleSelection(current, user.id))}
                        />
                        <span>${user.name}</span>
                        <small>@${user.handle}</small>
                      </label>
                    `)}
                  </div>
                </div>
                ${submitError ? html`<div className="form-error">${submitError}</div>` : null}
                <div className="modal-actions">
                  <button className="secondary-button" onClick=${() => setShowCreateRoom(false)}>${t("cancel")}</button>
                  <button className="send-button" disabled=${!roomTitle.trim()} onClick=${createRoom}>${t("create")}</button>
                </div>
              </div>
            </div>
          `
        : null}

      ${showInvite
        ? html`
            <div className="modal-backdrop" onClick=${() => setShowInvite(false)}>
              <div className="modal-card" onClick=${(event) => event.stopPropagation()}>
                <div className="modal-header">
                  <div>
                    <div className="modal-title">${t("inviteTitle")}</div>
                    <div className="modal-subtitle">${t("inviteSubtitle")}</div>
                  </div>
                  <button className="modal-close" onClick=${() => setShowInvite(false)}>${t("close")}</button>
                </div>
                <div className="field">
                  <span>${t("inviteCandidates")}</span>
                  <div className="selection-list">
                    ${inviteCandidates.length > 0
                      ? inviteCandidates.map((user) => html`
                          <label key=${user.id} className="selection-item">
                            <input
                              type="checkbox"
                              checked=${inviteUserIDs.includes(user.id)}
                              onChange=${() => setInviteUserIDs((current) => toggleSelection(current, user.id))}
                            />
                            <span>${user.name}</span>
                            <small>@${user.handle}</small>
                          </label>
                        `)
                      : html`<div className="selection-empty">${t("noInviteCandidates")}</div>`}
                  </div>
                </div>
                ${submitError ? html`<div className="form-error">${submitError}</div>` : null}
                <div className="modal-actions">
                  <button className="secondary-button" onClick=${() => setShowInvite(false)}>${t("cancel")}</button>
                  <button className="send-button" disabled=${inviteUserIDs.length === 0} onClick=${inviteUsers}>${t("sendInvite")}</button>
                </div>
              </div>
            </div>
          `
        : null}
    <//>
  `;
}

function ConversationSection({ title, items, activeConversationId, currentUserID, usersById, locale, t, onSelect, onDelete }) {
  if (!items.length) {
    return null;
  }

  return html`
    <section className="conversation-section">
      ${items.map((conversation) => {
        const lastMessage = conversation.messages[conversation.messages.length - 1];
        const displayUser = resolveConversationUser(conversation, currentUserID, usersById);
        const avatar = isTwoPersonConversation(conversation) && displayUser
          ? displayUser.avatar
          : conversation.title.slice(0, 2).toUpperCase();
        const color = isTwoPersonConversation(conversation) && displayUser
          ? displayUser.accent_hex
          : "#2563eb";
        return html`
          <div
            key=${conversation.id}
            className=${`conversation-item ${conversation.id === activeConversationId ? "active" : ""}`}
          >
            <button
              className="conversation-item-main"
              onClick=${() => onSelect(conversation.id)}
            >
              <div className="avatar" style=${{ background: `linear-gradient(135deg, ${color}, #10233f)` }}>${avatar}</div>
              <div className="conversation-main">
                <div className="conversation-head">
                  <div className="conversation-name truncate">${conversation.title}</div>
                  <div className="section-label">${formatTime(lastMessage?.created_at, locale)}</div>
                </div>
                <div className="conversation-preview truncate">
                  ${formatConversationPreview(lastMessage, conversation, currentUserID, usersById, locale, t)}
                </div>
              </div>
            </button>
            <button
              className="conversation-delete-button"
              aria-label=${`${t("deleteRoom")} ${conversation.title}`}
              title=${`${t("deleteRoom")} ${conversation.title}`}
              onClick=${(event) => {
                event.stopPropagation();
                onDelete(conversation.id);
              }}
            >
              <span className="conversation-delete-icon" aria-hidden="true"><${TrashIcon} /></span>
            </button>
          </div>
        `;
      })}
    </section>
  `;
}

function detectInitialLocale() {
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (stored === "zh" || stored === "en") {
    return stored;
  }
  return navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en";
}

function createTranslator(locale) {
  return (key, params = {}) => {
    const value = resolveTranslation(locale, key);
    if (typeof value !== "string") {
      return key;
    }
    return value.replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
  };
}

function resolveTranslation(locale, key) {
  return key.split(".").reduce((current, part) => current?.[part], messages[locale]);
}

function localizeRole(role, t) {
  return t(`roles.${role}`) === `roles.${role}` ? role : t(`roles.${role}`);
}

function localizeError(raw, t) {
  const cleaned = raw.trim();
  for (const key of Object.keys(messages.zh.errors)) {
    if (cleaned.includes(key)) {
      return t(`errors.${key}`);
    }
    const englishValue = messages.en.errors[key];
    if (englishValue && cleaned.includes(englishValue)) {
      return t(`errors.${key}`);
    }
    const prefix = `${key}:`;
    if (cleaned.startsWith(prefix)) {
      const suffix = cleaned.slice(prefix.length).trim();
      return `${t(`errors.${key}`)} ${suffix}`;
    }
  }
  return cleaned;
}

function getMentionState(text, selection) {
  const cursor = typeof selection === "number"
    ? selection
    : selection?.selectionStart ?? text.length;
  const before = text.slice(0, cursor);
  const match = before.match(/(^|\s)@([a-zA-Z0-9._-]*)$/);
  if (!match) return null;
  return {
    query: match[2],
    start: cursor - match[2].length - 1,
    end: cursor,
  };
}

function renderComposerHighlight(text) {
  if (!text) {
    return "\u00A0";
  }

  const parts = [];
  const mentionPattern = /(^|\s)(@[a-zA-Z0-9._-]+)/g;
  let cursor = 0;
  let match = mentionPattern.exec(text);

  while (match) {
    const prefix = match[1];
    const mention = match[2];
    const start = match.index;
    const mentionStart = start + prefix.length;

    if (cursor < start) {
      parts.push(text.slice(cursor, start));
    }
    if (prefix) {
      parts.push(prefix);
    }
    parts.push(html`<span key=${`${mentionStart}-${mention}`} className="composer-mention">${mention}</span>`);
    cursor = mentionStart + mention.length;
    match = mentionPattern.exec(text);
  }

  if (cursor < text.length) {
    parts.push(text.slice(cursor));
  }

  return parts;
}

function isToolCallMessage(content) {
  return (content ?? "").trimStart().startsWith("🔧 ");
}

function isEventMessage(message) {
  if (message?.kind === "event") {
    return true;
  }
  return isLegacySystemEventContent(message?.content);
}

function formatConversationPreview(message, conversation, currentUserID, usersById, locale, t) {
  if (message) {
    if (isEventMessage(message)) {
      return formatEventMessage(message, usersById, locale);
    }
    return message.content;
  }
  return getConversationSubtitle(conversation, currentUserID, usersById, locale, t);
}

function formatEventMessage(message, usersById, locale) {
  if (!message) {
    return "";
  }
  if (message.event?.key === "room_created") {
    const actor = userDisplayName(message.event.actor_id || message.sender_id, usersById);
    const title = message.event.title || message.content || "";
    return locale === "zh" ? `${actor} 创建了房间“${title}”` : `${actor} created the room "${title}"`;
  }
  if (message.event?.key === "room_members_added") {
    const actor = userDisplayName(message.event.actor_id || message.sender_id, usersById);
    const targets = (message.event.target_ids || message.mentions || []).map((id) => userDisplayName(id, usersById)).filter(Boolean);
    if (targets.length > 0) {
      return locale === "zh"
        ? `${actor} 邀请 ${targets.join("、")} 加入了房间`
        : `${actor} invited ${targets.join(", ")} to join the room`;
    }
  }
  return message.content || "";
}

function isLegacySystemEventContent(content) {
  const text = (content ?? "").trim();
  if (!text) {
    return false;
  }
  return [
    /^.+ invited .+ to join the room\.?$/,
    /^.+ created the room ".+"\.?$/,
    /^.+ 邀请 .+ 加入了房间。?$/,
    /^.+ 创建了房间“.+”。?$/,
  ].some((pattern) => pattern.test(text));
}

function userDisplayName(userID, usersById) {
  if (!userID) {
    return "";
  }
  const user = usersById.get(userID);
  if (!user) {
    return userID;
  }
  return user.name || (user.handle ? `@${user.handle}` : userID);
}

function resolveConversationUser(conversation, currentUserID, usersById) {
  const otherID = conversation.participants.find((id) => id !== currentUserID) ?? currentUserID;
  return usersById.get(otherID);
}

function isTwoPersonConversation(conversation) {
  return (conversation?.participants?.length ?? 0) === 2;
}

function getConversationSubtitle(conversation, currentUserID, usersById, locale, t) {
  return "";
}

function getConversationDescription(conversation, currentUserID, usersById, locale, t) {
  if (!isTwoPersonConversation(conversation)) {
    return conversation.description || "";
  }
  return "";
}

function formatTime(value, locale) {
  if (!value) return "";
  return new Date(value).toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone: locale === "zh" ? "Asia/Shanghai" : "UTC",
  });
}

function latestAt(conversation) {
  if (!conversation.messages.length) return 0;
  return new Date(conversation.messages[conversation.messages.length - 1].created_at).getTime();
}

function applyIMEvent(current, event) {
  if (!current || !event?.type) {
    return current;
  }

  if (event.type === "user.created" && event.user) {
    return upsertUserInData(current, event.user);
  }
  if (event.type === "message.created" && event.message) {
    return appendMessageToData(current, event.room_id, event.message);
  }
  if ((event.type === "conversation.created" || event.type === "conversation.members_added" || event.type === "room.created" || event.type === "room.members_added") && event.room) {
    return upsertConversationInData(current, event.room);
  }
  return current;
}

function appendMessageToData(current, conversationID, message) {
  if (!current || !conversationID || !message) {
    return current;
  }

  const rooms = current.rooms.map((room) => {
    if (room.id !== conversationID) {
      return room;
    }
    if (room.messages.some((item) => item.id === message.id)) {
      return room;
    }
    return { ...room, messages: [...room.messages, message] };
  });
  return { ...current, rooms: sortConversations(rooms) };
}

function upsertConversationInData(current, conversation) {
  if (!current || !conversation) {
    return current;
  }

  const existing = current.rooms.some((item) => item.id === conversation.id);
  const rooms = existing
    ? current.rooms.map((item) => (item.id === conversation.id ? conversation : item))
    : [conversation, ...current.rooms];
  return { ...current, rooms: sortConversations(rooms) };
}

function upsertUserInData(current, user) {
  if (!current || !user) {
    return current;
  }

  const existing = current.users.some((item) => item.id === user.id);
  const users = existing
    ? current.users.map((item) => (item.id === user.id ? user : item))
    : [...current.users, user];
  users.sort((a, b) => a.name.localeCompare(b.name));
  return { ...current, users };
}

function removeConversationFromData(current, conversationID) {
  if (!current || !conversationID) {
    return current;
  }

  const rooms = current.rooms.filter((item) => item.id !== conversationID);
  return { ...current, rooms };
}

function sortConversations(conversations) {
  return [...conversations].sort((a, b) => latestAt(b) - latestAt(a));
}

function normalizeIMData(payload) {
  if (!payload) {
    return payload;
  }
  return { ...payload, rooms: payload.rooms ?? [] };
}

function toggleSelection(current, id) {
  return current.includes(id) ? current.filter((item) => item !== id) : [...current, id];
}

function renderMarkdown(content) {
  const raw = marked.parse(content ?? "");
  return DOMPurify.sanitize(raw, {
    USE_PROFILES: { html: true },
    ADD_ATTR: ["target", "rel", "class"],
  });
}

function parseStructuredMessage(content) {
  const cleaned = (content ?? "").trim();
  if (!cleaned) {
    return null;
  }

  const fencedJSON = cleaned.match(/^```(?:json|javascript|js)?\s*([\s\S]+?)\s*```$/i);
  const rawJSON = fencedJSON ? fencedJSON[1].trim() : cleaned;
  const parsed = tryParseJSON(rawJSON);
  if (parsed && isStructuredPayload(parsed)) {
    return buildStructuredPayload(parsed);
  }

  const codeBlock = extractSingleLargeCodeBlock(cleaned);
  if (codeBlock) {
    return buildCodeBlockPayload(codeBlock);
  }

  return null;
}

function tryParseJSON(input) {
  if (!input || (!input.startsWith("{") && !input.startsWith("["))) {
    return null;
  }
  try {
    return JSON.parse(input);
  } catch {
    return null;
  }
}

function isStructuredPayload(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const keys = Object.keys(value);
  return keys.some((key) => ["tool", "name", "arguments", "input", "file", "path", "code", "content", "status", "action"].includes(key));
}

function buildStructuredPayload(value) {
  const title = String(value.tool || value.name || value.action || "Structured output");
  const target = firstNonEmptyString(value.file, value.path, value.file_path, value.filename);
  const code = findLargeCodeString(value);

  return {
    title,
    subtitle: target && title !== target ? target : "",
    badge: inferPayloadBadge(value),
    summary: summarizeStructuredValue(value, code),
    code,
    codeSummary: code ? summarizeCode(code) : "",
    payload: JSON.stringify(value, null, 2),
    payloadSummary: `查看原始 JSON · ${Object.keys(value).length} 个字段`,
  };
}

function buildCodeBlockPayload(codeBlock) {
  const lineCount = codeBlock.code.split("\n").length;
  return {
    title: "Long code block",
    subtitle: codeBlock.language ? codeBlock.language.toUpperCase() : "Plain text",
    badge: lineCount > 80 ? "Long output" : "Code",
    summary: `检测到 ${lineCount} 行代码，默认折叠以避免聊天流被长内容撑开。`,
    code: codeBlock.code,
    codeSummary: `展开代码 · ${lineCount} 行`,
    payload: "",
    payloadSummary: "",
  };
}

function extractSingleLargeCodeBlock(content) {
  const match = content.match(/^```([\w-]+)?\n([\s\S]+?)\n```$/);
  if (!match) {
    return null;
  }
  const code = match[2];
  if (code.length < 600 && code.split("\n").length < 18) {
    return null;
  }
  return {
    language: match[1] || "",
    code,
  };
}

function findLargeCodeString(value, seen = new Set()) {
  if (!value || typeof value !== "object" || seen.has(value)) {
    return "";
  }
  seen.add(value);

  for (const key of ["code", "content", "text", "body", "source"]) {
    if (typeof value[key] === "string" && looksLikeCode(value[key])) {
      return value[key];
    }
  }

  for (const item of Object.values(value)) {
    if (typeof item === "string" && looksLikeCode(item)) {
      return item;
    }
    if (item && typeof item === "object") {
      const nested = findLargeCodeString(item, seen);
      if (nested) {
        return nested;
      }
    }
  }

  return "";
}

function looksLikeCode(text) {
  if (typeof text !== "string") {
    return false;
  }
  const trimmed = text.trim();
  if (trimmed.length < 180) {
    return false;
  }
  return /[{};<>]/.test(trimmed) || trimmed.includes("\n");
}

function summarizeStructuredValue(value, code) {
  const parts = [];
  const args = value.arguments || value.input || value.params;
  if (args && typeof args === "object" && !Array.isArray(args)) {
    const interestingKeys = Object.keys(args).slice(0, 3);
    if (interestingKeys.length > 0) {
      parts.push(`参数: ${interestingKeys.join(", ")}`);
    }
  }
  if (code) {
    parts.push(`代码: ${summarizeCode(code)}`);
  }
  return parts.join(" · ") || "已识别为结构化工具输出，默认折叠原始内容。";
}

function summarizeCode(code) {
  const lines = code.split("\n").length;
  const chars = code.length;
  return `${lines} 行 / ${chars} 字符`;
}

function inferPayloadBadge(value) {
  if (typeof value.status === "string" && value.status.trim()) {
    return value.status.trim();
  }
  if (typeof value.tool === "string" && value.tool.trim()) {
    return "Tool";
  }
  return "JSON";
}

function firstNonEmptyString(...values) {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return "";
}

createRoot(document.getElementById("root")).render(html`<${App} />`);
