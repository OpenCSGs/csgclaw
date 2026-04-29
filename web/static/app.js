import React, { useEffect, useMemo, useRef, useState } from "https://esm.sh/react@18.3.1";
import { createRoot } from "https://esm.sh/react-dom@18.3.1/client";
import htm from "https://esm.sh/htm@3.1.1";
import { marked } from "https://esm.sh/marked@13.0.2";
import DOMPurify from "https://esm.sh/dompurify@3.1.6";
import mermaid from "https://esm.sh/mermaid@11.4.1";

const html = htm.bind(React.createElement);
const LOCALE_STORAGE_KEY = "csgclaw.im.locale";
const THEME_STORAGE_KEY = "csgclaw.im.theme";
const TOOL_CALLS_STORAGE_KEY = "csgclaw.im.showToolCalls";
const SIDEBAR_COLLAPSED_STORAGE_KEY = "csgclaw.im.sidebarCollapsed";
const MESSAGE_LIST_BOTTOM_THRESHOLD = 24;
const AGENT_STATUS_REFRESH_INTERVAL_MS = 2000;
const PROVIDERS = ["csghub_lite", "codex", "claude_code", "api"];
const REASONING_EFFORTS = ["low", "medium", "high", "xhigh"];

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
    emptyConversation: "请选择一个房间或私信",
    conversationSection: "房间",
    computerSection: "电脑",
    computerAgentsSection: "Agents",
    channelsSection: "房间",
    directMessagesSection: "私信",
    localComputer: "本机",
    computerOverview: "电脑概览",
    agentOverview: "Agent 概览",
    yourView: "你的视图",
    activeNow: "当前在线",
    totalThreads: "房间总数",
    teamMembers: "团队成员",
    membersTitle: "成员",
    conversationOverview: "房间概览",
    sendFailed: "消息发送失败，请重试。",
    roomCreatedToast: "房间已创建",
    inviteSentToast: "邀请已发送",
    noMessages: "还没有消息，发一条开始吧。",
    noVisibleMessages: "工具调用已隐藏，当前没有可显示的消息。",
    createRoom: "创建房间",
    deleteRoom: "删除房间",
    conversationLabel: "房间",
    members: "成员",
    mentionBadge: "@ 提及",
    inviteMembers: "添加成员",
    inputPlaceholder: "输入消息，使用 @ 选择成员",
    send: "发送",
    composerTip: "Enter 发送，Shift + Enter 换行。支持房间、私信和 @ 提及。",
    profileSetupTitle: "配置 Manager Profile",
    profileSetupSubtitle: "自动检测没有找到可用模型。请完成 Manager 的运行配置后再开始对话。",
    profileProvider: "Provider",
    profileModel: "Model",
    profileBaseURL: "Base URL",
    profileAPIKey: "API Key",
    profileHeaders: "Headers JSON",
    profileRequestOptions: "请求选项（JSON，合并到请求顶层）",
    profileEnv: "环境变量",
    profileEnvKey: "键",
    profileEnvValue: "值",
    profileEnvAdd: "添加变量",
    profileEnvRemove: "移除变量",
    profileReasoning: "Reasoning",
    profileFastMode: "Fast mode",
    profileBasics: "基础信息",
    profileRuntime: "运行时",
    profileAPIProvider: "API Provider",
    profileAdvanced: "高级选项",
    profilePreview: "Profile 预览",
    openProfile: "打开 Profile",
    openDM: "打开私信",
    personProfile: "成员资料",
    roleLabel: "角色",
    handleLabel: "Handle",
    userIDLabel: "用户 ID",
    status: "状态",
    profileSelectModel: "选择模型",
    profileLoadingModels: "正在加载模型...",
    profileSave: "保存并启动",
    profileIncomplete: "Manager profile incomplete. Complete setup before sending messages.",
    profileSavedToast: "Profile 已保存",
    agentsSection: "Agents",
    createAgent: "创建 Agent",
    createAgentTitle: "创建 Agent",
    createAgentSubtitle: "创建一个 Worker，并使用最新 Profile 默认值。",
    editAgentTitle: "编辑 Agent Profile",
    editAgentSubtitle: "修改运行配置。Env 变更需要重新创建沙箱。",
    agentName: "名称",
    agentNamePlaceholder: "例如：dev",
    agentDescription: "说明",
    agentImage: "镜像",
    agentImagePlaceholder: "默认使用 Manager 镜像",
    agentStart: "启动",
    agentStop: "停止",
    agentRecreate: "重建",
    agentDelete: "删除",
    editProfile: "编辑",
    inviteToRoom: "加入当前房间",
    agentCreateSave: "创建并启动",
    agentUpdateSave: "保存",
    agentCreated: "Agent 已创建",
    agentUpdated: "Agent 已更新",
    agentActionFailed: "Agent 操作失败",
    profileRestartRequired: "需要重建",
    profileCompleteBadge: "已配置",
    profileIncompleteBadge: "未配置",
    noAgents: "还没有 Worker。",
    noChannels: "还没有房间。",
    noDirectMessages: "还没有私信。",
    modelLoadFailed: "模型加载失败",
    detectionResults: "自动检测结果",
    createRoomTitle: "创建房间",
    createRoomSubtitle: "为一个新主题建立房间，并预先邀请成员。",
    createRoomFromDM: "创建房间",
    close: "关闭",
    roomName: "房间名",
    roomNamePlaceholder: "例如：Launch",
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
    themeSwitcher: "切换外观",
    themeLight: "浅色",
    themeDark: "深色",
    toggleToolCallsShow: "显示工具调用",
    toggleToolCallsHide: "隐藏工具调用",
    channelTools: "房间工具",
    enabled: "开启",
    disabled: "关闭",
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
    emptyConversation: "Select a room or DM",
    conversationSection: "Rooms",
    computerSection: "Computer",
    computerAgentsSection: "Agents",
    channelsSection: "Rooms",
    directMessagesSection: "Direct Messages",
    localComputer: "Local computer",
    computerOverview: "Computer overview",
    agentOverview: "Agent overview",
    yourView: "Your view",
    activeNow: "Active now",
    totalThreads: "Rooms",
    teamMembers: "Members",
    membersTitle: "Members",
    conversationOverview: "Room overview",
    sendFailed: "Failed to send the message. Please retry.",
    roomCreatedToast: "Room created",
    inviteSentToast: "Invite sent",
    noMessages: "No messages yet. Start the conversation.",
    noVisibleMessages: "Tool calls are hidden, and there are no visible messages in this conversation.",
    createRoom: "New Room",
    deleteRoom: "Delete Room",
    conversationLabel: "Room",
    members: "members",
    mentionBadge: "@ mention",
    inviteMembers: "Add Members",
    inputPlaceholder: "Type a message and use @ to mention members",
    send: "Send",
    composerTip: "Press Enter to send and Shift + Enter for a new line. Supports rooms, DMs, and @ mentions.",
    profileSetupTitle: "Manager Profile",
    profileSetupSubtitle: "Auto-detection did not find a usable model. Complete the manager runtime profile before chatting.",
    profileProvider: "Provider",
    profileModel: "Model",
    profileBaseURL: "Base URL",
    profileAPIKey: "API Key",
    profileHeaders: "Headers JSON",
    profileRequestOptions: "Request options (JSON, merged into top-level request)",
    profileEnv: "Environment variables",
    profileEnvKey: "Key",
    profileEnvValue: "Value",
    profileEnvAdd: "Add variable",
    profileEnvRemove: "Remove variable",
    profileReasoning: "Reasoning",
    profileFastMode: "Fast mode",
    profileBasics: "Basics",
    profileRuntime: "Runtime",
    profileAPIProvider: "API Provider",
    profileAdvanced: "Advanced",
    profilePreview: "Profile preview",
    openProfile: "Open profile",
    openDM: "Open DM",
    personProfile: "Person profile",
    roleLabel: "Role",
    handleLabel: "Handle",
    userIDLabel: "User ID",
    status: "Status",
    profileSelectModel: "Select model",
    profileLoadingModels: "Loading models...",
    profileSave: "Save and start",
    profileIncomplete: "Manager profile incomplete. Complete setup before sending messages.",
    profileSavedToast: "Profile saved",
    agentsSection: "Agents",
    createAgent: "Create Agent",
    createAgentTitle: "Create Agent",
    createAgentSubtitle: "Create a worker with the latest profile defaults.",
    editAgentTitle: "Edit Agent Profile",
    editAgentSubtitle: "Change runtime settings. Env changes need a sandbox recreate.",
    agentName: "Name",
    agentNamePlaceholder: "For example: dev",
    agentDescription: "Description",
    agentImage: "Image",
    agentImagePlaceholder: "Uses the manager image by default",
    agentStart: "Start",
    agentStop: "Stop",
    agentRecreate: "Recreate",
    agentDelete: "Delete",
    editProfile: "Edit",
    inviteToRoom: "Add to current room",
    agentCreateSave: "Create and start",
    agentUpdateSave: "Save",
    agentCreated: "Agent created",
    agentUpdated: "Agent updated",
    agentActionFailed: "Agent action failed",
    profileRestartRequired: "Restart needed",
    profileCompleteBadge: "Configured",
    profileIncompleteBadge: "Incomplete",
    noAgents: "No workers yet.",
    noChannels: "No rooms yet.",
    noDirectMessages: "No direct messages yet.",
    modelLoadFailed: "Failed to load models",
    detectionResults: "Auto-detection",
    createRoomTitle: "New Room",
    createRoomSubtitle: "Create a new room and invite members in advance.",
    createRoomFromDM: "New Room",
    close: "Close",
    roomName: "Room name",
    roomNamePlaceholder: "For example: Launch",
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
    themeSwitcher: "Switch theme",
    themeLight: "Light",
    themeDark: "Dark",
    toggleToolCallsShow: "Show tool calls",
    toggleToolCallsHide: "Hide tool calls",
    channelTools: "Room tools",
    enabled: "On",
    disabled: "Off",
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

function SunIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="3.8" fill="none" stroke="currentColor" stroke-width="1.8" />
      <path
        d="M12 3.25v2.1M12 18.65v2.1M4.25 12h2.1M17.65 12h2.1M6.52 6.52l1.48 1.48M16 16l1.48 1.48M17.48 6.52L16 8M8 16l-1.48 1.48"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-width="1.8"
      />
    </svg>
  `;
}

function MoonIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M19.2 14.6A7.3 7.3 0 0 1 9.4 4.8a7.6 7.6 0 1 0 9.8 9.8Z"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.8"
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
      <circle cx="12" cy="12" r="10" fill="var(--panel-soft)" />
      <path d="M12 7.5v9M7.5 12h9" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.9" />
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
      <circle cx="12" cy="12" r="10" fill="var(--panel-soft)" />
      <path
        d="M9.25 7.75c-2.35 0-4.25 1.64-4.25 3.67c0 1.01.47 1.93 1.23 2.59L5.5 16.25l2.91-.46c.27.04.55.05.84.05c2.35 0 4.25-1.64 4.25-3.67S11.6 7.75 9.25 7.75Zm5.3 2.92c2.04.21 3.65 1.65 3.65 3.42c0 .88-.4 1.69-1.08 2.29l.58 1.88l-2.35-.43c-.25.03-.52.04-.8.04c-1.75 0-3.25-.78-4-1.95"
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.7"
      />
    </svg>
  `;
}

function AgentIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M8.5 4.75a3.5 3.5 0 0 1 7 0v1.1h1.3a2.7 2.7 0 0 1 2.7 2.7v6.95a2.7 2.7 0 0 1-2.7 2.7H7.2a2.7 2.7 0 0 1-2.7-2.7V8.55a2.7 2.7 0 0 1 2.7-2.7h1.3v-1.1Zm1.5 1.1h4v-1.1a2 2 0 1 0-4 0v1.1Zm-2.8 1.5a1.2 1.2 0 0 0-1.2 1.2v6.95c0 .66.54 1.2 1.2 1.2h9.6a1.2 1.2 0 0 0 1.2-1.2V8.55a1.2 1.2 0 0 0-1.2-1.2H7.2Zm2.1 4.15a1.05 1.05 0 1 1 2.1 0a1.05 1.05 0 0 1-2.1 0Zm4.25 0a1.05 1.05 0 1 1 2.1 0a1.05 1.05 0 0 1-2.1 0Zm-4.22 3.1h5.34v1.35H9.33V14.6Z"
        fill="currentColor"
      />
    </svg>
  `;
}

function ComputerIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M4.5 5.75h15a1.75 1.75 0 0 1 1.75 1.75v8.25a1.75 1.75 0 0 1-1.75 1.75h-15a1.75 1.75 0 0 1-1.75-1.75V7.5A1.75 1.75 0 0 1 4.5 5.75Zm0 1.5a.25.25 0 0 0-.25.25v8.25c0 .14.11.25.25.25h15a.25.25 0 0 0 .25-.25V7.5a.25.25 0 0 0-.25-.25h-15ZM9 19h6v1.5H9V19Z"
        fill="currentColor"
      />
    </svg>
  `;
}

function PlayIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d="M8.3 5.7v12.6c0 .74.8 1.2 1.44.83l9.7-6.3a.98.98 0 0 0 0-1.66l-9.7-6.3a.97.97 0 0 0-1.44.83Z" fill="currentColor" />
    </svg>
  `;
}

function StopIcon() {
  return html`
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d="M7.4 6.4h9.2c.55 0 1 .45 1 1v9.2c0 .55-.45 1-1 1H7.4c-.55 0-1-.45-1-1V7.4c0-.55.45-1 1-1Z" fill="currentColor" />
    </svg>
  `;
}

function paneFromLocation(pathname = window.location.pathname) {
  const parts = String(pathname || "/").split("/").filter(Boolean).map(decodePathSegment);
  const section = parts[0] || "";
  const id = parts[1] || "";
  switch (section) {
    case "computer":
      return { type: "computer", id: "local" };
    case "agents":
    case "agent":
      return id ? { type: "agent", id } : { type: "computer", id: "local" };
    case "channels":
    case "channel":
    case "dms":
    case "dm":
    case "rooms":
    case "room":
    case "conversations":
    case "conversation":
      return id ? { type: "conversation", id } : { type: "conversation", id: "" };
    default:
      return { type: "conversation", id: "" };
  }
}

function pathForPane(pane, rooms = []) {
  if (!pane || pane.type === "computer") {
    return "/computer";
  }
  if (pane.type === "agent" && pane.id) {
    return `/agents/${encodeURIComponent(pane.id)}`;
  }
  if (pane.type === "conversation" && pane.id) {
    const room = rooms.find((item) => item.id === pane.id);
    const prefix = room && isDirectConversation(room) ? "/dms/" : "/rooms/";
    return `${prefix}${encodeURIComponent(pane.id)}`;
  }
  return "/";
}

function syncBrowserPath(pane, rooms, mode = "push") {
  const nextPath = pathForPane(pane, rooms);
  if (!nextPath || window.location.pathname === nextPath) {
    return;
  }
  const state = { pane };
  if (mode === "replace") {
    window.history.replaceState(state, "", nextPath);
    return;
  }
  window.history.pushState(state, "", nextPath);
}

function decodePathSegment(value) {
  try {
    return decodeURIComponent(value || "");
  } catch (_) {
    return value || "";
  }
}

function App() {
  const initialPane = useMemo(() => paneFromLocation(), []);
  const [locale, setLocale] = useState(() => detectInitialLocale());
  const [theme, setTheme] = useState(() => detectInitialTheme());
  const [showToolCalls, setShowToolCalls] = useState(() => {
    const value = window.localStorage.getItem(TOOL_CALLS_STORAGE_KEY);
    return value === "true";
  });
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(() => {
    const value = window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY);
    return value === "true";
  });
  const [data, setData] = useState(null);
  const [activeConversationId, setActiveConversationId] = useState(() => initialPane.type === "conversation" ? initialPane.id : "");
  const [activePane, setActivePane] = useState(initialPane);
  const [draftsByConversationId, setDraftsByConversationId] = useState({});
  const [composerMentionState, setComposerMentionState] = useState(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [showCreateRoom, setShowCreateRoom] = useState(false);
  const [showInvite, setShowInvite] = useState(false);
  const [showMemberList, setShowMemberList] = useState(false);
  const [showChannelTools, setShowChannelTools] = useState(false);
  const [roomTitle, setRoomTitle] = useState("");
  const [roomDescription, setRoomDescription] = useState("");
  const [roomMemberIDs, setRoomMemberIDs] = useState([]);
  const [lockedRoomMemberIDs, setLockedRoomMemberIDs] = useState([]);
  const [inviteUserIDs, setInviteUserIDs] = useState([]);
  const [submitError, setSubmitError] = useState("");
  const [composerError, setComposerError] = useState("");
  const [loadingError, setLoadingError] = useState("");
  const [managerProfile, setManagerProfile] = useState(null);
  const [profileDraft, setProfileDraft] = useState(null);
  const [profileModels, setProfileModels] = useState([]);
  const [profileError, setProfileError] = useState("");
  const [profileBusy, setProfileBusy] = useState(false);
  const [profileModelBusy, setProfileModelBusy] = useState(false);
  const [agents, setAgents] = useState([]);
  const [agentsLoaded, setAgentsLoaded] = useState(false);
  const [agentsError, setAgentsError] = useState("");
  const [showAgentModal, setShowAgentModal] = useState(false);
  const [agentModalMode, setAgentModalMode] = useState("create");
  const [editingAgent, setEditingAgent] = useState(null);
  const [agentDraft, setAgentDraft] = useState(null);
  const [agentModels, setAgentModels] = useState([]);
  const [agentBusy, setAgentBusy] = useState(false);
  const [agentModelBusy, setAgentModelBusy] = useState(false);
  const [agentError, setAgentError] = useState("");
  const [agentActionBusy, setAgentActionBusy] = useState("");
  const [agentPageDraft, setAgentPageDraft] = useState(null);
  const [agentPageModels, setAgentPageModels] = useState([]);
  const [agentPageBusy, setAgentPageBusy] = useState(false);
  const [agentPageModelBusy, setAgentPageModelBusy] = useState(false);
  const [agentPageError, setAgentPageError] = useState("");
  const [profilePreview, setProfilePreview] = useState(null);
  const editorRef = useRef(null);
  const messageListRef = useRef(null);
  const memberMenuRef = useRef(null);
  const channelToolsRef = useRef(null);
  const agentRefreshTimerRef = useRef(null);
  const shouldAutoScrollRef = useRef(true);

  useEffect(() => {
    refreshBootstrap();
  }, []);

  useEffect(() => {
    refreshManagerProfile();
    refreshAgents();
  }, []);

  useEffect(() => {
    function refreshVisibleAgents() {
      if (document.visibilityState === "visible") {
        refreshAgents({ silent: true });
      }
    }

    const intervalID = window.setInterval(refreshVisibleAgents, AGENT_STATUS_REFRESH_INTERVAL_MS);
    document.addEventListener("visibilitychange", refreshVisibleAgents);
    return () => {
      window.clearInterval(intervalID);
      document.removeEventListener("visibilitychange", refreshVisibleAgents);
    };
  }, []);

  useEffect(() => {
    const source = new EventSource("api/v1/events");

    source.onmessage = (event) => {
      const payload = JSON.parse(event.data);
      setData((current) => applyIMEvent(current, payload));
      if (isAgentRosterEvent(payload)) {
        scheduleAgentsRefresh();
      }
    };

    return () => {
      source.close();
      if (agentRefreshTimerRef.current) {
        window.clearTimeout(agentRefreshTimerRef.current);
        agentRefreshTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    document.title = messages[locale].pageTitle;
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  }, [locale]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: theme === "dark" ? "dark" : "neutral",
    });
  }, [theme]);

  useEffect(() => {
    window.localStorage.setItem(TOOL_CALLS_STORAGE_KEY, String(showToolCalls));
  }, [showToolCalls]);

  useEffect(() => {
    window.localStorage.setItem(SIDEBAR_COLLAPSED_STORAGE_KEY, String(isSidebarCollapsed));
  }, [isSidebarCollapsed]);

  useEffect(() => {
    function handlePopState() {
      const next = paneFromLocation();
      setActivePane(next);
      if (next.type === "conversation") {
        setActiveConversationId(next.id);
      }
      setShowMemberList(false);
    }

    window.history.replaceState({ pane: activePane }, "", window.location.pathname);
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const t = useMemo(() => createTranslator(locale), [locale]);
  const managerProfileIncomplete = managerProfile && managerProfile.profile_complete === false;

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
  const channels = useMemo(
    () => rooms.filter((room) => !isDirectConversation(room)),
    [rooms],
  );
  const directMessages = useMemo(
    () => rooms.filter((room) => isDirectConversation(room)),
    [rooms],
  );
  const selectedAgentForPage = useMemo(() => {
    if (activePane.type !== "agent") {
      return null;
    }
    const managerAgent = agents.find((item) => item.role === "manager" || item.id === "u-manager");
    const workerAgents = agents.filter((item) => item.id !== managerAgent?.id);
    return [managerAgent, ...workerAgents].filter(Boolean).find((item) => item.id === activePane.id) ?? null;
  }, [agents, activePane]);

  const mentionCandidates = useMemo(() => {
    if (!data || !composerMentionState) {
      return [];
    }
    const allowed = new Set(activeConversation?.members ?? []);
    return data.users
      .filter((user) => allowed.has(user.id))
      .filter((user) => user.handle.toLowerCase().includes(composerMentionState.query.toLowerCase()) || user.name.toLowerCase().includes(composerMentionState.query.toLowerCase()))
      .slice(0, 5);
  }, [data, activeConversation, composerMentionState]);

  const draftSegments = useMemo(
    () => draftsByConversationId[activeConversationId] ?? [],
    [draftsByConversationId, activeConversationId],
  );
  const draftText = useMemo(() => segmentsToPlainText(draftSegments), [draftSegments]);

  useEffect(() => {
    setMentionIndex(0);
  }, [activeConversationId, composerMentionState?.query, draftText]);

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
  }, [activeConversationId]);

  useEffect(() => {
    if (!showMemberList) {
      return undefined;
    }

    function handlePointerDown(event) {
      const menu = memberMenuRef.current;
      if (!menu || menu.contains(event.target)) {
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

    function handlePointerDown(event) {
      const menu = channelToolsRef.current;
      if (!menu || menu.contains(event.target)) {
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
    if (!activeConversationId) {
      if (data.rooms.length > 0) {
        setActiveConversationId(data.rooms[0].id);
        if (!activePane.id) {
          const next = { type: "conversation", id: data.rooms[0].id };
          setActivePane(next);
          syncBrowserPath(next, data.rooms, "replace");
        }
      } else {
        if (!activePane.id) {
          const next = { type: "computer", id: "local" };
          setActivePane(next);
          syncBrowserPath(next, data.rooms, "replace");
        }
      }
      return;
    }
    if (!data.rooms.some((room) => room.id === activeConversationId)) {
      const nextID = data.rooms[0]?.id ?? "";
      if (nextID) {
        selectConversation(nextID, { replace: true });
      } else {
        setActiveConversationId("");
        selectComputer({ replace: true });
      }
    }
  }, [data, activeConversationId, activePane.id]);

  useEffect(() => {
    if (!activePane || activePane.type !== "agent") {
      return;
    }
    if (!agentsLoaded) {
      return;
    }
    if (!agents.some((item) => item.id === activePane.id)) {
      if (activeConversationId) {
        selectConversation(activeConversationId, { replace: true });
      } else {
        selectComputer({ replace: true });
      }
    }
  }, [agents, agentsLoaded, activePane, activeConversationId]);

  useEffect(() => {
    if (!data || !activePane?.id) {
      return;
    }
    syncBrowserPath(activePane, rooms, "replace");
  }, [data, activePane?.type, activePane?.id, rooms]);

  useEffect(() => {
    if (!showAgentModal || !agentDraft?.provider) {
      return undefined;
    }
    const timer = window.setTimeout(() => loadAgentModels(agentDraft, { silent: true }), agentDraft.provider === "api" ? 420 : 0);
    return () => window.clearTimeout(timer);
  }, [showAgentModal, agentDraft?.provider, agentDraft?.base_url, agentDraft?.api_key, agentDraft?.headersText]);

  useEffect(() => {
    if (!selectedAgentForPage) {
      setAgentPageDraft(null);
      setAgentPageModels([]);
      setAgentPageError("");
      return;
    }
    loadAgentPageDraft(selectedAgentForPage);
  }, [selectedAgentForPage?.id]);

  useEffect(() => {
    if (activePane.type !== "agent" || !agentPageDraft?.provider) {
      return undefined;
    }
    const timer = window.setTimeout(() => loadAgentPageModels(agentPageDraft, { silent: true }), agentPageDraft.provider === "api" ? 420 : 0);
    return () => window.clearTimeout(timer);
  }, [activePane.type, activePane.id, agentPageDraft?.provider, agentPageDraft?.base_url, agentPageDraft?.api_key, agentPageDraft?.headersText]);

  useEffect(() => {
    if (!managerProfileIncomplete || !profileDraft?.provider) {
      return undefined;
    }
    const timer = window.setTimeout(() => loadProfileModels(profileDraft, { silent: true }), profileDraft.provider === "api" ? 420 : 0);
    return () => window.clearTimeout(timer);
  }, [managerProfileIncomplete, profileDraft?.provider, profileDraft?.base_url, profileDraft?.api_key, profileDraft?.headersText]);

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

  useEffect(() => {
    const el = messageListRef.current;
    if (!el) {
      return;
    }
    shouldAutoScrollRef.current = true;
    el.scrollTop = el.scrollHeight;
  }, [activeConversationId]);

  useEffect(() => {
    const el = messageListRef.current;
    if (!el || !shouldAutoScrollRef.current) {
      return;
    }
    el.scrollTop = el.scrollHeight;
  }, [visibleMessages.length]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    if (!areComposerSegmentsEqual(parseComposerSegments(editor), draftSegments)) {
      renderComposerSegments(editor, draftSegments);
    }
    setComposerMentionState(null);
  }, [activeConversationId]);

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

  async function refreshBootstrap() {
    try {
      const resp = await fetch("api/v1/bootstrap");
      if (!resp.ok) {
        throw new Error("bootstrap failed");
      }
      const payload = await resp.json();
      const normalized = normalizeIMData(payload);
      setData(normalized);
      setLoadingError("");
      setInviteUserIDs([]);
      if (!activeConversationId && payload.rooms.length > 0) {
        if (activePane.id && activePane.type !== "conversation") {
          setActiveConversationId(payload.rooms[0].id);
        } else {
          selectConversation(payload.rooms[0].id, { replace: true, rooms: normalized.rooms });
        }
      }
      return normalized;
    } catch (_) {
      setLoadingError(messages[locale].loadingFailed);
      return null;
    }
  }

  async function sendMessage() {
    if (managerProfileIncomplete) {
      setComposerError(t("profileIncomplete"));
      return;
    }
    if (!data || !activeConversation || !draftText.trim()) {
      return;
    }

    setComposerError("");
    const resp = await fetch("api/v1/messages", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        room_id: activeConversation.id,
        sender_id: data.current_user_id,
        content: serializeComposerSegments(draftSegments),
      }),
    });
    if (!resp.ok) {
      setComposerError(t("sendFailed"));
      return;
    }
    const created = await resp.json();
    setData((current) => appendMessageToData(current, activeConversation.id, created));
    clearComposer();
  }

  async function createRoom() {
    if (!data || !roomTitle.trim()) {
      return;
    }

    setSubmitError("");
    const memberIDs = roomMemberIDs.filter((id) => id && id !== data.current_user_id);
    const resp = await fetch("api/v1/rooms", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        title: roomTitle,
        description: roomDescription,
        creator_id: data.current_user_id,
        member_ids: memberIDs,
        locale,
      }),
    });
    if (!resp.ok) {
      setSubmitError(localizeError(await resp.text(), t));
      return;
    }

    const created = await resp.json();
    setData((current) => upsertConversationInData(current, created));
    selectConversation(created.id);
    setComposerError("");
    setShowCreateRoom(false);
  }

  function selectConversation(id, options = {}) {
    setActiveConversationId(id);
    const next = { type: "conversation", id };
    setActivePane(next);
    setShowMemberList(false);
    setShowChannelTools(false);
    if (options.updateURL !== false) {
      syncBrowserPath(next, options.rooms ?? rooms, options.replace ? "replace" : "push");
    }
  }

  function selectAgent(item, options = {}) {
    if (!item?.id) {
      return;
    }
    const next = { type: "agent", id: item.id };
    setActivePane(next);
    setShowMemberList(false);
    setShowChannelTools(false);
    if (options.updateURL !== false) {
      syncBrowserPath(next, rooms, options.replace ? "replace" : "push");
    }
  }

  function selectComputer(options = {}) {
    const next = { type: "computer", id: "local" };
    setActivePane(next);
    setShowMemberList(false);
    setShowChannelTools(false);
    if (options.updateURL !== false) {
      syncBrowserPath(next, rooms, options.replace ? "replace" : "push");
    }
  }

  function openCreateRoomModal(options = {}) {
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
    setShowInvite(true);
  }

  async function inviteUsers() {
    if (!data || !activeConversation || inviteUserIDs.length === 0) {
      return;
    }

    setSubmitError("");
    const resp = await fetch("api/v1/rooms/invite", {
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

    const resp = await fetch(`api/v1/rooms/${roomID}`, {
      method: "DELETE",
    });
    if (!resp.ok) {
      setComposerError(localizeError(await resp.text(), t));
      return;
    }

    const remainingRooms = rooms.filter((item) => item.id !== roomID);
    setData((current) => removeConversationFromData(current, roomID));
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

  function applyMention(user) {
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

  function syncComposerFromEditor() {
    const editor = editorRef.current;
    if (!editor || !activeConversationId) {
      return;
    }
    const segments = parseComposerSegments(editor);
    setDraftsByConversationId((current) => updateDrafts(current, activeConversationId, segments));
    setComposerMentionState(getComposerMentionState(editor));
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

  if (!data) {
    return html`<div className="empty-state">${loadingError || t("loading")}</div>`;
  }

  const createRoomCandidates = data.users;
  const inviteCandidates = activeConversation
    ? data.users.filter((user) => !activeConversation.members.includes(user.id))
    : [];
  const activeConversationMembers = activeConversation
    ? activeConversation.members.map((id) => usersById.get(id)).filter(Boolean)
    : [];
  const inviteActionLabel = activeConversation && isDirectConversation(activeConversation)
    ? t("createRoomFromDM")
    : t("inviteMembers");

  const managerAgent = agents.find((item) => item.role === "manager" || item.id === "u-manager");
  const workerAgents = agents.filter((item) => item.id !== managerAgent?.id);
  const agentItems = [managerAgent, ...workerAgents].filter(Boolean);
  const selectedAgent = selectedAgentForPage;
  const selectedConversation = activePane.type === "conversation" ? activeConversation : null;
  const activeChannel = selectedConversation && !isDirectConversation(selectedConversation) ? selectedConversation : null;
  const previewUser = profilePreview?.type === "user"
    ? usersById.get(profilePreview.id) ?? null
    : profilePreview?.type === "agent"
      ? usersById.get(profilePreview.id) ?? null
      : null;
  const previewAgent = profilePreview
    ? agentItems.find((item) => item.id === profilePreview.id || agentMatchesUser(item, previewUser)) ?? null
    : null;

  async function refreshManagerProfile() {
    try {
      const resp = await fetch("api/v1/agents/u-manager/profile");
      if (!resp.ok) {
        return;
      }
      const profile = await resp.json();
      setManagerProfile(profile);
      setProfileDraft(profileToDraft(profile));
    } catch (_) {
      // The manager may not exist during the first bootstrap milliseconds.
    }
  }

  async function loadProfileModels(draft = profileDraft, options = {}) {
    if (!draft?.provider) {
      return;
    }
    if (!options.silent) {
      setProfileError("");
    }
    setProfileModelBusy(true);
    try {
      const resp = await fetch("api/v1/agent-profiles/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          provider: draft.provider,
          base_url: draft.base_url,
          api_key: draft.api_key,
          headers: parseJSONMap(draft.headersText),
        }),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || t("modelLoadFailed"));
      }
      const payload = await resp.json();
      setProfileModels(payload.models ?? []);
      if (!draft.model_id && payload.models?.length > 0) {
        setProfileDraft((current) => ({ ...current, model_id: payload.models[0] }));
      }
    } catch (err) {
      if (!options.silent) {
        setProfileError(err.message || t("modelLoadFailed"));
      }
      setProfileModels([]);
    } finally {
      setProfileModelBusy(false);
    }
  }

  async function saveManagerProfile() {
    if (!profileDraft) {
      return;
    }
    setProfileBusy(true);
    setProfileError("");
    try {
      const payload = draftToProfile(profileDraft);
      const resp = await fetch("api/v1/agents/u-manager/profile", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim());
      }
      const saved = await resp.json();
      setManagerProfile(saved);
      setProfileDraft(profileToDraft(saved));
      if (saved.profile_complete) {
        await fetch("api/v1/agents/u-manager/recreate", { method: "POST" });
      }
      await refreshManagerProfile();
      setComposerError("");
    } catch (err) {
      setProfileError(err.message || t("sendFailed"));
    } finally {
      setProfileBusy(false);
    }
  }

  async function refreshAgents(options = {}) {
    try {
      const resp = await fetch(options.silent ? "api/v1/agents?poll=1" : "api/v1/agents");
      if (!resp.ok) {
        throw new Error((await resp.text()).trim());
      }
      setAgents(await resp.json());
      setAgentsLoaded(true);
      setAgentsError("");
    } catch (err) {
      if (!options.silent) {
        setAgentsError(err.message || t("agentActionFailed"));
      }
    }
  }

  function scheduleAgentsRefresh() {
    if (agentRefreshTimerRef.current) {
      window.clearTimeout(agentRefreshTimerRef.current);
    }
    agentRefreshTimerRef.current = window.setTimeout(() => {
      agentRefreshTimerRef.current = null;
      refreshAgents({ silent: true });
    }, 120);
  }

  async function openCreateAgentModal() {
    setAgentModalMode("create");
    setEditingAgent(null);
    setAgentError("");
    setAgentModels([]);
    const managerAgent = agents.find((item) => item.role === "manager" || item.id === "u-manager");
    try {
      const resp = await fetch("api/v1/agent-profile-defaults");
      const defaults = resp.ok ? await resp.json() : managerProfile;
      const draft = agentToDraft({ image: managerAgent?.image || "", agent_profile: defaults });
      setAgentDraft(draft);
      setShowAgentModal(true);
      loadAgentModels(draft, { silent: true });
    } catch (_) {
      const draft = agentToDraft({ image: managerAgent?.image || "", agent_profile: managerProfile });
      setAgentDraft(draft);
      setShowAgentModal(true);
      loadAgentModels(draft, { silent: true });
    }
  }

  async function openEditAgentModal(item) {
    setAgentModalMode("edit");
    setEditingAgent(item);
    setAgentError("");
    setAgentModels([]);
    try {
      const resp = await fetch(`api/v1/agents/${encodeURIComponent(item.id)}/profile`);
      const profile = resp.ok ? await resp.json() : item.agent_profile;
      const draft = agentToDraft({ ...item, agent_profile: profile });
      setAgentDraft(draft);
      setShowAgentModal(true);
      loadAgentModels(draft, { silent: true });
    } catch (err) {
      setAgentError(err.message || t("agentActionFailed"));
    }
  }

  async function loadAgentPageDraft(item) {
    if (!item?.id) {
      return;
    }
    setAgentPageError("");
    setAgentPageModels([]);
    try {
      const resp = await fetch(`api/v1/agents/${encodeURIComponent(item.id)}/profile`);
      const profile = resp.ok ? await resp.json() : item.agent_profile;
      const draft = agentToDraft({ ...item, agent_profile: profile });
      setAgentPageDraft(draft);
      loadAgentPageModels(draft, { silent: true });
    } catch (err) {
      setAgentPageError(err.message || t("agentActionFailed"));
      const draft = agentToDraft(item);
      setAgentPageDraft(draft);
      loadAgentPageModels(draft, { silent: true });
    }
  }

  async function loadAgentPageModels(draft = agentPageDraft, options = {}) {
    if (!draft?.provider) {
      return;
    }
    if (!options.silent) {
      setAgentPageError("");
    }
    setAgentPageModelBusy(true);
    try {
      const resp = await fetch("api/v1/agent-profiles/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          provider: draft.provider,
          base_url: draft.base_url,
          api_key: draft.api_key,
          headers: parseJSONMap(draft.headersText),
        }),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || t("modelLoadFailed"));
      }
      const payload = await resp.json();
      setAgentPageModels(payload.models ?? []);
      if (!draft.model_id && payload.models?.length > 0) {
        setAgentPageDraft((current) => ({ ...current, model_id: payload.models[0] }));
      }
    } catch (err) {
      if (!options.silent) {
        setAgentPageError(err.message || t("modelLoadFailed"));
      }
      setAgentPageModels([]);
    } finally {
      setAgentPageModelBusy(false);
    }
  }

  async function loadAgentModels(draft = agentDraft, options = {}) {
    if (!draft?.provider) {
      return;
    }
    if (!options.silent) {
      setAgentError("");
    }
    setAgentModelBusy(true);
    try {
      const resp = await fetch("api/v1/agent-profiles/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          provider: draft.provider,
          base_url: draft.base_url,
          api_key: draft.api_key,
          headers: parseJSONMap(draft.headersText),
        }),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || t("modelLoadFailed"));
      }
      const payload = await resp.json();
      setAgentModels(payload.models ?? []);
      if (!draft.model_id && payload.models?.length > 0) {
        setAgentDraft((current) => ({ ...current, model_id: payload.models[0] }));
      }
    } catch (err) {
      if (!options.silent) {
        setAgentError(err.message || t("modelLoadFailed"));
      }
      setAgentModels([]);
    } finally {
      setAgentModelBusy(false);
    }
  }

  async function saveAgentPage() {
    if (!agentPageDraft || !selectedAgentForPage?.id) {
      return;
    }
    setAgentPageBusy(true);
    setAgentPageError("");
    try {
      const profile = draftToProfile(agentPageDraft, {
        name: agentPageDraft.name,
        description: agentPageDraft.description,
      });
      const payload = {
        name: agentPageDraft.name,
        description: agentPageDraft.description,
        image: agentPageDraft.image,
        agent_profile: profile,
      };
      const resp = await fetch(`api/v1/agents/${encodeURIComponent(selectedAgentForPage.id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim());
      }
      const saved = await resp.json();
      await refreshAgents();
      if (saved.id === "u-manager") {
        await refreshManagerProfile();
      }
      setAgentPageDraft(agentToDraft(saved));
    } catch (err) {
      setAgentPageError(err.message || t("agentActionFailed"));
    } finally {
      setAgentPageBusy(false);
    }
  }

  async function saveAgent() {
    if (!agentDraft) {
      return;
    }
    setAgentBusy(true);
    setAgentError("");
    try {
      const profile = draftToProfile(agentDraft, {
        name: agentDraft.name,
        description: agentDraft.description,
      });
      const payload = {
        name: agentDraft.name,
        description: agentDraft.description,
        image: agentDraft.image,
        agent_profile: profile,
      };
      const isCreate = agentModalMode === "create";
      const url = isCreate ? "api/v1/agents" : `api/v1/agents/${encodeURIComponent(editingAgent.id)}`;
      const resp = await fetch(url, {
        method: isCreate ? "POST" : "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim());
      }
      const saved = await resp.json();
      await refreshAgents();
      if (isCreate) {
        await refreshBootstrap();
      }
      if (saved.id === "u-manager") {
        await refreshManagerProfile();
      }
      setShowAgentModal(false);
      setAgentDraft(null);
    } catch (err) {
      setAgentError(err.message || t("agentActionFailed"));
    } finally {
      setAgentBusy(false);
    }
  }

  async function runAgentAction(item, action) {
    if (!item?.id || agentActionBusy) {
      return;
    }
    if (action === "delete" && !window.confirm(`${t("agentDelete")} ${item.name}?`)) {
      return;
    }
    setAgentActionBusy(`${item.id}:${action}`);
    setAgentsError("");
    try {
      const url = action === "delete"
        ? `api/v1/agents/${encodeURIComponent(item.id)}`
        : `api/v1/agents/${encodeURIComponent(item.id)}/${action}`;
      const resp = await fetch(url, { method: action === "delete" ? "DELETE" : "POST" });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim());
      }
      await refreshAgents();
      if (item.id === "u-manager") {
        await refreshManagerProfile();
      }
    } catch (err) {
      setAgentsError(err.message || t("agentActionFailed"));
    } finally {
      setAgentActionBusy("");
    }
  }

  async function inviteAgentToRoom(item, options = {}) {
    if (!activeConversation || isDirectConversation(activeConversation) || !data?.current_user_id || !item?.id) {
      return;
    }
    if (!options.silent) {
      setAgentsError("");
    }
    try {
      const resp = await fetch("api/v1/im/agents/join", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agent_id: item.id,
          room_id: activeConversation.id,
          inviter_id: data.current_user_id,
          locale,
        }),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim());
      }
      await refreshBootstrap();
    } catch (err) {
      if (!options.silent) {
        setAgentsError(err.message || t("agentActionFailed"));
      }
    }
  }

  function directConversationForUser(userID, roomList = rooms, currentUserID = data?.current_user_id) {
    if (!userID || !currentUserID) {
      return null;
    }
    return roomList.find((room) => (
      isDirectConversation(room) &&
      room.members.includes(currentUserID) &&
      room.members.includes(userID)
    )) ?? null;
  }

  async function openAgentDirectMessage(item) {
    if (!item?.id || !data?.current_user_id) {
      return;
    }

    setAgentsError("");
    try {
      let nextData = null;
      let direct = directConversationForUser(item.id);
      if (!direct) {
        const resp = await fetch("api/v1/users", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            id: item.id,
            name: item.name,
            handle: item.handle || item.id.replace(/^u-/, "") || item.name,
            role: item.role || "worker",
          }),
        });
        if (!resp.ok) {
          setAgentsError(localizeError(await resp.text(), t));
          return;
        }
        nextData = await refreshBootstrap();
        direct = directConversationForUser(
          item.id,
          nextData?.rooms ?? rooms,
          nextData?.current_user_id ?? data.current_user_id,
        );
      }

      if (!direct) {
        setAgentsError(t("agentActionFailed"));
        return;
      }
      selectConversation(direct.id, { rooms: nextData?.rooms ?? rooms });
      closeProfilePreview();
    } catch (err) {
      setAgentsError(err.message || t("agentActionFailed"));
    }
  }

  function openParticipantPreview(user) {
    if (!user?.id) {
      return;
    }
    const agent = agents.find((item) => agentMatchesUser(item, user));
    setProfilePreview(agent ? { type: "agent", id: agent.id } : { type: "user", id: user.id });
    setShowMemberList(false);
    setShowChannelTools(false);
  }

  function openAgentPreview(item) {
    if (!item?.id) {
      return;
    }
    setProfilePreview({ type: "agent", id: item.id });
    setShowChannelTools(false);
  }

  function closeProfilePreview() {
    setProfilePreview(null);
  }

  return html`
    <${React.Fragment}>
      <div className=${`app-shell ${isSidebarCollapsed ? "sidebar-collapsed" : ""}`}>
        <div className="sidebar-slot">
          <aside
            className=${`sidebar ${isSidebarCollapsed ? "collapsed" : ""}`}
            aria-hidden=${isSidebarCollapsed}
            inert=${isSidebarCollapsed}
          >
            <div className="sidebar-header workspace-header">
              <div className="sidebar-brand-row">
                <div className="sidebar-brand">CSGClaw</div>
                <div className="sidebar-controls">
                  <div className="theme-switch" role="group" aria-label=${t("themeSwitcher")}>
                    <div className=${`theme-switch-track ${theme === "dark" ? "is-dark" : "is-light"}`}>
                      <span className="theme-switch-thumb" aria-hidden="true"></span>
                      <button
                        className=${`theme-toggle ${theme === "light" ? "active" : ""}`}
                        aria-label=${t("themeLight")}
                        aria-pressed=${theme === "light"}
                        title=${t("themeLight")}
                        onClick=${() => setTheme("light")}
                      >
                        <span aria-hidden="true"><${SunIcon} /></span>
                      </button>
                      <button
                        className=${`theme-toggle ${theme === "dark" ? "active" : ""}`}
                        aria-label=${t("themeDark")}
                        aria-pressed=${theme === "dark"}
                        title=${t("themeDark")}
                        onClick=${() => setTheme("dark")}
                      >
                        <span aria-hidden="true"><${MoonIcon} /></span>
                      </button>
                    </div>
                  </div>
                  <div className="language-switch sidebar-language-switch" role="group" aria-label=${t("languageSwitcher")}>
                    <span className="language-switch-icon" aria-hidden="true"><${GlobeIcon} /></span>
                    <div className=${`language-switch-track ${locale === "en" ? "is-en" : "is-zh"}`}>
                      <span className="language-switch-thumb" aria-hidden="true"></span>
                      <button className=${`language-toggle ${locale === "zh" ? "active" : ""}`} aria-pressed=${locale === "zh"} title=${t("languageOptionZh")} onClick=${() => setLocale("zh")}>中</button>
                      <button className=${`language-toggle ${locale === "en" ? "active" : ""}`} aria-pressed=${locale === "en"} title=${t("languageOptionEn")} onClick=${() => setLocale("en")}>EN</button>
                    </div>
                  </div>
                  <button
                    className="sidebar-toggle-button"
                    aria-label=${t("collapseSidebar")}
                    title=${t("collapseSidebar")}
                    onClick=${() => setIsSidebarCollapsed(true)}
                  >
                    <span className="sidebar-toggle-mark"><${SidebarToggleIcon} /></span>
                  </button>
                </div>
              </div>
            </div>
            <nav className="workspace-nav" aria-label="Workspace">
              <${WorkspaceGroup} title=${t("computerSection")} count=${1}>
                <${WorkspaceComputerRow}
                  title=${t("localComputer")}
                  active=${activePane.type === "computer"}
                  subtitle=${`${agentItems.length} ${t("computerAgentsSection")}`}
                  onSelect=${selectComputer}
                />
              <//>
              <${WorkspaceGroup} title=${t("computerAgentsSection")} count=${agentItems.length} onAdd=${openCreateAgentModal} addLabel=${t("createAgent")}>
                ${agentItems.length
                  ? agentItems.map((item) => html`
                      <${WorkspaceAgentRow}
                        key=${item.id}
                        item=${item}
                        active=${activePane.type === "agent" && activePane.id === item.id}
                        t=${t}
                        onSelect=${selectAgent}
                        onPreview=${openAgentPreview}
                      />
                    `)
                  : html`<div className="workspace-empty">${t("noAgents")}</div>`}
              <//>
              <${WorkspaceGroup} title=${t("channelsSection")} count=${channels.length} onAdd=${() => openCreateRoomModal()} addLabel=${t("createRoom")}>
                ${channels.length
                  ? channels.map((conversation) => html`
                      <${WorkspaceConversationRow}
                        key=${conversation.id}
                        conversation=${conversation}
                        active=${activePane.type === "conversation" && activePane.id === conversation.id}
                        currentUserID=${data.current_user_id}
                        usersById=${usersById}
                        locale=${locale}
                        t=${t}
                        onSelect=${selectConversation}
                        onPreviewUser=${openParticipantPreview}
                      />
                    `)
                  : html`<div className="workspace-empty">${t("noChannels")}</div>`}
              <//>
              <${WorkspaceGroup} title=${t("directMessagesSection")} count=${directMessages.length}>
                ${directMessages.length
                  ? directMessages.map((conversation) => html`
                      <${WorkspaceConversationRow}
                        key=${conversation.id}
                        conversation=${conversation}
                        active=${activePane.type === "conversation" && activePane.id === conversation.id}
                        currentUserID=${data.current_user_id}
                        usersById=${usersById}
                        locale=${locale}
                        t=${t}
                        onSelect=${selectConversation}
                        onPreviewUser=${openParticipantPreview}
                      />
                    `)
                  : html`<div className="workspace-empty">${t("noDirectMessages")}</div>`}
              <//>
              ${agentsError ? html`<div className="form-error agent-error">${agentsError}</div>` : null}
            </nav>
          </aside>

          <div
            className=${`sidebar-rail ${isSidebarCollapsed ? "visible" : ""}`}
            aria-hidden=${!isSidebarCollapsed}
            inert=${!isSidebarCollapsed}
          >
            <button className="sidebar-expand-button" aria-label=${t("expandSidebar")} title=${t("expandSidebar")} onClick=${() => setIsSidebarCollapsed(false)}>
              <span className="sidebar-toggle-mark"><${SidebarToggleIcon} /></span>
            </button>
            <nav className="sidebar-rail-nav" aria-label="Workspace">
              <button className=${`sidebar-rail-button ${activePane.type === "computer" ? "active" : ""}`} aria-label=${t("localComputer")} title=${t("localComputer")} onClick=${selectComputer}>
                <span className="sidebar-rail-icon" aria-hidden="true"><${ComputerIcon} /></span>
              </button>
              <button className="sidebar-rail-button" aria-label=${t("createAgent")} title=${t("createAgent")} onClick=${openCreateAgentModal}>
                <span className="sidebar-rail-icon" aria-hidden="true"><${AgentIcon} /></span>
              </button>
              <button className="sidebar-rail-button" aria-label=${t("createRoom")} title=${t("createRoom")} onClick=${() => openCreateRoomModal()}>
                <span className="sidebar-rail-icon" aria-hidden="true"><${RoomPlusIcon} /></span>
              </button>
            </nav>
          </div>
        </div>

        <main className="chat-panel">
          ${activePane.type === "agent" && selectedAgent
            ? html`
                <${AgentDetailPane}
                  item=${selectedAgent}
                  t=${t}
                  activeRoom=${activeChannel}
                  busyKey=${agentActionBusy}
                  error=${agentsError}
                  draft=${agentPageDraft}
                  models=${agentPageModels}
                  modelBusy=${agentPageModelBusy}
                  saving=${agentPageBusy}
                  saveError=${agentPageError}
                  onDraftChange=${setAgentPageDraft}
                  onSave=${saveAgentPage}
                  onStart=${(item) => runAgentAction(item, "start")}
                  onStop=${(item) => runAgentAction(item, "stop")}
                  onRecreate=${(item) => runAgentAction(item, "recreate")}
                  onDelete=${(item) => runAgentAction(item, "delete")}
                  onInvite=${inviteAgentToRoom}
                  onOpenDM=${openAgentDirectMessage}
                />
              `
            : activePane.type === "computer"
              ? html`
                  <${ComputerDetailPane}
                    t=${t}
                    agents=${agentItems}
                    channels=${channels}
                    directMessages=${directMessages}
                    activeAgentID=${activePane.type === "agent" ? activePane.id : ""}
                    busyKey=${agentActionBusy}
                    onSelectAgent=${selectAgent}
                    onCreateAgent=${openCreateAgentModal}
                    onStartAgent=${(item) => runAgentAction(item, "start")}
                  />
                `
              : selectedConversation
            ? html`
                <header className="chat-header">
                  <div className="chat-header-main">
                    <div className="chat-title-bar">
                      <div className="chat-title-row">
                        <div className="chat-title-group">
                          <div className="chat-title truncate">${activeConversation.title}</div>
                          <div ref=${memberMenuRef} className="header-menu">
                            <button
                              className=${`member-badge-button ${showMemberList ? "active" : ""}`}
                              aria-label=${t("membersTitle")}
                              aria-pressed=${showMemberList}
                              title=${t("membersTitle")}
                              onClick=${() => {
                                setShowMemberList((value) => !value);
                                setShowChannelTools(false);
                              }}
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
                                          <button
                                            type="button"
                                            className="avatar avatar-button"
                                            style=${{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}
                                            aria-label=${`${t("profilePreview")} ${user.name}`}
                                            onClick=${() => openParticipantPreview(user)}
                                          >${user.avatar}</button>
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
                        <div ref=${channelToolsRef} className="header-menu tools-menu">
                          <button
                            className=${`icon-button ${showChannelTools ? "active" : ""}`}
                            aria-label=${t("channelTools")}
                            aria-expanded=${showChannelTools}
                            title=${t("channelTools")}
                            onClick=${() => {
                              setShowChannelTools((value) => !value);
                              setShowMemberList(false);
                            }}
                          >
                            <span className="icon-button-mark"><${WrenchIcon} /></span>
                          </button>
                          ${showChannelTools
                            ? html`
                                <div className="header-popover tools-popover">
                                  <div className="header-popover-title">${t("channelTools")}</div>
                                  <button className="tool-menu-row" onClick=${() => setShowToolCalls((value) => !value)}>
                                    <span>${showToolCalls ? t("toggleToolCallsHide") : t("toggleToolCallsShow")}</span>
                                    <strong>${showToolCalls ? t("enabled") : t("disabled")}</strong>
                                  </button>
                                  ${!isDirectConversation(activeConversation)
                                    ? html`
                                        <button
                                          className="tool-menu-row danger"
                                          onClick=${() => {
                                            setShowChannelTools(false);
                                            deleteRoom(activeConversation.id);
                                          }}
                                        >
                                          <span>${t("deleteRoom")}</span>
                                          <span className="tool-menu-icon" aria-hidden="true"><${TrashIcon} /></span>
                                        </button>
                                      `
                                    : null}
                                </div>
                              `
                            : null}
                        </div>
                        <button
                          className="icon-button"
                          aria-label=${inviteActionLabel}
                          title=${inviteActionLabel}
                          onClick=${handleInviteAction}
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
                    if (!user) {
                      return null;
                    }
                    const own = message.sender_id === data.current_user_id;
                    const isAdmin = user?.role === "admin";
                    return html`
                      <div key=${message.id} className=${`message-row ${own ? "own" : ""} ${isAdmin ? "admin" : ""}`.trim()}>
                        <button
                          type="button"
                          className="avatar avatar-button"
                          style=${{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}
                          aria-label=${`${t("profilePreview")} ${user.name}`}
                          onClick=${() => openParticipantPreview(user)}
                        >${user.avatar}</button>
                        <div className="message-card">
                          <div className="message-meta">
                            <span className="message-author">${user.name}</span>
                            <span>${formatTime(message.created_at, locale)}</span>
                          </div>
                          <div className="message-bubble"><${MessageContent} key=${`${message.id}:${theme}`} content=${message.content} /></div>
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
                              <span className="avatar" style=${{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}>${user.avatar}</span>
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
                      ${draftSegments.length === 0
                        ? html`<div className="composer-placeholder" aria-hidden="true">${managerProfileIncomplete ? t("profileIncomplete") : t("inputPlaceholder")}</div>`
                        : null}
                      <div
                        ref=${editorRef}
                        className=${`composer-editor ${managerProfileIncomplete ? "disabled" : ""}`}
                        contentEditable=${managerProfileIncomplete ? "false" : "true"}
                        suppressContentEditableWarning=${true}
                        aria-label=${t("inputPlaceholder")}
                        onInput=${syncComposerFromEditor}
                        onClick=${syncComposerFromEditor}
                        onKeyDown=${onComposerKeyDown}
                        onKeyUp=${syncComposerFromEditor}
                        onPaste=${(event) => {
                          event.preventDefault();
                          insertPlainTextAtSelection(event.clipboardData?.getData("text/plain") ?? "");
                          syncComposerFromEditor();
                        }}
                      />
                      <button
                        type="button"
                        className="composer-send-button"
                        aria-label=${t("send")}
                        title=${t("send")}
                        disabled=${managerProfileIncomplete || !draftText.trim()}
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

      ${profilePreview && (previewAgent || previewUser)
        ? html`
            <${ProfilePreviewDrawer}
              agent=${previewAgent}
              user=${previewUser}
              t=${t}
              activeRoom=${activeChannel}
              onClose=${closeProfilePreview}
              onOpenAgent=${(item) => {
                selectAgent(item);
                closeProfilePreview();
              }}
              onInvite=${inviteAgentToRoom}
              onOpenDM=${openAgentDirectMessage}
            />
          `
        : null}

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
                          disabled=${lockedRoomMemberIDs.includes(user.id)}
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

      ${showAgentModal && agentDraft
        ? html`
            <div className="modal-backdrop" onClick=${() => setShowAgentModal(false)}>
              <div className="modal-card profile-modal agent-modal" onClick=${(event) => event.stopPropagation()}>
                <div className="modal-header">
                  <div>
                    <div className="modal-title">${agentModalMode === "create" ? t("createAgentTitle") : t("editAgentTitle")}</div>
                    <div className="modal-subtitle">${agentModalMode === "create" ? t("createAgentSubtitle") : t("editAgentSubtitle")}</div>
                  </div>
                  <button className="modal-close" onClick=${() => setShowAgentModal(false)}>${t("close")}</button>
                </div>
                <div className="profile-editor-shell">
                  <section className="profile-section">
                    <div className="profile-section-title">${t("profileBasics")}</div>
                    <div className="profile-grid profile-grid-compact">
                      <label className="field">
                        <span>${t("agentName")}</span>
                        <input
                          value=${agentDraft.name}
                          disabled=${agentModalMode === "edit" && editingAgent?.id === "u-manager"}
                          onInput=${(event) => setAgentDraft({ ...agentDraft, name: event.target.value })}
                          placeholder=${t("agentNamePlaceholder")}
                        />
                      </label>
                      <label className="field">
                        <span>${t("agentImage")}</span>
                        <input value=${agentDraft.image} onInput=${(event) => setAgentDraft({ ...agentDraft, image: event.target.value })} placeholder=${t("agentImagePlaceholder")} />
                      </label>
                      <label className="field span-2">
                        <span>${t("agentDescription")}</span>
                        <textarea className="compact-textarea" value=${agentDraft.description} onInput=${(event) => setAgentDraft({ ...agentDraft, description: event.target.value })} />
                      </label>
                    </div>
                  </section>
                  <section className="profile-section">
                    <div className="profile-section-title">${t("profileRuntime")}</div>
                    <div className="profile-runtime-grid">
                      <label className="field">
                        <span>${t("profileProvider")}</span>
                        <select
                          value=${agentDraft.provider}
                          onChange=${(event) => {
                            const next = { ...agentDraft, provider: event.target.value, model_id: "" };
                            setAgentDraft(next);
                            setAgentModels([]);
                          }}
                        >
                          ${["csghub_lite", "codex", "claude_code", "api"].map((provider) => html`
                            <option key=${provider} value=${provider}>${formatProviderLabel(provider)}</option>
                          `)}
                        </select>
                      </label>
                      <label className="field">
                        <span>${t("profileModel")}</span>
                        <select value=${agentDraft.model_id} onChange=${(event) => setAgentDraft({ ...agentDraft, model_id: event.target.value })}>
                          <option value="">${agentModelBusy ? t("profileLoadingModels") : t("profileSelectModel")}</option>
                          ${agentModels.map((model) => html`<option key=${model} value=${model}>${model}</option>`)}
                          ${agentDraft.model_id && !agentModels.includes(agentDraft.model_id)
                            ? html`<option value=${agentDraft.model_id}>${agentDraft.model_id}</option>`
                            : null}
                        </select>
                      </label>
                      <label className="field">
                        <span>${t("profileReasoning")}</span>
                        <select
                          value=${agentDraft.reasoning_effort}
                          onChange=${(event) => setAgentDraft({ ...agentDraft, reasoning_effort: event.target.value })}
                        >
                          ${["low", "medium", "high", "xhigh"].map((effort) => html`<option key=${effort} value=${effort}>${effort}</option>`)}
                        </select>
                      </label>
                      <label className="selection-item compact-toggle-row">
                        <input type="checkbox" checked=${agentDraft.enable_fast_mode} onChange=${() => setAgentDraft({ ...agentDraft, enable_fast_mode: !agentDraft.enable_fast_mode })} />
                        <span>${t("profileFastMode")}</span>
                      </label>
                    </div>
                  </section>
                  ${agentDraft.provider === "api"
                    ? html`
                        <section className="profile-section">
                          <div className="profile-section-title">${t("profileAPIProvider")}</div>
                          <div className="profile-api-grid">
                            <label className="field">
                              <span>${t("profileBaseURL")}</span>
                              <input value=${agentDraft.base_url} onInput=${(event) => setAgentDraft({ ...agentDraft, base_url: event.target.value })} placeholder="https://api.openai.com/v1" />
                            </label>
                            <label className="field">
                              <span>${t("profileAPIKey")}</span>
                              <input value=${agentDraft.api_key} onInput=${(event) => setAgentDraft({ ...agentDraft, api_key: event.target.value })} placeholder=${editingAgent?.agent_profile?.api_key_set ? "Stored key will be kept if blank" : "sk-..."} />
                            </label>
                            <label className="field span-2">
                              <span>${t("profileHeaders")}</span>
                              <textarea className="compact-textarea" value=${agentDraft.headersText} onInput=${(event) => setAgentDraft({ ...agentDraft, headersText: event.target.value })} />
                            </label>
                          </div>
                        </section>
                      `
                    : null}
                  <section className="profile-section">
                    <div className="profile-section-title">${t("profileAdvanced")}</div>
                    <div className="profile-advanced-grid">
                      <label className="field">
                        <span>${t("profileRequestOptions")}</span>
                        <textarea className="compact-json" value=${agentDraft.requestOptionsText} onInput=${(event) => setAgentDraft({ ...agentDraft, requestOptionsText: event.target.value })} />
                      </label>
                      <div className="field">
                        <span>${t("profileEnv")}</span>
                        <${EnvKeyValueEditor}
                          rows=${agentDraft.envRows}
                          t=${t}
                          onChange=${(rows) => setAgentDraft({ ...agentDraft, envRows: rows })}
                        />
                      </div>
                    </div>
                  </section>
                </div>
                ${agentError ? html`<div className="form-error">${agentError}</div>` : null}
                <div className="modal-actions">
                  <button className="secondary-button" onClick=${() => setShowAgentModal(false)}>${t("cancel")}</button>
                  <button className="send-button" disabled=${agentBusy || !agentDraft.name.trim() || !agentDraft.model_id} onClick=${saveAgent}>
                    ${agentBusy ? "..." : agentModalMode === "create" ? t("agentCreateSave") : t("agentUpdateSave")}
                  </button>
                </div>
              </div>
            </div>
          `
        : null}

      ${managerProfileIncomplete && profileDraft
        ? html`
            <div className="modal-backdrop profile-backdrop nonblocking">
              <div className="modal-card profile-modal" onClick=${(event) => event.stopPropagation()}>
                <div className="modal-header">
                  <div>
                    <div className="modal-title">${t("profileSetupTitle")}</div>
                    <div className="modal-subtitle">${t("profileSetupSubtitle")}</div>
                  </div>
                </div>
                ${managerProfile?.detection_results?.length
                  ? html`
                      <div className="detection-list">
                        <div className="section-label">${t("detectionResults")}</div>
                        ${managerProfile.detection_results.map((item) => html`
                          <div key=${item.provider} className=${`detection-row ${item.status === "ok" ? "ok" : "failed"}`}>
                            <span>${formatProviderLabel(item.provider)}</span>
                            <small>${item.status === "ok" ? item.model_id : item.error}</small>
                          </div>
                        `)}
                      </div>
                    `
                  : null}
                <div className="profile-editor-shell">
                  <section className="profile-section">
                    <div className="profile-section-title">${t("profileRuntime")}</div>
                    <div className="profile-runtime-grid">
                      <label className="field">
                        <span>${t("profileProvider")}</span>
                        <select
                          value=${profileDraft.provider}
                          onChange=${(event) => {
                            const next = { ...profileDraft, provider: event.target.value, model_id: "" };
                            setProfileDraft(next);
                            setProfileModels([]);
                          }}
                        >
                          ${["csghub_lite", "codex", "claude_code", "api"].map((provider) => html`
                            <option key=${provider} value=${provider}>${formatProviderLabel(provider)}</option>
                          `)}
                        </select>
                      </label>
                      <label className="field">
                        <span>${t("profileModel")}</span>
                        <select
                          value=${profileDraft.model_id}
                          onChange=${(event) => setProfileDraft({ ...profileDraft, model_id: event.target.value })}
                        >
                          <option value="">${profileModelBusy ? t("profileLoadingModels") : t("profileSelectModel")}</option>
                          ${profileModels.map((model) => html`<option key=${model} value=${model}>${model}</option>`)}
                          ${profileDraft.model_id && !profileModels.includes(profileDraft.model_id)
                            ? html`<option value=${profileDraft.model_id}>${profileDraft.model_id}</option>`
                            : null}
                        </select>
                      </label>
                      <label className="field">
                        <span>${t("profileReasoning")}</span>
                        <select
                          value=${profileDraft.reasoning_effort}
                          onChange=${(event) => setProfileDraft({ ...profileDraft, reasoning_effort: event.target.value })}
                        >
                          ${["low", "medium", "high", "xhigh"].map((effort) => html`<option key=${effort} value=${effort}>${effort}</option>`)}
                        </select>
                      </label>
                      <label className="selection-item compact-toggle-row">
                        <input
                          type="checkbox"
                          checked=${profileDraft.enable_fast_mode}
                          onChange=${() => setProfileDraft({ ...profileDraft, enable_fast_mode: !profileDraft.enable_fast_mode })}
                        />
                        <span>${t("profileFastMode")}</span>
                      </label>
                    </div>
                  </section>
                  ${profileDraft.provider === "api"
                    ? html`
                        <section className="profile-section">
                          <div className="profile-section-title">${t("profileAPIProvider")}</div>
                          <div className="profile-api-grid">
                            <label className="field">
                              <span>${t("profileBaseURL")}</span>
                              <input value=${profileDraft.base_url} onInput=${(event) => setProfileDraft({ ...profileDraft, base_url: event.target.value })} placeholder="https://api.openai.com/v1" />
                            </label>
                            <label className="field">
                              <span>${t("profileAPIKey")}</span>
                              <input value=${profileDraft.api_key} onInput=${(event) => setProfileDraft({ ...profileDraft, api_key: event.target.value })} placeholder=${managerProfile.api_key_set ? "Stored key will be kept if blank" : "sk-..."} />
                            </label>
                            <label className="field span-2">
                              <span>${t("profileHeaders")}</span>
                              <textarea className="compact-textarea" value=${profileDraft.headersText} onInput=${(event) => setProfileDraft({ ...profileDraft, headersText: event.target.value })} />
                            </label>
                          </div>
                        </section>
                      `
                    : null}
                  <section className="profile-section">
                    <div className="profile-section-title">${t("profileAdvanced")}</div>
                    <div className="profile-advanced-grid">
                      <label className="field">
                        <span>${t("profileRequestOptions")}</span>
                        <textarea className="compact-json" value=${profileDraft.requestOptionsText} onInput=${(event) => setProfileDraft({ ...profileDraft, requestOptionsText: event.target.value })} />
                      </label>
                      <div className="field">
                        <span>${t("profileEnv")}</span>
                        <${EnvKeyValueEditor}
                          rows=${profileDraft.envRows}
                          t=${t}
                          onChange=${(rows) => setProfileDraft({ ...profileDraft, envRows: rows })}
                        />
                      </div>
                    </div>
                  </section>
                </div>
                ${profileError ? html`<div className="form-error">${profileError}</div>` : null}
                <div className="modal-actions">
                  <button className="send-button" disabled=${profileBusy || !profileDraft.model_id} onClick=${saveManagerProfile}>
                    ${profileBusy ? "..." : t("profileSave")}
                  </button>
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
        const isDirect = isDirectConversation(conversation);
        const avatar = isDirect && displayUser
          ? displayUser.avatar
          : conversation.title.slice(0, 2).toUpperCase();
        const color = isDirect && displayUser
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

function AgentSection({ title, manager, workers, t, activeRoom, busyKey, error, onCreate, onEdit, onStart, onStop, onRecreate, onDelete, onInvite }) {
  const items = [manager, ...workers].filter(Boolean);
  return html`
    <section className="agent-section">
      <div className="agent-section-head">
        <div>
          <div className="section-label">${title} ${items.length}</div>
        </div>
        <button className="agent-add-button" aria-label=${t("createAgent")} title=${t("createAgent")} onClick=${onCreate}>
          <span aria-hidden="true"><${AgentIcon} /></span>
        </button>
      </div>
      <div className="agent-list">
        ${items.length
          ? items.map((item) => html`
              <${AgentRow}
                key=${item.id}
                item=${item}
                t=${t}
                activeRoom=${activeRoom}
                busyKey=${busyKey}
                onEdit=${onEdit}
                onStart=${onStart}
                onStop=${onStop}
                onRecreate=${onRecreate}
                onDelete=${onDelete}
                onInvite=${onInvite}
              />
            `)
          : html`<div className="agent-empty">${t("noAgents")}</div>`}
      </div>
      ${error ? html`<div className="form-error agent-error">${error}</div>` : null}
    </section>
  `;
}

function AgentRow({ item, t, activeRoom, busyKey, onEdit, onStart, onStop, onRecreate, onDelete, onInvite }) {
  const isManager = item.role === "manager" || item.id === "u-manager";
  const status = String(item.status || "").toLowerCase();
  const running = status === "running" || status === "online";
  const incomplete = item.profile_complete === false || item.agent_profile?.profile_complete === false;
  const restartNeeded = Boolean(item.agent_profile?.env_restart_required);
  const busyPrefix = `${item.id}:`;
  return html`
    <div className=${`agent-row ${isManager ? "manager" : ""} ${incomplete ? "incomplete" : ""}`.trim()}>
      <div className="agent-avatar" aria-hidden="true"><${AgentIcon} /></div>
      <div className="agent-row-main">
        <div className="agent-row-top">
          <span className="agent-name truncate">${item.name}</span>
          <span className=${`agent-status ${running ? "running" : ""}`}>${item.status || "unknown"}</span>
        </div>
        <div className="agent-meta truncate">${formatProviderLabel(item.provider)} · ${item.model_id || item.agent_profile?.model_id || "no model"}</div>
        <div className="agent-badges">
          <span className=${`agent-badge ${incomplete ? "warn" : ""}`}>${incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}</span>
          ${restartNeeded ? html`<span className="agent-badge warn">${t("profileRestartRequired")}</span>` : null}
        </div>
      </div>
      <div className="agent-actions">
        <button className="agent-icon-button" aria-label=${t("editProfile")} title=${t("editProfile")} onClick=${() => onEdit(item)}>
          <span aria-hidden="true"><${WrenchIcon} /></span>
        </button>
        <button className="agent-icon-button" aria-label=${running ? t("agentStop") : t("agentStart")} title=${running ? t("agentStop") : t("agentStart")} disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => running ? onStop(item) : onStart(item)}>
          <span aria-hidden="true">${running ? html`<${StopIcon} />` : html`<${PlayIcon} />`}</span>
        </button>
        <button className="agent-action-text" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>
        ${activeRoom && !isManager
          ? html`<button className="agent-action-text" disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onInvite(item)}>${t("inviteToRoom")}</button>`
          : null}
        ${!isManager
          ? html`
              <button className="agent-icon-button danger" aria-label=${t("agentDelete")} title=${t("agentDelete")} disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onDelete(item)}>
                <span aria-hidden="true"><${TrashIcon} /></span>
              </button>
            `
          : null}
      </div>
    </div>
  `;
}

function WorkspaceGroup({ title, count, onAdd, addLabel, children }) {
  return html`
    <section className="workspace-group">
      <div className="workspace-group-head">
        <div className="workspace-group-title">
          <span>${title}</span>
          <small>${count}</small>
        </div>
        ${onAdd
          ? html`
              <button className="workspace-add-button" aria-label=${addLabel || title} title=${addLabel || title} onClick=${onAdd}>
                <span aria-hidden="true">+</span>
              </button>
            `
          : null}
      </div>
      <div className="workspace-group-items">${children}</div>
    </section>
  `;
}

function WorkspaceComputerRow({ title, active, subtitle, onSelect }) {
  return html`
    <button className=${`workspace-row computer-row ${active ? "active" : ""}`} onClick=${onSelect}>
      <span className="workspace-row-icon"><${ComputerIcon} /></span>
      <span className="workspace-row-main">
        <span className="workspace-row-title truncate">${title}</span>
        <span className="workspace-row-meta truncate">${subtitle}</span>
      </span>
      <span className="workspace-status-dot online" aria-hidden="true"></span>
    </button>
  `;
}

function WorkspaceAgentRow({ item, active, t, onSelect, onPreview }) {
  const incomplete = isAgentIncomplete(item);
  const restartNeeded = isAgentRestartNeeded(item);
  const running = isAgentRunning(item);
  return html`
    <button className=${`workspace-row agent-nav-row ${active ? "active" : ""} ${incomplete ? "warn" : ""}`.trim()} onClick=${() => onSelect(item)}>
      <span
        className="workspace-row-icon workspace-row-icon-clickable"
        role="button"
        tabIndex="0"
        aria-label=${`${t("profilePreview")} ${item.name}`}
        onClick=${(event) => {
          event.stopPropagation();
          onPreview?.(item);
        }}
        onKeyDown=${(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            event.stopPropagation();
            onPreview?.(item);
          }
        }}
      ><${AgentIcon} /></span>
      <span className="workspace-row-main">
        <span className="workspace-row-title-line">
          <span className="workspace-row-title truncate">${item.name}</span>
          <span className=${`workspace-status-dot ${running ? "online" : ""}`} aria-hidden="true"></span>
        </span>
        <span className="workspace-row-meta truncate">${formatProviderLabel(item.provider || item.agent_profile?.provider)} · ${agentModelID(item)}</span>
      </span>
      <span className="workspace-row-badges">
        ${incomplete ? html`<span className="mini-badge warn">${t("profileIncompleteBadge")}</span>` : null}
        ${restartNeeded ? html`<span className="mini-badge warn">${t("profileRestartRequired")}</span>` : null}
      </span>
    </button>
  `;
}

function WorkspaceConversationRow({ conversation, active, currentUserID, usersById, locale, t, onSelect, onPreviewUser }) {
  const lastMessage = conversation.messages[conversation.messages.length - 1];
  const isDirect = isDirectConversation(conversation);
  const displayUser = isDirect ? resolveConversationUser(conversation, currentUserID, usersById) : null;
  const title = isDirect && displayUser ? displayUser.name : conversation.title;
  const icon = isDirect && displayUser ? displayUser.avatar : "#";
  return html`
    <button className=${`workspace-row conversation-nav-row ${active ? "active" : ""}`} onClick=${() => onSelect(conversation.id)}>
      <span
        className=${`workspace-row-icon ${isDirect ? "avatar-icon workspace-row-icon-clickable" : ""}`}
        role=${isDirect ? "button" : undefined}
        tabIndex=${isDirect ? "0" : undefined}
        aria-label=${isDirect && displayUser ? `${t("profilePreview")} ${displayUser.name}` : undefined}
        onClick=${isDirect && displayUser
          ? (event) => {
              event.stopPropagation();
              onPreviewUser?.(displayUser);
            }
          : undefined}
        onKeyDown=${isDirect && displayUser
          ? (event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                event.stopPropagation();
                onPreviewUser?.(displayUser);
              }
            }
          : undefined}
      >${icon}</span>
      <span className="workspace-row-main">
        <span className="workspace-row-title truncate">${title}</span>
        <span className="workspace-row-meta truncate">${formatConversationPreview(lastMessage, conversation, currentUserID, usersById, locale, t)}</span>
      </span>
      <span className="workspace-row-time">${formatTime(lastMessage?.created_at, locale)}</span>
    </button>
  `;
}

function AgentDetailPane({ item, t, activeRoom, busyKey, error, draft, models, modelBusy, saving, saveError, onDraftChange, onSave, onStart, onStop, onRecreate, onDelete, onInvite, onOpenDM }) {
  const isManager = item.role === "manager" || item.id === "u-manager";
  const running = isAgentRunning(item);
  const incomplete = isAgentIncomplete(item);
  const restartNeeded = isAgentRestartNeeded(item);
  const busyPrefix = `${item.id}:`;
  const provider = item.provider || item.agent_profile?.provider;
  const updateDraft = (patch) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch });
  return html`
    <section className="entity-pane agent-detail-pane">
      <header className="entity-header">
        <div className="entity-avatar"><${AgentIcon} /></div>
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>${item.name}</h1>
            <span className=${`status-pill ${running ? "online" : ""}`}>${item.status || "unknown"}</span>
          </div>
          <p>${item.description || item.agent_profile?.description || ""}</p>
        </div>
      </header>
      <div className="entity-toolbar">
        <button className="send-button compact" disabled=${saving || !draft?.name?.trim() || !draft?.model_id} onClick=${onSave}>${saving ? t("profileLoadingModels") : t("agentUpdateSave")}</button>
        <button className="secondary-button" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => running ? onStop(item) : onStart(item)}>
          ${running ? t("agentStop") : t("agentStart")}
        </button>
        <button className="secondary-button" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>
        ${activeRoom && !isManager
          ? html`<button className="secondary-button" disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onInvite(item)}>${t("inviteToRoom")}</button>`
          : null}
        ${!isManager
          ? html`<button className="secondary-button" onClick=${() => onOpenDM(item)}>${t("openDM")}</button>`
          : null}
        ${!isManager
          ? html`<button className="danger-button" disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onDelete(item)}>${t("agentDelete")}</button>`
          : null}
      </div>
      ${error ? html`<div className="form-error">${error}</div>` : null}
      ${saveError ? html`<div className="form-error">${saveError}</div>` : null}
      ${!draft
        ? html`
            <div className="entity-grid">
              <div className="entity-field">
                <span>${t("profileProvider")}</span>
                <strong>${formatProviderLabel(provider)}</strong>
              </div>
              <div className="entity-field">
                <span>${t("profileModel")}</span>
                <strong>${agentModelID(item)}</strong>
              </div>
              <div className="entity-field">
                <span>${t("profileReasoning")}</span>
                <strong>${item.reasoning_effort || item.agent_profile?.reasoning_effort || "medium"}</strong>
              </div>
              <div className="entity-field">
                <span>${t("profileFastMode")}</span>
                <strong>${item.enable_fast_mode || item.agent_profile?.enable_fast_mode ? "on" : "off"}</strong>
              </div>
            </div>
          `
        : null}
      <div className="entity-badge-row">
        <span className=${`agent-badge ${incomplete ? "warn" : ""}`}>${incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}</span>
        ${restartNeeded ? html`<span className="agent-badge warn">${t("profileRestartRequired")}</span>` : null}
      </div>
      ${draft
        ? html`
            <div className="profile-editor-shell agent-page-editor">
              <section className="profile-section">
                <div className="profile-section-title">${t("profileBasics")}</div>
                <div className="profile-grid-compact">
                  <label className="field">
                    <span>${t("agentName")}</span>
                    <input value=${draft.name} onInput=${(event) => updateDraft({ name: event.target.value })} placeholder=${t("agentNamePlaceholder")} />
                  </label>
                  <label className="field">
                    <span>${t("agentImage")}</span>
                    <input value=${draft.image} onInput=${(event) => updateDraft({ image: event.target.value })} placeholder=${t("agentImagePlaceholder")} />
                  </label>
                  <label className="field span-2">
                    <span>${t("agentDescription")}</span>
                    <textarea className="compact-textarea" value=${draft.description} onInput=${(event) => updateDraft({ description: event.target.value })} />
                  </label>
                </div>
              </section>

              <section className="profile-section">
                <div className="profile-section-title">${t("profileRuntime")}</div>
                <div className="profile-runtime-grid">
                  <label className="field">
                    <span>${t("profileProvider")}</span>
                    <select
                      value=${draft.provider}
                      onChange=${(event) => updateDraft({ provider: event.target.value, model_id: "" })}
                    >
                      ${PROVIDERS.map((provider) => html`<option key=${provider} value=${provider}>${formatProviderLabel(provider)}</option>`)}
                    </select>
                  </label>
                  <label className="field">
                    <span>${t("profileModel")}</span>
                    <select value=${draft.model_id} onChange=${(event) => updateDraft({ model_id: event.target.value })}>
                      <option value="">${modelBusy ? t("profileLoadingModels") : t("profileSelectModel")}</option>
                      ${models.map((model) => html`<option key=${model} value=${model}>${model}</option>`)}
                      ${draft.model_id && !models.includes(draft.model_id)
                        ? html`<option value=${draft.model_id}>${draft.model_id}</option>`
                        : null}
                    </select>
                  </label>
                  <label className="field">
                    <span>${t("profileReasoning")}</span>
                    <select value=${draft.reasoning_effort} onChange=${(event) => updateDraft({ reasoning_effort: event.target.value })}>
                      ${REASONING_EFFORTS.map((effort) => html`<option key=${effort} value=${effort}>${effort}</option>`)}
                    </select>
                  </label>
                  <label className="selection-item compact-toggle-row">
                    <input type="checkbox" checked=${draft.enable_fast_mode} onChange=${() => updateDraft({ enable_fast_mode: !draft.enable_fast_mode })} />
                    <span>${t("profileFastMode")}</span>
                  </label>
                </div>
              </section>

              ${draft.provider === "api"
                ? html`
                    <section className="profile-section">
                      <div className="profile-section-title">${t("profileAPIProvider")}</div>
                      <div className="profile-api-grid">
                        <label className="field">
                          <span>${t("profileBaseURL")}</span>
                          <input value=${draft.base_url} onInput=${(event) => updateDraft({ base_url: event.target.value })} placeholder="https://api.openai.com/v1" />
                        </label>
                        <label className="field">
                          <span>${t("profileAPIKey")}</span>
                          <input value=${draft.api_key} onInput=${(event) => updateDraft({ api_key: event.target.value })} placeholder=${item.agent_profile?.api_key_set ? "Stored key will be kept if blank" : "sk-..."} />
                        </label>
                        <label className="field span-2">
                          <span>${t("profileHeaders")}</span>
                          <textarea className="compact-textarea" value=${draft.headersText} onInput=${(event) => updateDraft({ headersText: event.target.value })} />
                        </label>
                      </div>
                    </section>
                  `
                : null}

              <section className="profile-section">
                <div className="profile-section-title">${t("profileAdvanced")}</div>
                <div className="profile-advanced-grid">
                  <label className="field">
                    <span>${t("profileRequestOptions")}</span>
                    <textarea className="compact-json" value=${draft.requestOptionsText} onInput=${(event) => updateDraft({ requestOptionsText: event.target.value })} />
                  </label>
                  <div className="field">
                    <span>${t("profileEnv")}</span>
                    <${EnvKeyValueEditor}
                      rows=${draft.envRows}
                      t=${t}
                      onChange=${(rows) => updateDraft({ envRows: rows })}
                    />
                  </div>
                </div>
              </section>
            </div>
          `
        : null}
    </section>
  `;
}

function ProfilePreviewDrawer({ agent, user, t, activeRoom, onClose, onOpenAgent, onInvite, onOpenDM }) {
  const running = agent ? isAgentRunning(agent) : false;
  const incomplete = agent ? isAgentIncomplete(agent) : false;
  const restartNeeded = agent ? isAgentRestartNeeded(agent) : false;
  const provider = agent?.provider || agent?.agent_profile?.provider;
  const displayName = agent?.name || user?.name || "";
  const displayRole = agent ? (agent.role || "worker") : user?.role;
  return html`
    <aside className="profile-preview-drawer" aria-label=${t("profilePreview")}>
      <div className="preview-header">
        <div className="preview-title">${agent ? t("profilePreview") : t("personProfile")}</div>
        <button className="modal-close" aria-label=${t("close")} onClick=${onClose}>
          <span aria-hidden="true">×</span>
        </button>
      </div>
      <div className="preview-hero">
        ${agent
          ? html`<div className="entity-avatar preview-avatar"><${AgentIcon} /></div>`
          : html`<div className="avatar preview-avatar" style=${{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}>${user.avatar}</div>`}
        <div className="preview-identity">
          <div className="preview-name">${displayName}</div>
          <div className="preview-meta">@${user?.handle || agent?.id || ""} · ${localizeRole(displayRole, t)}</div>
        </div>
      </div>
      ${agent?.description || user?.name
        ? html`<p className="preview-description">${agent?.description || ""}</p>`
        : null}
      ${agent
        ? html`
            <div className="preview-fields">
              <div className="entity-field">
                <span>${t("status")}</span>
                <strong>${agent.status || "unknown"}</strong>
              </div>
              <div className="entity-field">
                <span>${t("profileProvider")}</span>
                <strong>${formatProviderLabel(provider)}</strong>
              </div>
              <div className="entity-field">
                <span>${t("profileModel")}</span>
                <strong>${agentModelID(agent)}</strong>
              </div>
              <div className="entity-field">
                <span>${t("profileReasoning")}</span>
                <strong>${agent.reasoning_effort || agent.agent_profile?.reasoning_effort || "medium"}</strong>
              </div>
            </div>
            <div className="entity-badge-row">
              <span className=${`agent-badge ${running ? "" : "warn"}`}>${running ? t("online") : t("offline")}</span>
              <span className=${`agent-badge ${incomplete ? "warn" : ""}`}>${incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}</span>
              ${restartNeeded ? html`<span className="agent-badge warn">${t("profileRestartRequired")}</span>` : null}
            </div>
            <div className="preview-actions">
              <button className="send-button compact" onClick=${() => onOpenAgent(agent)}>${t("openProfile")}</button>
              <button className="secondary-button" onClick=${() => onOpenDM(agent)}>${t("openDM")}</button>
              ${activeRoom && agent.role !== "manager" && agent.id !== "u-manager"
                ? html`<button className="secondary-button" onClick=${() => onInvite(agent)}>${t("inviteToRoom")}</button>`
                : null}
            </div>
          `
        : html`
            <div className="preview-fields">
              <div className="entity-field">
                <span>${t("status")}</span>
                <strong>${t("online")}</strong>
              </div>
              <div className="entity-field">
                <span>${t("roleLabel")}</span>
                <strong>${localizeRole(user?.role, t)}</strong>
              </div>
              <div className="entity-field">
                <span>${t("handleLabel")}</span>
                <strong>${user?.handle ? `@${user.handle}` : "-"}</strong>
              </div>
              <div className="entity-field">
                <span>${t("userIDLabel")}</span>
                <strong>${user?.id || ""}</strong>
              </div>
            </div>
          `}
    </aside>
  `;
}

function ComputerDetailPane({ t, agents, channels, directMessages, busyKey, onSelectAgent, onCreateAgent, onStartAgent }) {
  const runningAgents = agents.filter(isAgentRunning);
  return html`
    <section className="entity-pane computer-detail-pane">
      <header className="entity-header">
        <div className="entity-avatar"><${ComputerIcon} /></div>
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>${t("localComputer")}</h1>
            <span className="status-pill online">${t("online")}</span>
          </div>
          <p>${t("computerOverview")}</p>
        </div>
      </header>
      <div className="metric-row">
        <div className="metric"><span>${t("computerAgentsSection")}</span><strong>${agents.length}</strong></div>
        <div className="metric"><span>${t("activeNow")}</span><strong>${runningAgents.length}</strong></div>
        <div className="metric"><span>${t("channelsSection")}</span><strong>${channels.length}</strong></div>
        <div className="metric"><span>${t("directMessagesSection")}</span><strong>${directMessages.length}</strong></div>
      </div>
      <div className="section-header-inline">
        <div className="section-label">${t("computerAgentsSection")}</div>
        <button className="send-button compact" onClick=${onCreateAgent}>${t("createAgent")}</button>
      </div>
      <div className="entity-list">
        ${agents.length
          ? agents.map((item) => html`
              <div key=${item.id} className="entity-list-row">
                <button className="entity-list-main-button" onClick=${() => onSelectAgent(item)}>
                  <span className="entity-list-icon"><${AgentIcon} /></span>
                  <span className="entity-list-main">
                    <strong>${item.name}</strong>
                    <small>${formatProviderLabel(item.provider || item.agent_profile?.provider)} · ${agentModelID(item)}</small>
                  </span>
                  <span className=${`workspace-status-dot ${isAgentRunning(item) ? "online" : ""}`}></span>
                </button>
                <button
                  className="agent-icon-button"
                  disabled=${busyKey.startsWith(`${item.id}:`) || isAgentIncomplete(item)}
                  onClick=${() => onStartAgent(item)}
                >
                  <span aria-hidden="true"><${PlayIcon} /></span>
                </button>
              </div>
            `)
          : html`<div className="agent-empty">${t("noAgents")}</div>`}
      </div>
    </section>
  `;
}

function EnvKeyValueEditor({ rows = [], t, onChange }) {
  const items = rows.length ? rows : [{ key: "", value: "" }];
  function update(index, patch) {
    onChange(items.map((row, rowIndex) => rowIndex === index ? { ...row, ...patch } : row));
  }
  function remove(index) {
    const next = items.filter((_, rowIndex) => rowIndex !== index);
    onChange(next.length ? next : [{ key: "", value: "" }]);
  }
  return html`
    <div className="env-editor">
      ${items.map((row, index) => html`
        <div key=${index} className="env-row">
          <input
            value=${row.key}
            placeholder=${t("profileEnvKey")}
            onInput=${(event) => update(index, { key: event.target.value })}
          />
          <input
            value=${row.value}
            placeholder=${t("profileEnvValue")}
            onInput=${(event) => update(index, { value: event.target.value })}
          />
          <button type="button" className="env-remove-button" aria-label=${t("profileEnvRemove")} title=${t("profileEnvRemove")} onClick=${() => remove(index)}>
            ×
          </button>
        </div>
      `)}
      <button type="button" className="secondary-button env-add-button" onClick=${() => onChange([...items, { key: "", value: "" }])}>
        ${t("profileEnvAdd")}
      </button>
    </div>
  `;
}

function detectInitialLocale() {
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (stored === "zh" || stored === "en") {
    return stored;
  }
  return navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en";
}

function detectInitialTheme() {
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") {
    return stored;
  }
  return "dark";
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
    return flattenMentionText(message.content);
  }
  return getConversationSubtitle(conversation, currentUserID, usersById, locale, t);
}

const mentionMarkupPattern = /<at\s+user_id="([^"]+)">([\s\S]*?)<\/at>/g;

function flattenMentionText(content) {
  return String(content ?? "").replace(mentionMarkupPattern, (_, __, name) => `@${name}`);
}

function decorateMentionMarkup(content) {
  return String(content ?? "").replace(mentionMarkupPattern, (_, userID, name) => {
    const safeUserID = escapeHTML(userID);
    const safeName = escapeHTML(name);
    return `<span class="message-mention" data-user-id="${safeUserID}">@${safeName}</span>`;
  });
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
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
    const targets = (message.event.target_ids || mentionIDs(message.mentions) || []).map((id) => userDisplayName(id, usersById)).filter(Boolean);
    if (targets.length > 0) {
      return locale === "zh"
        ? `${actor} 邀请 ${targets.join("、")} 加入了房间`
        : `${actor} invited ${targets.join(", ")} to join the room`;
    }
  }
  return message.content || "";
}

function mentionIDs(mentions) {
  return (mentions || []).map((mention) => {
    if (typeof mention === "string") {
      return mention;
    }
    return mention?.id || "";
  }).filter(Boolean);
}

function isLegacySystemEventContent(content) {
  const text = (content ?? "").trim();
  if (!text) {
    return false;
  }
  return [
    /^.+ invited .+ to join the room\.?$/,
    /^.+ invited .+ to join the channel\.?$/,
    /^.+ created the room ".+"\.?$/,
    /^.+ created the channel ".+"\.?$/,
    /^.+ 邀请 .+ 加入了房间。?$/,
    /^.+ 邀请 .+ 加入了频道。?$/,
    /^.+ 创建了房间“.+”。?$/,
    /^.+ 创建了频道“.+”。?$/,
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
  const otherID = conversation.members.find((id) => id !== currentUserID) ?? currentUserID;
  return usersById.get(otherID);
}

function agentMatchesUser(agent, user) {
  if (!agent || !user) {
    return false;
  }
  const agentHandle = normalizeComparable(agent.handle);
  const userHandle = normalizeComparable(user.handle);
  const agentName = normalizeComparable(agent.name);
  const userName = normalizeComparable(user.name);
  return agent.id === user.id ||
    agent.user_id === user.id ||
    Boolean(agentHandle && userHandle && agentHandle === userHandle) ||
    Boolean(agentName && userName && agentName === userName);
}

function normalizeComparable(value) {
  return String(value || "").trim().toLowerCase();
}

function isDirectConversation(conversation) {
  return Boolean(conversation?.is_direct);
}

function getConversationSubtitle(conversation, currentUserID, usersById, locale, t) {
  return "";
}

function getConversationDescription(conversation, currentUserID, usersById, locale, t) {
  if (isDirectConversation(conversation)) {
    return "";
  }
  return conversation.description || "";
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
  if (event.type === "user.deleted" && event.user) {
    return removeUserFromData(current, event.user.id);
  }
  if (event.type === "message.created" && event.message) {
    return appendMessageToData(current, event.room_id, event.message);
  }
  if ((event.type === "conversation.created" || event.type === "conversation.members_added" || event.type === "room.created" || event.type === "room.members_added") && event.room) {
    return upsertConversationInData(current, event.room);
  }
  return current;
}

function isAgentRosterEvent(event) {
  if (!event?.type) {
    return false;
  }
  if (event.type === "user.created" || event.type === "user.deleted") {
    return true;
  }
  if (event.type === "conversation.created" || event.type === "room.created") {
    return Boolean(event.room?.is_direct);
  }
  return false;
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

function removeUserFromData(current, userID) {
  if (!current || !userID) {
    return current;
  }

  const users = current.users.filter((item) => item.id !== userID);
  const rooms = current.rooms
    .map((room) => {
      const members = room.members.filter((id) => id !== userID);
      const messages = room.messages.filter((message) => message.sender_id !== userID);
      if (members.length < 2) {
        return null;
      }
      return {
        ...room,
        members,
        messages,
      };
    })
    .filter(Boolean);

  return { ...current, users, rooms: sortConversations(rooms) };
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

function profileToDraft(profile) {
  return {
    provider: profile?.provider || "csghub_lite",
    base_url: profile?.base_url || "",
    api_key: "",
    model_id: profile?.model_id || "",
    reasoning_effort: profile?.reasoning_effort || "medium",
    enable_fast_mode: Boolean(profile?.enable_fast_mode),
    headersText: stringifyJSON(profile?.headers || {}),
    requestOptionsText: stringifyJSON(profile?.request_options || {}),
    envRows: mapToEnvRows(profile?.env || {}),
  };
}

function agentToDraft(agent) {
  const profile = agent?.agent_profile || agent || {};
  return {
    name: agent?.name || "",
    description: agent?.description || profile.description || "",
    image: agent?.image || "",
    ...profileToDraft(profile),
  };
}

function draftToProfile(draft, options = {}) {
  return {
    name: options.name || draft.name || "manager",
    description: options.description || draft.description || "Manager Worker Dispatch",
    provider: draft.provider,
    base_url: draft.base_url,
    api_key: draft.api_key,
    model_id: draft.model_id,
    reasoning_effort: draft.reasoning_effort || "medium",
    enable_fast_mode: Boolean(draft.enable_fast_mode),
    headers: parseJSONMap(draft.headersText),
    request_options: parseJSONMap(draft.requestOptionsText),
    env: envRowsToMap(draft.envRows),
  };
}

function mapToEnvRows(value) {
  const object = value && typeof value === "object" && !Array.isArray(value) ? value : {};
  const entries = Object.entries(object).sort(([left], [right]) => left.localeCompare(right));
  if (entries.length === 0) {
    return [{ key: "", value: "" }];
  }
  return entries.map(([key, val]) => ({ key, value: String(val ?? "") }));
}

function envRowsToMap(rows) {
  const result = {};
  const seen = new Set();
  for (const row of rows ?? []) {
    const key = String(row?.key ?? "").trim();
    const value = String(row?.value ?? "");
    if (!key && !value.trim()) {
      continue;
    }
    if (!key) {
      throw new Error("Environment variable key is required");
    }
    const normalized = key.toUpperCase();
    if (seen.has(normalized)) {
      throw new Error(`Duplicate environment variable: ${key}`);
    }
    seen.add(normalized);
    result[key] = value;
  }
  return result;
}

function isAgentRunning(item) {
  const status = String(item?.status || "").toLowerCase();
  return status === "running" || status === "online";
}

function isAgentIncomplete(item) {
  return item?.profile_complete === false || item?.agent_profile?.profile_complete === false;
}

function isAgentRestartNeeded(item) {
  return Boolean(item?.env_restart_required || item?.agent_profile?.env_restart_required);
}

function agentModelID(item) {
  return item?.model_id || item?.agent_profile?.model_id || "no model";
}

function stringifyJSON(value) {
  const object = value && typeof value === "object" ? value : {};
  return JSON.stringify(object, null, 2);
}

function parseJSONMap(text) {
  const cleaned = String(text ?? "").trim();
  if (!cleaned) {
    return {};
  }
  const parsed = JSON.parse(cleaned);
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error("Expected a JSON object");
  }
  return parsed;
}

function formatProviderLabel(provider) {
  switch (provider) {
    case "csghub_lite":
      return "CSGHub Lite";
    case "codex":
      return "Codex";
    case "claude_code":
      return "Claude Code";
    case "api":
      return "OpenAI API";
    default:
      return provider || "";
  }
}

function toggleSelection(current, id) {
  return current.includes(id) ? current.filter((item) => item !== id) : [...current, id];
}

function renderMarkdown(content) {
  const raw = marked.parse(decorateMentionMarkup(content));
  return DOMPurify.sanitize(raw, {
    USE_PROFILES: { html: true },
    ADD_ATTR: ["target", "rel", "class", "data-user-id"],
  });
}

function createMentionTokenElement(user) {
  const token = document.createElement("span");
  token.className = "composer-mention-token";
  token.dataset.userId = user.id;
  token.dataset.userName = user.name || user.handle || user.id;
  token.contentEditable = "false";
  token.textContent = `@${token.dataset.userName}`;
  return token;
}

function renderComposerSegments(root, segments) {
  if (!root) {
    return;
  }
  root.replaceChildren();
  for (const segment of segments ?? []) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      root.append(createMentionTokenElement({
        id: segment.userId,
        name: segment.userName,
        handle: segment.userName,
      }));
      continue;
    }
    const parts = String(segment.text ?? "").split("\n");
    parts.forEach((part, index) => {
      if (part) {
        root.append(document.createTextNode(part));
      }
      if (index < parts.length - 1) {
        root.append(document.createElement("br"));
      }
    });
  }
}

function parseComposerSegments(root) {
  if (!root) {
    return [];
  }
  const segments = [];
  collectComposerSegments(root, segments);
  return normalizeComposerSegments(segments);
}

function collectComposerSegments(node, segments) {
  node.childNodes.forEach((child) => {
    if (child.nodeType === Node.TEXT_NODE) {
      segments.push({ type: "text", text: child.textContent ?? "" });
      return;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) {
      return;
    }
    if (child.dataset?.userId) {
      segments.push({
        type: "mention",
        userId: child.dataset.userId,
        userName: child.dataset.userName || child.textContent?.replace(/^@/, "") || child.dataset.userId,
      });
      return;
    }
    if (child.tagName === "BR") {
      segments.push({ type: "text", text: "\n" });
      return;
    }
    collectComposerSegments(child, segments);
    if (child.tagName === "DIV" || child.tagName === "P") {
      segments.push({ type: "text", text: "\n" });
    }
  });
}

function normalizeComposerSegments(segments) {
  const normalized = [];
  for (const segment of segments) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      if (!segment.userId) {
        continue;
      }
      normalized.push(segment);
      continue;
    }
    const text = segment.text ?? "";
    if (!text) {
      continue;
    }
    const previous = normalized[normalized.length - 1];
    if (previous?.type === "text") {
      previous.text += text;
    } else {
      normalized.push({ type: "text", text });
    }
  }
  while (normalized.at(-1)?.type === "text" && normalized.at(-1).text.endsWith("\n")) {
    normalized.at(-1).text = normalized.at(-1).text.replace(/\n+$/, "");
    if (!normalized.at(-1).text) {
      normalized.pop();
    }
  }
  return normalized;
}

function segmentsToPlainText(segments) {
  return (segments ?? []).map((segment) => {
    if (segment.type === "mention") {
      return `@${segment.userName || segment.userId}`;
    }
    return segment.text ?? "";
  }).join("");
}

function areComposerSegmentsEqual(left, right) {
  if (left === right) {
    return true;
  }
  if (!left || !right || left.length !== right.length) {
    return false;
  }
  return left.every((segment, index) => {
    const other = right[index];
    return segment.type === other?.type
      && segment.text === other?.text
      && segment.userId === other?.userId
      && segment.userName === other?.userName;
  });
}

function updateDrafts(current, conversationID, segments) {
  const normalized = normalizeComposerSegments(segments ?? []);
  const existing = current[conversationID] ?? [];
  if (areComposerSegmentsEqual(existing, normalized)) {
    return current;
  }
  if (normalized.length === 0) {
    if (!current[conversationID]) {
      return current;
    }
    const next = { ...current };
    delete next[conversationID];
    return next;
  }
  return { ...current, [conversationID]: normalized };
}

function serializeComposerSegments(segments) {
  return (segments ?? []).map((segment) => {
    if (segment.type === "mention") {
      const userID = segment.userId || "";
      const userName = segment.userName || userID;
      return `<at user_id="${userID}">${userName}</at>`;
    }
    return segment.text ?? "";
  }).join("");
}

function getComposerMentionState(root) {
  if (!root) {
    return null;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return null;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return null;
  }
  const context = getActiveTextQueryContext(range.startContainer, range.startOffset);
  if (!context) {
    return null;
  }
  const match = context.textBeforeCursor.match(/(^|\s)@([a-zA-Z0-9._-]*)$/);
  if (!match) {
    return null;
  }
  return {
    query: match[2],
    textNode: context.textNode,
    startOffset: context.offset - match[2].length - 1,
    endOffset: context.offset,
  };
}

function getActiveTextQueryContext(node, offset) {
  if (node.nodeType === Node.TEXT_NODE) {
    return {
      textNode: node,
      offset,
      textBeforeCursor: (node.textContent ?? "").slice(0, offset),
    };
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const child = node.childNodes[offset - 1];
  if (!child || child.nodeType !== Node.TEXT_NODE) {
    return null;
  }
  return {
    textNode: child,
    offset: child.textContent?.length ?? 0,
    textBeforeCursor: child.textContent ?? "",
  };
}

function replaceMentionQueryWithToken(root, mentionState, user) {
  if (!root || !mentionState?.textNode || !user) {
    return false;
  }
  const range = document.createRange();
  range.setStart(mentionState.textNode, mentionState.startOffset);
  range.setEnd(mentionState.textNode, mentionState.endOffset);
  range.deleteContents();

  const spacer = document.createTextNode(" ");
  const token = createMentionTokenElement(user);
  const fragment = document.createDocumentFragment();
  fragment.append(token, spacer);
  range.insertNode(fragment);

  const selection = window.getSelection();
  const afterRange = document.createRange();
  afterRange.setStart(spacer, spacer.textContent.length);
  afterRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(afterRange);
  root.focus();
  return true;
}

function insertComposerLineBreak(root) {
  if (!root) {
    return;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return;
  }
  range.deleteContents();
  const br = document.createElement("br");
  const spacer = document.createTextNode("");
  range.insertNode(br);
  br.after(spacer);
  const nextRange = document.createRange();
  nextRange.setStart(spacer, 0);
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
}

function insertPlainTextAtSelection(text) {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  range.deleteContents();
  const node = document.createTextNode(text);
  range.insertNode(node);
  const nextRange = document.createRange();
  nextRange.setStart(node, node.textContent.length);
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
}

function removeAdjacentMentionToken(root, direction) {
  if (!root) {
    return false;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || !selection.isCollapsed) {
    return false;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return false;
  }
  const token = findAdjacentMentionToken(range.startContainer, range.startOffset, direction);
  if (!token) {
    return false;
  }
  const sibling = direction === "backward" ? token.nextSibling : token.previousSibling;
  token.remove();
  if (sibling?.nodeType === Node.TEXT_NODE && sibling.textContent === " ") {
    sibling.remove();
  }
  placeCaretNearNode(root, direction === "backward" ? sibling?.previousSibling ?? root : sibling?.nextSibling ?? root, direction);
  return true;
}

function findAdjacentMentionToken(node, offset, direction) {
  if (node.nodeType === Node.TEXT_NODE) {
    if (direction === "backward" && offset > 0) {
      return null;
    }
    if (direction === "forward" && offset < (node.textContent?.length ?? 0)) {
      return null;
    }
    const sibling = direction === "backward" ? node.previousSibling : node.nextSibling;
    return sibling?.dataset?.userId ? sibling : null;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const index = direction === "backward" ? offset - 1 : offset;
  const sibling = node.childNodes[index];
  return sibling?.dataset?.userId ? sibling : null;
}

function placeCaretNearNode(root, node, direction) {
  const selection = window.getSelection();
  const range = document.createRange();
  if (node?.nodeType === Node.TEXT_NODE) {
    const offset = direction === "backward" ? node.textContent.length : 0;
    range.setStart(node, offset);
  } else if (node?.parentNode) {
    const parent = node.parentNode;
    const index = Array.prototype.indexOf.call(parent.childNodes, node);
    range.setStart(parent, direction === "backward" ? index + 1 : index);
  } else {
    range.setStart(root, root.childNodes.length);
  }
  range.collapse(true);
  selection.removeAllRanges();
  selection.addRange(range);
  root.focus();
}

function placeCaretAtEnd(root) {
  placeCaretNearNode(root, root.lastChild, "backward");
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

class AppErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error) {
    return { error };
  }

  componentDidCatch(error) {
    console.error(error);
  }

  render() {
    if (this.state.error) {
      return html`
        <div className="empty-state app-error-state">
          <strong>CSGClaw UI crashed</strong>
          <span>${this.state.error?.message || "Unknown frontend error"}</span>
          <button className="secondary-button" onClick=${() => window.location.reload()}>Reload</button>
        </div>
      `;
    }
    return this.props.children;
  }
}

createRoot(document.getElementById("root")).render(html`<${AppErrorBoundary}><${App} /><//>`);
