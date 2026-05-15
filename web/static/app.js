import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from "https://esm.sh/react@18.3.1";
import { createRoot } from "https://esm.sh/react-dom@18.3.1/client";
import htm from "https://esm.sh/htm@3.1.1";
import { marked } from "https://esm.sh/marked@13.0.2";
import DOMPurify from "https://esm.sh/dompurify@3.1.6";
import mermaid from "https://esm.sh/mermaid@11.4.1";

const html = htm.bind(React.createElement);
const LOCALE_STORAGE_KEY = "csgclaw.im.locale";
const THEME_STORAGE_KEY = "csgclaw.im.theme";
const SIDEBAR_COLLAPSED_STORAGE_KEY = "csgclaw.im.sidebarCollapsed";
const WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY = "csgclaw.im.workspaceGroupsCollapsed";
const MESSAGE_LIST_BOTTOM_THRESHOLD = 24;
const AGENT_STATUS_REFRESH_INTERVAL_MS = 2000;
const IM_EVENTS_ENDPOINT = "/api/v1/events";
const IM_EVENTS_SHARED_WORKER_PATH = "/sse-shared-worker.js";
const VERSION_ENDPOINT = "/api/v1/version";
const UPGRADE_STATUS_ENDPOINT = "/api/v1/upgrade/status";
const UPGRADE_APPLY_ENDPOINT = "/api/v1/upgrade/apply";
const PROVIDERS = ["csghub_lite", "codex", "claude_code", "api"];
const RUNTIME_KIND_OPTIONS = [
  { value: "picoclaw_sandbox", label: "picoclaw_sandbox" },
  { value: "openclaw_sandbox", label: "openclaw_sandbox" },
  { value: "codex", label: "codex" },
];
const GATEWAY_RUNTIME_KIND_OPTIONS = RUNTIME_KIND_OPTIONS.filter((option) => option.value === "picoclaw_sandbox");
const CLIPROXY_AUTH_PROVIDERS = new Set(["codex", "claude_code"]);
const REASONING_EFFORTS = ["low", "medium", "high", "xhigh"];
const WORKSPACE_TAB_MESSAGES = "messages";
const WORKSPACE_TAB_AGENTS = "agents";
const WORKSPACE_TAB_HUB = "hub";
const CSGCLAW_ACTION_CARD_TYPE = "csgclaw.action_card";
const ACTION_REBUILD_MANAGER = "rebuild-manager";

marked.setOptions({
  gfm: true,
  breaks: true,
});

mermaid.initialize({
  startOnLoad: false,
  securityLevel: "strict",
  theme: "neutral",
});

function safeParseEventData(raw) {
  try {
    return JSON.parse(raw);
  } catch (error) {
    console.warn("Failed to parse IM event payload", error);
    return null;
  }
}

// API returns Version from git describe (e.g. "v0.2.1-5-gabc-dirty") or "dev"; avoid "vv" in the UI.
function formatSidebarVersionLabel(version) {
  const raw = typeof version === "string" ? version.trim() : "";
  if (!raw) {
    return "csgclaw dev";
  }
  return raw.startsWith("v") ? `csgclaw ${raw}` : `csgclaw v${raw}`;
}

function subscribeIMEvents(onEvent) {
  if (typeof window.SharedWorker === "function") {
    try {
      const worker = new SharedWorker(IM_EVENTS_SHARED_WORKER_PATH);
      const port = worker.port;
      const handleMessage = ({ data }) => {
        if (!data || data.type !== "message") {
          return;
        }
        const payload = safeParseEventData(data.data);
        if (payload) {
          onEvent(payload);
        }
      };

      port.addEventListener("message", handleMessage);
      port.start();
      port.postMessage({ type: "subscribe", endpoint: IM_EVENTS_ENDPOINT });

      return () => {
        port.postMessage({ type: "close" });
        port.removeEventListener("message", handleMessage);
        port.close();
      };
    } catch (error) {
      console.warn("SharedWorker SSE unavailable, falling back to EventSource", error);
    }
  }

  const source = new EventSource(IM_EVENTS_ENDPOINT);
  source.onmessage = (event) => {
    const payload = safeParseEventData(event.data);
    if (payload) {
      onEvent(payload);
    }
  };

  return () => source.close();
}

const messages = {
  zh: {
    pageTitle: "CSGClaw",
    localAgentConsole: "本地 Agent 控制台",
    loading: "正在加载 IM 工作区...",
    loadingFailed: "加载失败，请稍后重试。",
    emptyConversation: "请选择一个房间或私信",
    conversationSection: "房间",
    computerSection: "电脑",
    computerAgentsSection: "Agents",
    channelsSection: "房间",
    directMessagesSection: "私信",
    messagesTab: "消息",
    agentsTab: "Agents",
    hubTab: "Hub",
    newBadge: "NEW",
    computersSection: "电脑",
    localComputer: "本机",
    computerOverview: "电脑概览",
    agentOverview: "Agent 概览",
    hubOverview: "Hub 概览",
    hubTitle: "Hub",
    hubSubtitle: "发现可复用的 Agent 模板，并作为全局入口浏览模板市场。",
    hubTemplatesSection: "模板",
    hubAllTab: "全部",
    hubTemplateCountSuffix: "个 Agent 模板",
    hubSourceLabel: "来源",
    hubRuntimeLabel: "运行时",
    hubImageLabel: "镜像",
    hubWorkspaceLabel: "工作区",
    hubUpdatedAtLabel: "更新时间",
    hubDescriptionLabel: "描述",
    hubWorkspaceTemplateLabel: "Workspace（模板文件目录）",
    hubWorkspacePreviewTitle: "选择一个文件查看内容",
    hubWorkspacePreviewHint: "在左侧文件树中选择文件以查看其内容",
    hubWorkspaceLoading: "正在加载模板工作区...",
    hubWorkspaceLoadFailed: "模板工作区加载失败，请稍后重试。",
    hubWorkspaceFileLoading: "正在加载文件内容...",
    hubWorkspaceFileLoadFailed: "文件内容加载失败，请稍后重试。",
    hubWorkspaceBinary: "该文件是二进制文件，暂不支持预览。",
    hubWorkspaceEmptyFile: "该文件为空。",
    hubListEnd: "没有更多了",
    hubLoading: "正在加载 Hub 模板...",
    hubRefresh: "刷新模板",
    hubTemplateSourceLabel: "模板来源",
    hubEmpty: "还没有可用模板。",
    hubLoadFailed: "Hub 模板加载失败，请稍后重试。",
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
    profileAPIKeyNewPlaceholder: "sk-...",
    profileHeaders: "Headers JSON",
    profileRequestOptions: "请求选项（JSON，合并到请求顶层）",
    profileEnv: "环境变量",
    profileEnvKey: "键",
    profileEnvValue: "值",
    profileEnvAdd: "添加变量",
    profileEnvRemove: "移除变量",
    profileReasoning: "Reasoning",
    profileFastMode: "Fast mode",
    agentRuntime: "Agent Runtime",
    runtimePicoclaw: "PicoClaw",
    runtimeOpenclaw: "OpenClaw",
    profileBasics: "基础信息",
    profileRuntimeKind: "运行时",
    profileModelSection: "模型",
    profileAPIProvider: "API Provider",
    profileAdvanced: "高级选项",
    templateLabel: "模板",
    templateNone: "空白",
    profilePreview: "Profile 预览",
    openProfile: "打开 Profile",
    openDM: "私信",
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
    agentPublish: "发布",
    agentPublishing: "发布中...",
    agentCreateProgressPreparing: "准备创建",
    agentCreateProgressSandboxConfig: "写入沙箱配置",
    agentCreateProgressImage: "准备镜像",
    agentCreateProgressRuntime: "创建运行时",
    agentCreateProgressStart: "启动 Agent",
    agentCreateProgressFinishing: "同步状态",
    agentCreateProgressDone: "完成",
    agentCreateProgressFailed: "创建失败",
    agentCreated: "Agent 已创建",
    agentUpdated: "Agent 已更新",
    agentActionFailed: "Agent 操作失败",
    managerRebuildConfirm: "重建 Manager 会中断当前 Manager，会话可能需要刷新。确认继续？",
    managerRebuildTitle: "重建 Manager",
    managerRebuildSubtitle: "选择重建时使用的 runtime 和 image。这个操作会中断当前 Manager。",
    managerRebuildAction: "重建",
    profileRestartRequired: "需要重建",
    profileCompleteBadge: "已配置",
    profileIncompleteBadge: "未配置",
    noAgents: "还没有 Worker。",
    noChannels: "还没有房间。",
    noDirectMessages: "还没有私信。",
    modelLoadFailed: "模型加载失败",
    authConnected: "已连接",
    authMissing: "需要登录",
    authConnect: "连接",
    authConnecting: "连接中...",
    authRequired: "请先连接当前 Provider 后再发送消息。",
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
    allMembers: "全部成员",
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
    upgradeAction: "更新并重启",
    upgradeActionBusy: "更新中...",
    upgradeApplyFailed: "启动升级失败，请重试。",
    upgradeTitle: "发现新版本",
    upgradeSubtitle: "可以直接在界面中完成升级，升级过程会短暂重启本地服务。",
    upgradeCurrentVersion: "当前版本",
    upgradeLatestVersion: "最新版本",
    upgradeStatus: "状态",
    upgradeStatusReady: "准备升级",
    upgradeStatusStarting: "正在启动升级",
    upgradeStatusRestarting: "正在升级并等待服务重启",
    upgradeStatusDone: "升级完成",
    upgradeStatusError: "升级失败",
    upgradeConfirmBody: "点击更新后会运行 csgclaw upgrade，并在完成后重启本地服务。",
    upgradeRestartingBody: "升级 helper 已启动。页面会自动等待服务恢复，期间连接短暂中断是正常现象。",
    upgradeDoneBody: "服务已经恢复，刷新页面后即可使用新版本。",
    upgradeNoLatest: "未知",
    upgradeRefresh: "刷新页面",
    upgradeLater: "稍后",
    upgradeConfirm: "立即更新并重启",
    upgradeBackground: "后台升级中",
    upgradeViewProgress: "查看进度",
    upgradeContinueUsing: "升级已在后台运行，你可以继续使用产品。",
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
    pageTitle: "CSGClaw",
    localAgentConsole: "Local agent console",
    loading: "Loading IM workspace...",
    loadingFailed: "Failed to load the workspace. Please try again.",
    emptyConversation: "Select a room or DM",
    conversationSection: "Rooms",
    computerSection: "Computer",
    computerAgentsSection: "Agents",
    channelsSection: "Rooms",
    directMessagesSection: "Direct Messages",
    messagesTab: "Messages",
    agentsTab: "Agents",
    hubTab: "Hub",
    newBadge: "NEW",
    computersSection: "Computers",
    localComputer: "Local computer",
    computerOverview: "Computer overview",
    agentOverview: "Agent overview",
    hubOverview: "Hub overview",
    hubTitle: "Hub",
    hubSubtitle: "Browse reusable agent templates from a global entry point.",
    hubTemplatesSection: "Templates",
    hubAllTab: "All",
    hubTemplateCountSuffix: "Agent templates",
    hubSourceLabel: "Source",
    hubRuntimeLabel: "Runtime",
    hubImageLabel: "Image",
    hubWorkspaceLabel: "Workspace",
    hubUpdatedAtLabel: "Updated",
    hubDescriptionLabel: "Description",
    hubWorkspaceTemplateLabel: "Workspace (template directory)",
    hubWorkspacePreviewTitle: "Select a file to preview",
    hubWorkspacePreviewHint: "Choose a file from the tree on the left to view its content",
    hubWorkspaceLoading: "Loading template workspace...",
    hubWorkspaceLoadFailed: "Failed to load the template workspace. Please try again later.",
    hubWorkspaceFileLoading: "Loading file content...",
    hubWorkspaceFileLoadFailed: "Failed to load the file content. Please try again later.",
    hubWorkspaceBinary: "This file is binary and cannot be previewed here.",
    hubWorkspaceEmptyFile: "This file is empty.",
    hubListEnd: "No more templates",
    hubLoading: "Loading Hub templates...",
    hubRefresh: "Refresh templates",
    hubTemplateSourceLabel: "Template source",
    hubEmpty: "No templates available yet.",
    hubLoadFailed: "Failed to load Hub templates. Please try again later.",
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
    profileAPIKeyNewPlaceholder: "sk-...",
    profileHeaders: "Headers JSON",
    profileRequestOptions: "Request options (JSON, merged into top-level request)",
    profileEnv: "Environment variables",
    profileEnvKey: "Key",
    profileEnvValue: "Value",
    profileEnvAdd: "Add variable",
    profileEnvRemove: "Remove variable",
    profileReasoning: "Reasoning",
    profileFastMode: "Fast mode",
    agentRuntime: "Agent Runtime",
    runtimePicoclaw: "PicoClaw",
    runtimeOpenclaw: "OpenClaw",
    profileBasics: "Basics",
    profileRuntimeKind: "Runtime",
    profileModelSection: "Model",
    profileAPIProvider: "API Provider",
    profileAdvanced: "Advanced",
    templateLabel: "Template",
    templateNone: "Blank",
    profilePreview: "Profile preview",
    openProfile: "Open profile",
    openDM: "DM",
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
    agentPublish: "Publish",
    agentPublishing: "Publishing...",
    agentCreateProgressPreparing: "Preparing",
    agentCreateProgressSandboxConfig: "Writing sandbox config",
    agentCreateProgressImage: "Preparing image",
    agentCreateProgressRuntime: "Creating runtime",
    agentCreateProgressStart: "Starting agent",
    agentCreateProgressFinishing: "Syncing status",
    agentCreateProgressDone: "Done",
    agentCreateProgressFailed: "Create failed",
    agentCreated: "Agent created",
    agentUpdated: "Agent updated",
    agentActionFailed: "Agent action failed",
    managerRebuildConfirm: "Rebuilding Manager interrupts the current Manager and this session may need a refresh. Continue?",
    managerRebuildTitle: "Recreate Manager",
    managerRebuildSubtitle: "Choose the runtime and image to use for recreate. This interrupts the current Manager.",
    managerRebuildAction: "Recreate",
    profileRestartRequired: "Restart needed",
    profileCompleteBadge: "Configured",
    profileIncompleteBadge: "Incomplete",
    noAgents: "No workers yet.",
    noChannels: "No rooms yet.",
    noDirectMessages: "No direct messages yet.",
    modelLoadFailed: "Failed to load models",
    authConnected: "Connected",
    authMissing: "Login required",
    authConnect: "Connect",
    authConnecting: "Connecting...",
    authRequired: "Connect the current provider before sending messages.",
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
    allMembers: "All members",
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
    upgradeAction: "Update & Restart",
    upgradeActionBusy: "Updating...",
    upgradeApplyFailed: "Failed to start the upgrade. Please retry.",
    upgradeTitle: "New version available",
    upgradeSubtitle: "Upgrade directly from the app. The local service will restart briefly.",
    upgradeCurrentVersion: "Current version",
    upgradeLatestVersion: "Latest version",
    upgradeStatus: "Status",
    upgradeStatusReady: "Ready to update",
    upgradeStatusStarting: "Starting upgrade",
    upgradeStatusRestarting: "Upgrading and waiting for service restart",
    upgradeStatusDone: "Upgrade complete",
    upgradeStatusError: "Upgrade failed",
    upgradeConfirmBody: "Updating runs csgclaw upgrade and restarts the local service when it finishes.",
    upgradeRestartingBody: "The upgrade helper has started. This page will wait for the service to come back; a brief disconnect is normal.",
    upgradeDoneBody: "The service is back. Refresh the page to use the new version.",
    upgradeNoLatest: "Unknown",
    upgradeRefresh: "Refresh page",
    upgradeLater: "Later",
    upgradeConfirm: "Update & Restart now",
    upgradeBackground: "Updating in background",
    upgradeViewProgress: "View progress",
    upgradeContinueUsing: "The upgrade is running in the background. You can keep using the product.",
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

function requiredFieldLabel(label) {
  return html`
    <span className="field-label">
      ${label}
      <span className="field-required-star" aria-hidden="true">*</span>
    </span>
  `;
}

function APIKeyField({ value, onInput, profile, t }) {
  const stored = Boolean(profile?.api_key_set);
  const preview = String(profile?.api_key_preview || "").trim();
  const showStoredMask = stored && isBlank(value);
  const previewPrefix = preview.endsWith("...") ? preview.slice(0, -3) : "";
  const placeholder = stored ? "" : t("profileAPIKeyNewPlaceholder");
  return html`
    <label className="field api-key-field">
      <span>${t("profileAPIKey")}</span>
      <div className="api-key-input-shell">
        <input
          value=${value}
          onInput=${onInput}
          placeholder=${placeholder}
          autoComplete="off"
          spellCheck=${false}
        />
        ${showStoredMask
          ? html`
              <div className="api-key-mask" aria-hidden="true">
                ${previewPrefix ? html`<span className="api-key-mask-prefix">${previewPrefix}</span>` : null}
                <span className="api-key-mask-dots">••••••••</span>
              </div>
            `
          : null}
      </div>
    </label>
  `;
}

function isBlank(value) {
  return !String(value ?? "").trim();
}

function profileBaseURLMissing(draft) {
  return draft?.provider === "api" && isBlank(draft.base_url);
}

function IconImage(name) {
  return html`
    <span
      className="svg-icon"
      aria-hidden="true"
      style=${{
        WebkitMaskImage: `url("/icons/${name}.svg")`,
        maskImage: `url("/icons/${name}.svg")`,
      }}
    ></span>
  `;
}

function GlobeIcon() {
  return IconImage("globe");
}

function SunIcon() {
  return IconImage("sun");
}

function MoonIcon() {
  return IconImage("moon");
}

function MessageContent({ content, message, actionBusy, actionError, onAction }) {
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
    if (structured.kind === "action_card") {
      return html`<${ActionCard} data=${structured} message=${message} busyKey=${actionBusy} error=${actionError} onAction=${onAction} />`;
    }
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

function ActionCard({ data, message, busyKey, error, onAction }) {
  const actionError = data.actions?.some((action) => `${message?.id || "message"}:${action.id}` === error?.key)
    ? error?.message
    : "";
  return html`
    <div className="structured-message action-card">
      <div className="structured-message-header">
        <div>
          <div className="structured-message-title">${data.title}</div>
          ${data.subtitle ? html`<div className="structured-message-subtitle">${data.subtitle}</div>` : null}
        </div>
        ${data.badge ? html`<span className="structured-message-badge">${data.badge}</span>` : null}
      </div>
      ${data.summary ? html`<div className="structured-message-summary">${data.summary}</div>` : null}
      ${data.actions?.length
        ? html`
            <div className="structured-message-actions">
              ${data.actions.map((action) => {
                const key = `${message?.id || "message"}:${action.id}`;
                const busy = busyKey === key;
                const danger = action.style === "danger";
                return html`
                  <button
                    key=${action.id}
                    type="button"
                    className=${`btn ${danger ? "btn-outline-danger" : "btn-secondary-gray"} btn-sm structured-message-action-button`}
                    disabled=${busy || !onAction}
                    onClick=${() => onAction?.(action, message)}
                  >
                    ${busy ? "..." : action.label}
                  </button>
                `;
              })}
            </div>
          `
        : null}
      ${actionError ? html`<div className="structured-message-action-error">${actionError}</div>` : null}
      ${data.fallback ? html`<div className="structured-message-subtitle">${data.fallback}</div>` : null}
    </div>
  `;
}

function AddUserIcon() {
  return IconImage("add-user");
}

function UsersIcon() {
  return IconImage("users");
}

function WrenchIcon() {
  return IconImage("wrench");
}

function SidebarToggleIcon() {
  return IconImage("sidebar-toggle");
}

function ChevronIcon() {
  return IconImage("chevron");
}

function RoomPlusIcon() {
  return IconImage("room-plus");
}

function TrashIcon() {
  return IconImage("trash");
}

function RoomsIcon() {
  return IconImage("rooms");
}

function AgentIcon() {
  return IconImage("agent");
}

function ComputerIcon() {
  return IconImage("computer");
}

function HubIcon() {
  return IconImage("hub");
}

function PlayIcon() {
  return IconImage("play");
}

function StopIcon() {
  return IconImage("stop");
}

function WorkspaceFileIcon() {
  return html`
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path d="M8.16634 1.45817C8.16634 2.58384 8.16634 3.14668 8.38433 3.5745C8.57608 3.95083 8.88204 4.25679 9.25836 4.44853C9.68618 4.66652 10.249 4.66652 11.3747 4.66652M11.6663 5.40865V9.63317C11.6663 10.7533 11.6663 11.3133 11.4484 11.7412C11.2566 12.1175 10.9506 12.4234 10.5743 12.6152C10.1465 12.8332 9.58645 12.8332 8.46634 12.8332H5.53301C4.4129 12.8332 3.85285 12.8332 3.42503 12.6152C3.0487 12.4234 2.74274 12.1175 2.55099 11.7412C2.33301 11.3133 2.33301 10.7533 2.33301 9.63317V4.36651C2.33301 3.2464 2.33301 2.68635 2.55099 2.25852C2.74274 1.8822 3.0487 1.57624 3.42503 1.38449C3.85285 1.1665 4.4129 1.1665 5.53301 1.1665H7.42419C7.91337 1.1665 8.15796 1.1665 8.38814 1.22176C8.59221 1.27076 8.7873 1.35157 8.96624 1.46122C9.16808 1.58491 9.34103 1.75786 9.68693 2.10376L10.7291 3.14591C11.075 3.49182 11.2479 3.66477 11.3716 3.8666C11.4813 4.04555 11.5621 4.24063 11.6111 4.44471C11.6663 4.67488 11.6663 4.91947 11.6663 5.40865Z" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"/>
    </svg>
  `;
}

function WorkspaceDirIcon() {
  return html`
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path d="M3.52949 0.729004C2.5494 0.729004 2.05935 0.729004 1.68501 0.919743C1.35573 1.08752 1.08801 1.35524 0.920231 1.68452C0.729492 2.05887 0.729492 2.54891 0.729492 3.52901V9.53734C0.729492 10.8441 0.729492 11.4975 0.98381 11.9966C1.20751 12.4357 1.56447 12.7926 2.00351 13.0164C2.50264 13.2707 3.15604 13.2707 4.46283 13.2707H9.53783C10.8446 13.2707 11.498 13.2707 11.9971 13.0164C12.4362 12.7926 12.7931 12.4357 13.0168 11.9966C13.2712 11.4975 13.2712 10.8441 13.2712 9.53734V6.79567C13.2712 5.48888 13.2712 4.83549 13.0168 4.33636C12.7931 3.89731 12.4362 3.54036 11.9971 3.31666C11.498 3.06234 10.8446 3.06234 9.53783 3.06234H8.89755C8.58581 3.06234 8.42993 3.06234 8.2892 3.02677C8.05664 2.96799 7.84784 2.83894 7.69126 2.65722C7.59651 2.54725 7.5268 2.40784 7.38738 2.129C7.17826 1.71076 7.0737 1.50163 6.93157 1.33668C6.6967 1.06409 6.3835 0.870531 6.03465 0.782358C5.82356 0.729004 5.58975 0.729004 5.12213 0.729004H3.52949Z" fill="currentColor"/>
    </svg>
  `;
}

function HubPreviewEmptyIcon() {
  return html`
    <svg className="hub-preview-empty-icon" width="32" height="32" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path opacity="0.12" d="M5.33337 13.3337V18.667C5.33337 22.4007 5.33337 24.2675 6.06 25.6936C6.69915 26.948 7.71902 27.9679 8.97344 28.607C10.3995 29.3337 12.2664 29.3337 16 29.3337C19.7337 29.3337 21.6006 29.3337 23.0266 28.607C24.2811 27.9679 25.3009 26.948 25.9401 25.6936C26.6667 24.2675 26.6667 22.4007 26.6667 18.667V12.8003C26.6667 12.0536 26.6667 11.6802 26.5214 11.395C26.3936 11.1441 26.1896 10.9401 25.9387 10.8123C25.6535 10.667 25.2801 10.667 24.5334 10.667H22.9334C21.4399 10.667 20.6932 10.667 20.1227 10.3763C19.621 10.1207 19.213 9.71273 18.9574 9.21097C18.6667 8.64054 18.6667 7.8938 18.6667 6.40033V4.80033C18.6667 4.05359 18.6667 3.68022 18.5214 3.395C18.3936 3.14412 18.1896 2.94015 17.9387 2.81232C17.6535 2.66699 17.2801 2.66699 16.5334 2.66699H16C12.2664 2.66699 10.3995 2.66699 8.97344 3.39362C7.71902 4.03277 6.69915 5.05264 6.06 6.30706C5.33337 7.73313 5.33337 9.59997 5.33337 13.3337Z" fill="#4D6AD6"/>
      <path d="M18.6667 3.33366V6.40037C18.6667 7.89384 18.6667 8.64058 18.9574 9.21101C19.213 9.71277 19.621 10.1207 20.1227 10.3764C20.6932 10.667 21.4399 10.667 22.9334 10.667H26M12 16.0003H20M12 21.3337H17.3334M26.6667 11.9846V22.9337C26.6667 25.1739 26.6667 26.294 26.2307 27.1496C25.8472 27.9023 25.2353 28.5142 24.4827 28.8977C23.627 29.3337 22.5069 29.3337 20.2667 29.3337H11.7334C9.49317 29.3337 8.37306 29.3337 7.51741 28.8977C6.76476 28.5142 6.15284 27.9023 5.76935 27.1496C5.33337 26.294 5.33337 25.1739 5.33337 22.9337V9.06699C5.33337 6.82678 5.33337 5.70668 5.76935 4.85103C6.15284 4.09838 6.76476 3.48646 7.51741 3.10297C8.37306 2.66699 9.49316 2.66699 11.7334 2.66699H17.3491C18.3274 2.66699 18.8166 2.66699 19.277 2.77751C19.6851 2.8755 20.0753 3.03712 20.4332 3.25643C20.8368 3.5038 21.1828 3.8497 21.8746 4.54151L24.7922 7.45914C25.484 8.15095 25.8299 8.49685 26.0773 8.90052C26.2966 9.25841 26.4582 9.64859 26.5562 10.0567C26.6667 10.5171 26.6667 11.0063 26.6667 11.9846Z" stroke="#4D6AD6" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
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
    case "hub":
      return { type: "hub", id: "hub" };
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
  if (pane.type === "hub") {
    return "/hub";
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

function workspaceTabForPane(pane) {
  if (pane?.type === "hub") {
    return WORKSPACE_TAB_HUB;
  }
  if (pane?.type === "agent" || pane?.type === "computer") {
    return WORKSPACE_TAB_AGENTS;
  }
  return WORKSPACE_TAB_MESSAGES;
}

function readCollapsedWorkspaceGroups() {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY) || "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return {};
    }
    return parsed;
  } catch (_) {
    return {};
  }
}

function normalizeUpgradeStatus(status) {
  if (!status || typeof status !== "object") {
    return null;
  }
  return {
    current_version: typeof status.current_version === "string" ? status.current_version : "",
    latest_version: typeof status.latest_version === "string" ? status.latest_version : "",
    update_available: Boolean(status.update_available),
    checking: Boolean(status.checking),
    upgrading: Boolean(status.upgrading),
    last_checked_at: status.last_checked_at || "",
    last_error: typeof status.last_error === "string" ? status.last_error : "",
  };
}

function upgradeStatusLabel(phase, t) {
  switch (phase) {
    case "starting":
      return t("upgradeStatusStarting");
    case "restarting":
      return t("upgradeStatusRestarting");
    case "done":
      return t("upgradeStatusDone");
    case "error":
      return t("upgradeStatusError");
    default:
      return t("upgradeStatusReady");
  }
}

async function readErrorMessage(resp) {
  if (!resp) {
    return "";
  }
  try {
    const text = await resp.text();
    return text.trim();
  } catch (_) {
    return "";
  }
}

function isManagerAgent(item) {
  return item?.role === "manager" || item?.id === "u-manager";
}

function App() {
  const initialPane = useMemo(() => paneFromLocation(), []);
  const [locale, setLocale] = useState(() => detectInitialLocale());
  const [theme, setTheme] = useState(() => detectInitialTheme());
  const [showToolCalls, setShowToolCalls] = useState(true);
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(() => {
    const value = window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY);
    return value === "true";
  });
  const [workspaceTab, setWorkspaceTab] = useState(() => workspaceTabForPane(initialPane));
  const [collapsedWorkspaceGroups, setCollapsedWorkspaceGroups] = useState(() => readCollapsedWorkspaceGroups());
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
  const [bootstrapConfig, setBootstrapConfig] = useState(null);
  const [managerProfile, setManagerProfile] = useState(null);
  const [profileDraft, setProfileDraft] = useState(null);
  const [profileModels, setProfileModels] = useState([]);
  const [profileError, setProfileError] = useState("");
  const [profileBusy, setProfileBusy] = useState(false);
  const [profileModelBusy, setProfileModelBusy] = useState(false);
  const [cliproxyAuthStatuses, setCLIProxyAuthStatuses] = useState({});
  const [cliproxyAuthBusy, setCLIProxyAuthBusy] = useState("");
  const [agents, setAgents] = useState([]);
  const [agentsLoaded, setAgentsLoaded] = useState(false);
  const [agentsError, setAgentsError] = useState("");
  const [hubTemplates, setHubTemplates] = useState([]);
  const [hubLoaded, setHubLoaded] = useState(false);
  const [hubError, setHubError] = useState("");
  const [selectedHubTemplateId, setSelectedHubTemplateId] = useState("");
  const [hubTemplateDetail, setHubTemplateDetail] = useState(null);
  const [hubTemplateDetailLoading, setHubTemplateDetailLoading] = useState(false);
  const [hubTemplateDetailError, setHubTemplateDetailError] = useState("");
  const [hubWorkspaceFile, setHubWorkspaceFile] = useState(null);
  const [hubWorkspaceFileLoading, setHubWorkspaceFileLoading] = useState(false);
  const [hubWorkspaceFileError, setHubWorkspaceFileError] = useState("");
  const [selectedHubWorkspacePath, setSelectedHubWorkspacePath] = useState("");
  const [showAgentModal, setShowAgentModal] = useState(false);
  const [agentModalMode, setAgentModalMode] = useState("create");
  const [editingAgent, setEditingAgent] = useState(null);
  const [agentDraft, setAgentDraft] = useState(null);
  const [agentModels, setAgentModels] = useState([]);
  const [agentBusy, setAgentBusy] = useState(false);
  const [agentModelBusy, setAgentModelBusy] = useState(false);
  const [agentError, setAgentError] = useState("");
  const [agentProgress, setAgentProgress] = useState(null);
  const [agentActionBusy, setAgentActionBusy] = useState("");
  const [messageActionBusy, setMessageActionBusy] = useState("");
  const [messageActionError, setMessageActionError] = useState({ key: "", message: "" });
  const [agentPageDraft, setAgentPageDraft] = useState(null);
  const [agentPageModels, setAgentPageModels] = useState([]);
  const [agentPageBusy, setAgentPageBusy] = useState(false);
  const [agentPagePublishBusy, setAgentPagePublishBusy] = useState(false);
  const [agentPageModelBusy, setAgentPageModelBusy] = useState(false);
  const [agentPageError, setAgentPageError] = useState("");
  const [profilePreview, setProfilePreview] = useState(null);
  const [appVersion, setAppVersion] = useState("dev");
  const [upgradeStatus, setUpgradeStatus] = useState(null);
  const [upgradeBusy, setUpgradeBusy] = useState(false);
  const [upgradeError, setUpgradeError] = useState("");
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradePhase, setUpgradePhase] = useState("idle");
  const [showManagerRebuildModal, setShowManagerRebuildModal] = useState(false);
  const [managerRebuildRuntimeKind, setManagerRebuildRuntimeKind] = useState("picoclaw_sandbox");
  const [managerRebuildImage, setManagerRebuildImage] = useState("");
  const editorRef = useRef(null);
  const messageListRef = useRef(null);
  const memberMenuRef = useRef(null);
  const channelToolsRef = useRef(null);
  const profilePreviewRef = useRef(null);
  const agentRefreshTimerRef = useRef(null);
  const profileDraftRef = useRef(null);
  const agentDraftRef = useRef(null);
  const agentPageDraftRef = useRef(null);
  const upgradePollTimerRef = useRef(null);
  const shouldAutoScrollRef = useRef(true);
  const autoScrollConversationRef = useRef(activeConversationId);

  useEffect(() => {
    refreshBootstrap();
  }, []);

  useEffect(() => {
    refreshVersion();
  }, []);

  useEffect(() => {
    refreshUpgradeStatus();
  }, []);

  useEffect(() => {
    return () => {
      if (upgradePollTimerRef.current) {
        window.clearInterval(upgradePollTimerRef.current);
        upgradePollTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    refreshManagerProfile();
    refreshAgents();
    refreshBootstrapConfig();
    refreshHubTemplates();
  }, []);

  useEffect(() => {
    if (!bootstrapConfig?.runtime_kind) {
      return;
    }
    setProfileDraft((current) => current && !current.runtime_kind
      ? { ...current, runtime_kind: normalizeRuntimeKind(bootstrapConfig.runtime_kind) }
      : current);
  }, [bootstrapConfig?.runtime_kind]);

  useEffect(() => {
    if (!agentBusy || !agentProgress?.steps?.length) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      setAgentProgress((current) => advanceAgentProgress(current));
    }, 1200);
    return () => window.clearInterval(timer);
  }, [agentBusy, agentProgress?.startedAt]);

  useEffect(() => {
    // Temporarily disable background agent polling for debugging multi-tab pending requests.
    // function refreshVisibleAgents() {
    //   if (document.visibilityState === "visible") {
    //     refreshAgents({ silent: true });
    //   }
    // }
    //
    // const intervalID = window.setInterval(refreshVisibleAgents, AGENT_STATUS_REFRESH_INTERVAL_MS);
    // document.addEventListener("visibilitychange", refreshVisibleAgents);
    return () => {
      // window.clearInterval(intervalID);
      // document.removeEventListener("visibilitychange", refreshVisibleAgents);
    };
  }, []);

  useEffect(() => {
    const unsubscribe = subscribeIMEvents((payload) => {
      setData((current) => applyIMEvent(current, payload));
      if (payload?.type === "upgrade.status_changed" && payload.upgrade) {
        const next = normalizeUpgradeStatus(payload.upgrade);
        setUpgradeStatus(next);
        if (next?.upgrading) {
          setUpgradeBusy(true);
          setUpgradePhase((phase) => phase === "done" ? phase : "restarting");
        } else if (!next?.update_available) {
          setUpgradeBusy(false);
        }
      }
      // Temporarily disable event-driven agent polling for debugging multi-tab pending requests.
      // if (isAgentRosterEvent(payload)) {
      //   scheduleAgentsRefresh();
      // }
    });

    return () => {
      unsubscribe();
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
    window.localStorage.setItem(SIDEBAR_COLLAPSED_STORAGE_KEY, String(isSidebarCollapsed));
  }, [isSidebarCollapsed]);

  useEffect(() => {
    window.localStorage.setItem(WORKSPACE_GROUPS_COLLAPSED_STORAGE_KEY, JSON.stringify(collapsedWorkspaceGroups));
  }, [collapsedWorkspaceGroups]);

  useEffect(() => {
    profileDraftRef.current = profileDraft;
  }, [profileDraft]);

  useEffect(() => {
    agentDraftRef.current = agentDraft;
  }, [agentDraft]);

  useEffect(() => {
    agentPageDraftRef.current = agentPageDraft;
  }, [agentPageDraft]);

  useEffect(() => {
    setWorkspaceTab(workspaceTabForPane(activePane));
  }, [activePane?.type]);

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
  const mentionableUsersByHandle = useMemo(() => {
    const result = new Map();
    if (!data) {
      return result;
    }
    const allowed = new Set(activeConversation?.members ?? []);
    data.users
      .filter((user) => allowed.has(user.id))
      .forEach((user) => {
        const handle = String(user.handle ?? "").trim().toLowerCase();
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
    if (!profilePreview) {
      return undefined;
    }

    function handlePointerDown(event) {
      const preview = profilePreviewRef.current;
      const anchor = profilePreview?.anchorEl;
      if (!preview || preview.contains(event.target) || anchor?.contains?.(event.target)) {
        return;
      }
      closeProfilePreview();
    }

    function handleViewportChange() {
      closeProfilePreview();
    }

    document.addEventListener("mousedown", handlePointerDown);
    window.addEventListener("resize", handleViewportChange);
    window.addEventListener("scroll", handleViewportChange, true);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      window.removeEventListener("resize", handleViewportChange);
      window.removeEventListener("scroll", handleViewportChange, true);
    };
  }, [profilePreview]);

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
      setAgentPagePublishBusy(false);
      return;
    }
    loadAgentPageDraft(selectedAgentForPage);
  }, [selectedAgentForPage?.id]);

  useEffect(() => {
    if (activePane.type === "hub" && !hubLoaded && !hubError) {
      refreshHubTemplates();
    }
  }, [activePane.type, hubLoaded, hubError]);

  useEffect(() => {
    if (!hubTemplates.length) {
      setSelectedHubTemplateId("");
      setHubTemplateDetail(null);
      setHubTemplateDetailError("");
      setSelectedHubWorkspacePath("");
      setHubWorkspaceFile(null);
      setHubWorkspaceFileError("");
      return;
    }
    setSelectedHubTemplateId((current) => hubTemplates.some((item) => item.id === current) ? current : hubTemplates[0].id);
  }, [hubTemplates]);

  useEffect(() => {
    if (!selectedHubTemplateId) {
      setHubTemplateDetail(null);
      setHubTemplateDetailLoading(false);
      setHubTemplateDetailError("");
      setSelectedHubWorkspacePath("");
      setHubWorkspaceFile(null);
      setHubWorkspaceFileLoading(false);
      setHubWorkspaceFileError("");
      return;
    }
    loadHubTemplateDetail(selectedHubTemplateId);
  }, [selectedHubTemplateId]);

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
    refreshCLIProxyAuthStatus(managerProfile?.provider);
  }, [managerProfile?.provider]);

  useEffect(() => {
    refreshCLIProxyAuthStatus(profileDraft?.provider);
  }, [profileDraft?.provider]);

  useEffect(() => {
    refreshCLIProxyAuthStatus(agentDraft?.provider);
  }, [agentDraft?.provider]);

  useEffect(() => {
    refreshCLIProxyAuthStatus(agentPageDraft?.provider);
  }, [agentPageDraft?.provider]);

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
    if (activePane.type !== "conversation") {
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

  async function refreshVersion() {
    try {
      const resp = await fetch(VERSION_ENDPOINT);
      if (!resp.ok) {
        throw new Error("version failed");
      }
      const payload = await resp.json();
      if (payload && typeof payload.version === "string" && payload.version.trim()) {
        setAppVersion(payload.version.trim());
      }
    } catch (_) {
      setAppVersion("dev");
    }
  }

  async function refreshUpgradeStatus() {
    try {
      const resp = await fetch(UPGRADE_STATUS_ENDPOINT);
      if (!resp.ok) {
        throw new Error("upgrade status failed");
      }
      const payload = normalizeUpgradeStatus(await resp.json());
      setUpgradeStatus(payload);
      if (payload?.upgrading) {
        setUpgradeBusy(true);
        setUpgradePhase((phase) => phase === "done" ? phase : "restarting");
      } else if (!payload?.update_available) {
        setUpgradeBusy(false);
      }
      return payload;
    } catch (_) {
      setUpgradeStatus(null);
      setUpgradeBusy(false);
      return null;
    }
  }

  function stopUpgradePoll() {
    if (upgradePollTimerRef.current) {
      window.clearInterval(upgradePollTimerRef.current);
      upgradePollTimerRef.current = null;
    }
  }

  function startUpgradeReconnectPoll(expectedVersion) {
    stopUpgradePoll();
    let attempts = 0;
    const poll = async () => {
      attempts += 1;
      try {
        const resp = await fetch(`${VERSION_ENDPOINT}?_=${Date.now()}`, { cache: "no-store" });
        if (!resp.ok) {
          throw new Error("version unavailable");
        }
        const payload = await resp.json();
        const version = typeof payload?.version === "string" ? payload.version.trim() : "";
        const expected = (expectedVersion || "").trim();
        if (version && (!expected || version === expected)) {
          stopUpgradePoll();
          setAppVersion(version);
          setUpgradeBusy(false);
          setUpgradePhase("done");
          setUpgradeStatus((current) => ({
            ...(current || {}),
            current_version: version,
            latest_version: version,
            update_available: false,
            checking: false,
            upgrading: false,
            last_error: "",
          }));
          return;
        }
      } catch (_) {
        // The daemon is expected to be unavailable while the upgrade helper restarts it.
      }
      if (attempts >= 60) {
        stopUpgradePoll();
        setUpgradeBusy(false);
        setUpgradePhase("error");
        const latest = await refreshUpgradeStatus();
        const detail = latest?.last_error ? ` ${latest.last_error}` : "";
        setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
      }
    };
    poll();
    upgradePollTimerRef.current = window.setInterval(poll, 2000);
  }

  async function applyUpgrade() {
    if (upgradeBusy || upgradeStatus?.upgrading) {
      return;
    }

    setUpgradeBusy(true);
    setUpgradeError("");
    setUpgradePhase("starting");
    setShowUpgradeModal(true);
    try {
      const resp = await fetch(UPGRADE_APPLY_ENDPOINT, { method: "POST" });
      if (!resp.ok) {
        const detail = await readErrorMessage(resp);
        throw new Error(detail || "upgrade apply failed");
      }
      setUpgradePhase("restarting");
      setUpgradeStatus((current) => ({
        ...(current || {}),
        upgrading: true,
        last_error: "",
      }));
      startUpgradeReconnectPoll(upgradeStatus?.latest_version);
      setShowUpgradeModal(false);
    } catch (err) {
      setUpgradeBusy(false);
      setUpgradePhase("error");
      const detail = err?.message && err.message !== "upgrade apply failed" ? ` ${err.message}` : "";
      setUpgradeError(`${t("upgradeApplyFailed")}${detail}`);
    }
  }

  async function refreshBootstrapConfig() {
    try {
      const resp = await fetch("api/v1/config/bootstrap");
      if (!resp.ok) {
        return null;
      }
      const payload = await resp.json();
      const normalized = {
        ...payload,
        runtime_kind: normalizeRuntimeKind(payload.runtime_kind),
        supported_runtime_kinds: Array.isArray(payload.supported_runtime_kinds)
          ? payload.supported_runtime_kinds.map((item) => normalizeRuntimeKind(item)).filter((item, index, array) => item && array.indexOf(item) === index)
          : [],
        runtime_default_images: normalizeRuntimeImageMap(payload.runtime_default_images),
      };
      setBootstrapConfig(normalized);
      return normalized;
    } catch (_) {
      return null;
    }
  }

  async function sendMessage() {
    if (managerProfileIncomplete) {
      setComposerError(t("profileIncomplete"));
      return;
    }
    const managerProvider = normalizeAuthProviderName(managerProfile?.provider);
    if (providerNeedsAuth(managerProvider) && cliproxyAuthStatuses[managerProvider]?.authenticated === false) {
      setComposerError(t("authRequired"));
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
    setWorkspaceTab(WORKSPACE_TAB_MESSAGES);
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
    setWorkspaceTab(WORKSPACE_TAB_AGENTS);
    setShowMemberList(false);
    setShowChannelTools(false);
    if (options.updateURL !== false) {
      syncBrowserPath(next, rooms, options.replace ? "replace" : "push");
    }
  }

  function selectComputer(options = {}) {
    const next = { type: "computer", id: "local" };
    setActivePane(next);
    setWorkspaceTab(WORKSPACE_TAB_AGENTS);
    setShowMemberList(false);
    setShowChannelTools(false);
    if (options.updateURL !== false) {
      syncBrowserPath(next, rooms, options.replace ? "replace" : "push");
    }
  }

  function selectHub(options = {}) {
    const next = { type: "hub", id: "hub" };
    setActivePane(next);
    setWorkspaceTab(WORKSPACE_TAB_HUB);
    setShowMemberList(false);
    setShowChannelTools(false);
    if (options.updateURL !== false) {
      syncBrowserPath(next, rooms, options.replace ? "replace" : "push");
    }
  }

  function selectHubTemplate(item) {
    if (!item?.id) {
      selectHub();
      return;
    }
    setSelectedHubTemplateId(item.id);
    selectHub();
  }

  function toggleWorkspaceGroup(id) {
    setCollapsedWorkspaceGroups((current) => ({
      ...current,
      [id]: !current[id],
    }));
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
    setInviteUserIDs([]);
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

  const selectedHubTemplate = useMemo(
    () => hubTemplates.find((item) => item.id === selectedHubTemplateId) || hubTemplates[0] || null,
    [hubTemplates, selectedHubTemplateId],
  );
  const selectedHubTemplateView = hubTemplateDetail?.id === selectedHubTemplateId ? hubTemplateDetail : selectedHubTemplate;

  if (!data) {
    return html`<div className="empty-state">${loadingError || t("loading")}</div>`;
  }

  const createRoomCandidates = data.users;
  const createRoomCandidateIDs = createRoomCandidates.map((user) => user.id).filter(Boolean);
  const createRoomSelectableMemberIDs = createRoomCandidateIDs.filter((id) => !lockedRoomMemberIDs.includes(id));
  const allCreateRoomMembersSelected = createRoomCandidateIDs.length > 0 && createRoomCandidateIDs.every((id) => roomMemberIDs.includes(id));
  const createRoomSelectedMemberCount = createRoomCandidateIDs.filter((id) => roomMemberIDs.includes(id)).length;
  const inviteCandidates = activeConversation
    ? data.users.filter((user) => !activeConversation.members.includes(user.id))
    : [];
  const inviteCandidateIDs = inviteCandidates.map((user) => user.id).filter(Boolean);
  const allInviteCandidatesSelected = inviteCandidateIDs.length > 0 && inviteCandidateIDs.every((id) => inviteUserIDs.includes(id));
  const inviteSelectedMemberCount = inviteCandidateIDs.filter((id) => inviteUserIDs.includes(id)).length;
  const activeConversationMembers = activeConversation
    ? activeConversation.members.map((id) => usersById.get(id)).filter(Boolean)
    : [];
  const inviteActionLabel = activeConversation && isDirectConversation(activeConversation)
    ? t("createRoomFromDM")
    : t("inviteMembers");

  const managerAgent = agents.find((item) => item.role === "manager" || item.id === "u-manager");
  const workerAgents = agents.filter((item) => item.id !== managerAgent?.id);
  const agentItems = [managerAgent, ...workerAgents].filter(Boolean);
  const runningAgentCount = agentItems.filter(isAgentRunning).length;
  const selectedAgent = selectedAgentForPage;
  const selectedConversation = activePane.type === "conversation" ? activeConversation : null;
  const activeChannel = selectedConversation && !isDirectConversation(selectedConversation) ? selectedConversation : null;
  const selectedMessageCount = selectedConversation?.messages?.length ?? 0;
  const currentWorkspaceLabel = activePane.type === "agent"
    ? t("agentOverview")
    : activePane.type === "computer"
      ? t("computerOverview")
      : activePane.type === "hub"
        ? t("hubOverview")
      : t("conversationOverview");
  const previewUser = profilePreview?.type === "user"
    ? usersById.get(profilePreview.id) ?? null
    : profilePreview?.type === "agent"
      ? usersById.get(profilePreview.id) ?? null
      : null;
  const previewAgent = profilePreview
    ? agentItems.find((item) => item.id === profilePreview.id || agentMatchesUser(item, previewUser)) ?? null
    : null;
  const managerRuntimeOptions = availableManagerRuntimeOptions(bootstrapConfig);

  async function refreshManagerProfile() {
    try {
      const resp = await fetch("api/v1/agents/u-manager/profile");
      if (!resp.ok) {
        return;
      }
      const profile = await resp.json();
      setManagerProfile(profile);
      setProfileDraft({
        ...profileToDraft(profile),
        runtime_kind: normalizeRuntimeKind(bootstrapConfig?.runtime_kind || profile.runtime_kind),
      });
    } catch (_) {
      // The manager may not exist during the first bootstrap milliseconds.
    }
  }

  async function refreshCLIProxyAuthStatus(provider) {
    const normalized = normalizeAuthProviderName(provider);
    if (!providerNeedsAuth(normalized)) {
      return;
    }
    try {
      const resp = await fetch(`api/v1/cliproxy/auth/status?provider=${encodeURIComponent(normalized)}`);
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || t("authMissing"));
      }
      const status = await resp.json();
      setCLIProxyAuthStatuses((current) => ({ ...current, [normalized]: status }));
      setComposerError("");
    } catch (err) {
      setCLIProxyAuthStatuses((current) => ({
        ...current,
        [normalized]: {
          provider: normalized,
          authenticated: false,
          login_required: true,
          message: err.message || t("authMissing"),
        },
      }));
    }
  }

  async function refreshHubTemplates() {
    try {
      const resp = await fetch("/api/v1/hub/templates");
      if (!resp.ok) {
        throw new Error("hub templates failed");
      }
      const payload = await resp.json();
      setHubTemplates(Array.isArray(payload) ? payload : []);
      setHubError("");
      setHubLoaded(true);
    } catch (_) {
      setHubTemplates([]);
      setHubError(t("hubLoadFailed"));
      setHubLoaded(true);
    }
  }

  async function loadHubTemplateDetail(templateID) {
    if (!templateID) {
      return;
    }
    setHubTemplateDetailLoading(true);
    setHubTemplateDetailError("");
    setSelectedHubWorkspacePath("");
    setHubWorkspaceFile(null);
    setHubWorkspaceFileError("");
    try {
      const resp = await fetch(`/api/v1/hub/templates/${encodeURIComponent(templateID)}`);
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || t("hubWorkspaceLoadFailed"));
      }
      const payload = await resp.json();
      setHubTemplateDetail(payload);
    } catch (err) {
      setHubTemplateDetail(null);
      setHubTemplateDetailError(err.message || t("hubWorkspaceLoadFailed"));
    } finally {
      setHubTemplateDetailLoading(false);
    }
  }

  async function loadHubWorkspaceFile(templateID, workspacePath) {
    if (!templateID || !workspacePath) {
      return;
    }
    setHubWorkspaceFileLoading(true);
    setHubWorkspaceFileError("");
    try {
      const resp = await fetch(`/api/v1/hub/templates/${encodeURIComponent(templateID)}/workspace/file?path=${encodeURIComponent(workspacePath)}`);
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || t("hubWorkspaceFileLoadFailed"));
      }
      setHubWorkspaceFile(await resp.json());
    } catch (err) {
      setHubWorkspaceFile(null);
      setHubWorkspaceFileError(err.message || t("hubWorkspaceFileLoadFailed"));
    } finally {
      setHubWorkspaceFileLoading(false);
    }
  }

  async function loginCLIProxyProvider(provider) {
    const normalized = normalizeAuthProviderName(provider);
    if (!providerNeedsAuth(normalized) || cliproxyAuthBusy) {
      return;
    }
    setCLIProxyAuthBusy(normalized);
    setCLIProxyAuthStatuses((current) => ({
      ...current,
      [normalized]: {
        ...(current[normalized] || {}),
        provider: normalized,
        message: t("authConnecting"),
      },
    }));
    try {
      const resp = await fetch("api/v1/cliproxy/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ provider: normalized }),
      });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || t("authMissing"));
      }
      const status = await resp.json();
      setCLIProxyAuthStatuses((current) => ({ ...current, [normalized]: status }));
    } catch (err) {
      setCLIProxyAuthStatuses((current) => ({
        ...current,
        [normalized]: {
          provider: normalized,
          authenticated: false,
          login_required: true,
          message: err.message || t("authMissing"),
        },
      }));
    } finally {
      setCLIProxyAuthBusy("");
    }
  }

  async function loadProfileModels(draft = profileDraft, options = {}) {
    if (!draft?.provider) {
      return;
    }
    const requestKey = modelRequestKey(draft);
    if (!options.silent) {
      setProfileError("");
    }
    setProfileModelBusy(true);
    try {
      const resp = await fetch("api/v1/agent-profiles/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agent_id: draft.agent_id,
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
      if (modelRequestKey(profileDraftRef.current) !== requestKey) {
        return;
      }
      setProfileModels(payload.models ?? []);
      if (!profileDraftRef.current?.model_id && payload.models?.length > 0) {
        setProfileDraft((current) => {
          if (modelRequestKey(current) !== requestKey || current.model_id) {
            return current;
          }
          return { ...current, model_id: payload.models[0] };
        });
      }
    } catch (err) {
      if (!options.silent) {
        setProfileError(err.message || t("modelLoadFailed"));
      }
      if (modelRequestKey(profileDraftRef.current) === requestKey) {
        setProfileModels([]);
      }
    } finally {
      setProfileModelBusy(false);
    }
  }

  function openManagerRebuildModal(item = managerAgent) {
    const initialRuntimeKind = normalizeRuntimeKind(item?.runtime_kind || bootstrapConfig?.runtime_kind || managerRebuildRuntimeKind);
    const fallbackRuntimeKind = managerRuntimeOptions[0]?.value || "picoclaw_sandbox";
    const resolvedRuntimeKind = managerRuntimeOptions.some((option) => option.value === initialRuntimeKind)
      ? initialRuntimeKind
      : fallbackRuntimeKind;
    const resolvedImage = runtimeImageForKind(
      resolvedRuntimeKind,
      bootstrapConfig,
      item?.image || managerAgent?.image || "",
    );
    setManagerRebuildRuntimeKind(resolvedRuntimeKind);
    setManagerRebuildImage(resolvedImage);
    setShowManagerRebuildModal(true);
  }

  async function requestManagerRebuild(options = {}) {
    const runtimeKind = normalizeRuntimeKind(options.runtimeKind || managerAgent?.runtime_kind || bootstrapConfig?.runtime_kind || managerRuntimeOptions[0]?.value);
    const image = String(options.image ?? managerAgent?.image ?? "").trim();
    const resp = await fetch("api/v1/agents", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: "u-manager",
        replace: true,
        image,
        runtime_kind: runtimeKind,
      }),
    });
    if (!resp.ok) {
      throw new Error((await readErrorMessage(resp)) || t("agentActionFailed"));
    }
    await refreshAgents();
    await refreshManagerProfile();
    await refreshBootstrapConfig();
  }

  async function rebuildManagerFromBrowser(options = {}) {
    setAgentActionBusy("u-manager:recreate");
    setAgentsError("");
    try {
      await requestManagerRebuild(options);
      return true;
    } catch (err) {
      setAgentsError(err.message || t("agentActionFailed"));
      return false;
    } finally {
      setAgentActionBusy("");
    }
  }

  async function confirmManagerRebuild() {
    if (agentActionBusy) {
      return;
    }
    const selectedRuntimeKind = normalizeRuntimeKind(managerRebuildRuntimeKind || managerAgent?.runtime_kind || bootstrapConfig?.runtime_kind);
    const selectedImage = String(managerRebuildImage ?? "").trim();
    setMessageActionError({ key: "", message: "" });
    const rebuilt = await rebuildManagerFromBrowser({ runtimeKind: selectedRuntimeKind, image: selectedImage });
    if (rebuilt) {
      setShowManagerRebuildModal(false);
    }
  }

  async function handleMessageAction(action, message) {
    if (!action || action.id !== ACTION_REBUILD_MANAGER) {
      return;
    }
    if (messageActionBusy || agentActionBusy) {
      return;
    }
    setMessageActionError({ key: "", message: "" });
    openManagerRebuildModal(managerAgent);
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
      setProfileDraft({ ...profileToDraft(saved), agent_id: "u-manager" });
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

  async function openCreateAgentModal(template = undefined) {
    setAgentModalMode("create");
    setEditingAgent(null);
    setAgentError("");
    setAgentProgress(null);
    setAgentModels([]);
    const preferredRuntimeKind = normalizeRuntimeKind(bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "");
    const selectedTemplate = template === undefined
      ? pickDefaultAgentTemplate(hubTemplates, preferredRuntimeKind, bootstrapConfig)
      : normalizeTemplateSelection(template);
    try {
      const resp = await fetch("api/v1/agent-profile-defaults");
      const defaults = resp.ok ? await resp.json() : managerProfile;
      const runtimeKind = normalizeRuntimeKind(selectedTemplate?.runtime_kind || bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "");
      let draft = agentToDraft({
        image: runtimeImageForKind(runtimeKind, bootstrapConfig, managerAgent?.image || ""),
        runtime_kind: runtimeKind,
        agent_profile: defaults,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
      setAgentDraft(draft);
      setShowAgentModal(true);
      loadAgentModels(draft, { silent: true });
    } catch (_) {
      const runtimeKind = normalizeRuntimeKind(selectedTemplate?.runtime_kind || bootstrapConfig?.runtime_kind || managerAgent?.runtime_kind || "");
      let draft = agentToDraft({
        image: runtimeImageForKind(runtimeKind, bootstrapConfig, managerAgent?.image || ""),
        runtime_kind: runtimeKind,
        agent_profile: managerProfile,
      });
      draft = applyTemplateToDraft(draft, selectedTemplate, bootstrapConfig, managerAgent?.image || "");
      setAgentDraft(draft);
      setShowAgentModal(true);
      loadAgentModels(draft, { silent: true });
    }
  }

  async function openEditAgentModal(item) {
    setAgentModalMode("edit");
    setEditingAgent(item);
    setAgentError("");
    setAgentProgress(null);
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
    const requestKey = modelRequestKey(draft);
    if (!options.silent) {
      setAgentPageError("");
    }
    setAgentPageModelBusy(true);
    try {
      const resp = await fetch("api/v1/agent-profiles/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agent_id: draft.agent_id,
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
      if (modelRequestKey(agentPageDraftRef.current) !== requestKey) {
        return;
      }
      setAgentPageModels(payload.models ?? []);
      if (!agentPageDraftRef.current?.model_id && payload.models?.length > 0) {
        setAgentPageDraft((current) => {
          if (modelRequestKey(current) !== requestKey || current.model_id) {
            return current;
          }
          return { ...current, model_id: payload.models[0] };
        });
      }
    } catch (err) {
      if (!options.silent) {
        setAgentPageError(err.message || t("modelLoadFailed"));
      }
      if (modelRequestKey(agentPageDraftRef.current) === requestKey) {
        setAgentPageModels([]);
      }
    } finally {
      setAgentPageModelBusy(false);
    }
  }

  async function loadAgentModels(draft = agentDraft, options = {}) {
    if (!draft?.provider) {
      return;
    }
    const requestKey = modelRequestKey(draft);
    if (!options.silent) {
      setAgentError("");
    }
    setAgentModelBusy(true);
    try {
      const resp = await fetch("api/v1/agent-profiles/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agent_id: draft.agent_id,
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
      if (modelRequestKey(agentDraftRef.current) !== requestKey) {
        return;
      }
      setAgentModels(payload.models ?? []);
      if (!agentDraftRef.current?.model_id && payload.models?.length > 0) {
        setAgentDraft((current) => {
          if (modelRequestKey(current) !== requestKey || current.model_id) {
            return current;
          }
          return { ...current, model_id: payload.models[0] };
        });
      }
    } catch (err) {
      if (!options.silent) {
        setAgentError(err.message || t("modelLoadFailed"));
      }
      if (modelRequestKey(agentDraftRef.current) === requestKey) {
        setAgentModels([]);
      }
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

  async function publishAgentPage() {
    if (!selectedAgentForPage?.id || agentPagePublishBusy) {
      return;
    }
    setAgentPagePublishBusy(true);
    setAgentPageError("");
    try {
      const resp = await fetch("/api/v1/hub/templates", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agent_id: selectedAgentForPage.id,
        }),
      });
      if (!resp.ok) {
        throw new Error((await readErrorMessage(resp)) || t("agentActionFailed"));
      }
      const published = await resp.json();
      await refreshHubTemplates();
      if (published?.id) {
        setSelectedHubTemplateId(published.id);
      }
      selectHub();
    } catch (err) {
      setAgentPageError(err.message || t("agentActionFailed"));
    } finally {
      setAgentPagePublishBusy(false);
    }
  }

  async function saveAgent() {
    if (!agentDraft) {
      return;
    }
    setAgentBusy(true);
    setAgentError("");
    const isCreate = agentModalMode === "create";
    const runtimeKind = normalizeRuntimeKind(agentDraft.runtime_kind);
    setAgentProgress(isCreate ? startAgentCreateProgress(runtimeKind) : null);
    try {
      const profile = draftToProfile(agentDraft, {
        name: agentDraft.name,
        description: agentDraft.description,
      });
      const payload = {
        name: agentDraft.name,
        role: agentDraft.role,
        description: agentDraft.description,
        image: agentDraft.image,
        runtime_kind: runtimeKind,
        from_template: agentDraft.from_template || "",
        agent_profile: profile,
      };
      const url = isCreate ? "api/v1/bots" : `api/v1/agents/${encodeURIComponent(editingAgent.id)}`;
      const resp = await fetch(url, {
        method: isCreate ? "POST" : "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(isCreate ? payload : {
          name: payload.name,
          description: payload.description,
          agent_profile: payload.agent_profile,
        }),
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
      if (isCreate) {
        setAgentProgress((current) => current ? { ...current, percent: 100, status: "done", index: Math.max(0, (current.steps?.length || 1) - 1) } : current);
      }
      setShowAgentModal(false);
      setAgentDraft(null);
      setAgentProgress(null);
    } catch (err) {
      setAgentProgress((current) => current ? { ...current, status: "failed" } : current);
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
      if (action === "recreate" && isManagerAgent(item)) {
        openManagerRebuildModal(item);
        return;
      }
      const url = action === "delete"
        ? `api/v1/bots/${encodeURIComponent(item.id)}`
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

  async function deletePreviewBot(item) {
    if (!item?.id || agentActionBusy) {
      return;
    }
    if (!window.confirm(`${t("agentDelete")} ${item.name}?`)) {
      return;
    }
    setAgentActionBusy(`${item.id}:delete-bot`);
    setAgentsError("");
    try {
      const resp = await fetch(`api/v1/bots/${encodeURIComponent(item.id)}`, { method: "DELETE" });
      if (!resp.ok) {
        throw new Error((await resp.text()).trim());
      }
      closeProfilePreview();
      await refreshAgents();
      await refreshBootstrap();
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

  function openParticipantPreview(user, anchor) {
    if (!user?.id) {
      return;
    }
    const rect = anchor?.getBoundingClientRect?.();
    if (!rect) {
      return;
    }
    const agent = agents.find((item) => agentMatchesUser(item, user));
    setProfilePreview((current) => {
      const nextType = agent ? "agent" : "user";
      const nextID = agent ? agent.id : user.id;
      if (current?.type === nextType && current?.id === nextID) {
        return null;
      }
      return {
        type: nextType,
        id: nextID,
        anchorRect: {
          top: rect.top,
          right: rect.right,
          bottom: rect.bottom,
          left: rect.left,
        },
        anchorEl: anchor,
      };
    });
    setShowMemberList(false);
    setShowChannelTools(false);
  }

  function openAgentPreview(item, anchor) {
    if (!item?.id) {
      return;
    }
    const rect = anchor?.getBoundingClientRect?.();
    if (!rect) {
      return;
    }
    setProfilePreview((current) => {
      if (current?.type === "agent" && current?.id === item.id) {
        return null;
      }
      return {
        type: "agent",
        id: item.id,
        anchorRect: {
          top: rect.top,
          right: rect.right,
          bottom: rect.bottom,
          left: rect.left,
        },
        anchorEl: anchor,
      };
    });
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
                <div className="sidebar-brand-lockup" aria-label="CSGClaw">
                  <div className="sidebar-brand-mark sidebar-brand-wordmark" aria-hidden="true">CSGClaw</div>
                </div>
                <div className="sidebar-controls">
                  <div className="theme-switch" role="group" aria-label=${t("themeSwitcher")}>
                    <div className=${`theme-switch-track ${theme === "dark" ? "is-dark" : "is-light"}`}>
                      <span className="theme-switch-thumb" aria-hidden="true"></span>
                      <button
                        className=${`btn btn-ghost theme-toggle ${theme === "light" ? "active" : ""}`}
                        aria-label=${t("themeLight")}
                        aria-pressed=${theme === "light"}
                        title=${t("themeLight")}
                        onClick=${() => setTheme("light")}
                      >
                        <span aria-hidden="true"><${SunIcon} /></span>
                      </button>
                      <button
                        className=${`btn btn-ghost theme-toggle ${theme === "dark" ? "active" : ""}`}
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
                      <button className=${`btn btn-ghost language-toggle ${locale === "zh" ? "active" : ""}`} aria-pressed=${locale === "zh"} title=${t("languageOptionZh")} onClick=${() => setLocale("zh")}>中</button>
                      <button className=${`btn btn-ghost language-toggle ${locale === "en" ? "active" : ""}`} aria-pressed=${locale === "en"} title=${t("languageOptionEn")} onClick=${() => setLocale("en")}>EN</button>
                    </div>
                  </div>
                  <button
                    className="btn btn-ghost btn-sm sidebar-toggle-button"
                    aria-label=${t("collapseSidebar")}
                    title=${t("collapseSidebar")}
                    onClick=${() => setIsSidebarCollapsed(true)}
                  >
                    <span className="sidebar-toggle-mark"><${SidebarToggleIcon} /></span>
                  </button>
                </div>
              </div>
              <div className="workspace-signal-panel" aria-label=${currentWorkspaceLabel}>
                <div className="workspace-signal-copy">
                  <span>${currentWorkspaceLabel}</span>
                  <strong>${runningAgentCount}/${agentItems.length || 0} ${t("activeNow")}</strong>
                </div>
                <div className="workspace-signal-meter" aria-hidden="true">
                  <span></span>
                  <span></span>
                  <span></span>
                </div>
              </div>
            </div>
            <nav className="workspace-nav" aria-label="Workspace">
              <div className="workspace-tabbar" role="tablist" aria-label="Workspace sections">
                <button
                  className=${`btn btn-secondary-gray btn-sm workspace-tab ${workspaceTab === WORKSPACE_TAB_MESSAGES ? "active" : ""}`}
                  role="tab"
                  aria-selected=${workspaceTab === WORKSPACE_TAB_MESSAGES}
                  aria-label=${t("messagesTab")}
                  title=${t("messagesTab")}
                  onClick=${() => setWorkspaceTab(WORKSPACE_TAB_MESSAGES)}
                >
                  <span className="workspace-tab-icon" aria-hidden="true"><${RoomsIcon} /></span>
                  <span className="workspace-tab-copy">
                    <strong>${t("messagesTab")}</strong>
                    <small>${roomCount}</small>
                  </span>
                </button>
                <button
                  className=${`btn btn-secondary-gray btn-sm workspace-tab ${workspaceTab === WORKSPACE_TAB_AGENTS ? "active" : ""}`}
                  role="tab"
                  aria-selected=${workspaceTab === WORKSPACE_TAB_AGENTS}
                  aria-label=${t("agentsTab")}
                  title=${t("agentsTab")}
                  onClick=${() => setWorkspaceTab(WORKSPACE_TAB_AGENTS)}
                >
                  <span className="workspace-tab-icon" aria-hidden="true"><${UsersIcon} /></span>
                  <span className="workspace-tab-copy">
                    <strong>${t("agentsTab")}</strong>
                    <small>${agentItems.length}</small>
                  </span>
                </button>
                <button
                  className=${`btn btn-secondary-gray btn-sm workspace-tab ${workspaceTab === WORKSPACE_TAB_HUB ? "active" : ""}`}
                  role="tab"
                  aria-selected=${workspaceTab === WORKSPACE_TAB_HUB}
                  aria-label=${t("hubTab")}
                  title=${t("hubTab")}
                  onClick=${() => selectHub()}
                >
                  <span className="workspace-tab-icon" aria-hidden="true"><${HubIcon} /></span>
                  <span className="workspace-tab-copy">
                    <strong>${t("hubTab")}</strong>
                    <span className="workspace-tab-badge">${t("newBadge")}</span>
                  </span>
                </button>
              </div>
              ${workspaceTab === WORKSPACE_TAB_MESSAGES
                ? html`
                    <div className="workspace-tab-panel" role="tabpanel" aria-label=${t("messagesTab")}>
                      <${WorkspaceGroup}
                        id="rooms"
                        title=${t("channelsSection")}
                        count=${channels.length}
                        collapsed=${Boolean(collapsedWorkspaceGroups.rooms)}
                        onToggle=${() => toggleWorkspaceGroup("rooms")}
                        onAdd=${() => openCreateRoomModal()}
                        addLabel=${t("createRoom")}
                      >
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
                      <${WorkspaceGroup}
                        id="direct-messages"
                        title=${t("directMessagesSection")}
                        count=${directMessages.length}
                        collapsed=${Boolean(collapsedWorkspaceGroups["direct-messages"])}
                        onToggle=${() => toggleWorkspaceGroup("direct-messages")}
                      >
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
                    </div>
                  `
                : workspaceTab === WORKSPACE_TAB_HUB
                  ? html`
                      <div className="workspace-tab-panel" role="tabpanel" aria-label=${t("hubTab")}>
                        <${WorkspaceGroup}
                          id="hub"
                          title=${t("hubTemplatesSection")}
                          count=${hubTemplates.length}
                          collapsed=${Boolean(collapsedWorkspaceGroups.hub)}
                          onToggle=${() => toggleWorkspaceGroup("hub")}
                        >
                          ${hubError
                            ? html`<div className="workspace-empty">${hubError}</div>`
                            : hubLoaded && hubTemplates.length === 0
                              ? html`<div className="workspace-empty">${t("hubEmpty")}</div>`
                              : hubTemplates.slice(0, 6).map((item) => html`
                                  <button key=${item.id} className=${`workspace-row hub-template-row ${selectedHubTemplateId === item.id ? "active" : ""}`} onClick=${() => selectHubTemplate(item)}>
                                    <span className="workspace-row-icon"><${HubIcon} /></span>
                                    <span className="workspace-row-main">
                                      <span className="workspace-row-title truncate">${item.name || item.id}</span>
                                      <span className="workspace-row-meta truncate">${item.description || item.source?.name || item.id}</span>
                                    </span>
                                    <span className="mini-badge template-source-badge"><span className="template-source-badge-dot" aria-hidden="true"></span>${localizeTemplateSourceTag(item.source?.name, locale)}</span>
                                  </button>
                                `)}
                        <//>
                      </div>
                    `
                : html`
                    <div className="workspace-tab-panel" role="tabpanel" aria-label=${t("agentsTab")}>
                      <${WorkspaceGroup}
                        id="agents"
                        title=${t("computerAgentsSection")}
                        count=${agentItems.length}
                        collapsed=${Boolean(collapsedWorkspaceGroups.agents)}
                        onToggle=${() => toggleWorkspaceGroup("agents")}
                        onAdd=${openCreateAgentModal}
                        addLabel=${t("createAgent")}
                      >
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
                      <${WorkspaceGroup}
                        id="computers"
                        title=${t("computersSection")}
                        count=${1}
                        collapsed=${Boolean(collapsedWorkspaceGroups.computers)}
                        onToggle=${() => toggleWorkspaceGroup("computers")}
                      >
                        <${WorkspaceComputerRow}
                          title=${t("localComputer")}
                          active=${activePane.type === "computer"}
                          subtitle=${`${agentItems.length} ${t("computerAgentsSection")}`}
                          onSelect=${selectComputer}
                        />
                      <//>
                    </div>
                  `}
              ${agentsError ? html`<div className="form-error agent-error">${agentsError}</div>` : null}
            </nav>
            <div className="sidebar-footer">
              <div className="sidebar-footer-row">
                <span className="sidebar-version-label">${formatSidebarVersionLabel(appVersion)}</span>
                ${upgradeStatus?.update_available || upgradeBusy || upgradeStatus?.upgrading || upgradePhase === "done" || upgradePhase === "error"
                  ? html`
                      <button
                        type="button"
                        className=${`sidebar-upgrade-button ${upgradeBusy || upgradeStatus?.upgrading ? "is-running" : ""} ${upgradePhase === "done" ? "is-done" : ""}`}
                        onClick=${() => {
                          setUpgradeError("");
                          setUpgradePhase(upgradeBusy || upgradeStatus?.upgrading ? "restarting" : "idle");
                          setShowUpgradeModal(true);
                        }}
                      >
                        <span className="sidebar-upgrade-dot" aria-hidden="true"></span>
                        <span>${upgradePhase === "done" ? t("upgradeRefresh") : upgradeBusy || upgradeStatus?.upgrading ? t("upgradeBackground") : t("upgradeAction")}</span>
                      </button>
                    `
                  : null}
              </div>
              ${upgradeError ? html`<div className="sidebar-footer-error">${upgradeError}</div>` : null}
            </div>
          </aside>

          <div
            className=${`sidebar-rail ${isSidebarCollapsed ? "visible" : ""}`}
            aria-hidden=${!isSidebarCollapsed}
            inert=${!isSidebarCollapsed}
          >
            <button className="btn btn-ghost btn-sm sidebar-expand-button" aria-label=${t("expandSidebar")} title=${t("expandSidebar")} onClick=${() => setIsSidebarCollapsed(false)}>
              <span className="sidebar-toggle-mark"><${SidebarToggleIcon} /></span>
            </button>
            <nav className="sidebar-rail-nav" aria-label="Workspace">
              <button className=${`btn btn-ghost btn-sm sidebar-rail-button ${workspaceTab === WORKSPACE_TAB_MESSAGES ? "active" : ""}`} aria-label=${t("messagesTab")} title=${t("messagesTab")} onClick=${() => setWorkspaceTab(WORKSPACE_TAB_MESSAGES)}>
                <span className="sidebar-rail-icon" aria-hidden="true"><${RoomsIcon} /></span>
              </button>
              <button type="button" className=${`btn btn-ghost btn-sm sidebar-rail-button ${workspaceTab === WORKSPACE_TAB_AGENTS ? "active" : ""}`} aria-label=${t("agentsTab")} title=${t("agentsTab")} onClick=${() => setWorkspaceTab(WORKSPACE_TAB_AGENTS)}>
                <span className="sidebar-rail-icon" aria-hidden="true"><${UsersIcon} /></span>
              </button>
              <button type="button" className=${`btn btn-ghost btn-sm sidebar-rail-button ${workspaceTab === WORKSPACE_TAB_HUB ? "active" : ""}`} aria-label=${t("hubTab")} title=${t("hubTab")} onClick=${() => selectHub()}>
                <span className="sidebar-rail-icon" aria-hidden="true"><${HubIcon} /></span>
              </button>
              <button type="button" className="btn btn-ghost btn-sm sidebar-rail-button" aria-label=${t("createRoom")} title=${t("createRoom")} onClick=${() => openCreateRoomModal()}>
                <span className="sidebar-rail-icon" aria-hidden="true"><${RoomPlusIcon} /></span>
              </button>
            </nav>
          </div>
        </div>

        <main className="chat-panel">
          ${activePane.type === "hub"
            ? html`
                <${HubDetailPane}
                  t=${t}
                  locale=${locale}
                  templates=${hubTemplates}
                  selectedTemplate=${selectedHubTemplateView}
                  selectedTemplateId=${selectedHubTemplateId}
                  loaded=${hubLoaded}
                  error=${hubError || hubTemplateDetailError}
                  detailLoading=${hubTemplateDetailLoading}
                  selectedWorkspacePath=${selectedHubWorkspacePath}
                  workspaceFile=${hubWorkspaceFile}
                  workspaceFileLoading=${hubWorkspaceFileLoading}
                  workspaceFileError=${hubWorkspaceFileError}
                  onRetry=${async () => {
                    await refreshHubTemplates();
                    if (selectedHubTemplateId) {
                      await loadHubTemplateDetail(selectedHubTemplateId);
                    }
                  }}
                  onSelectTemplate=${selectHubTemplate}
                  onSelectWorkspaceFile=${(workspacePath) => {
                    setSelectedHubWorkspacePath(workspacePath);
                    loadHubWorkspaceFile(selectedHubTemplateId, workspacePath);
                  }}
                  onCreateFromTemplate=${openCreateAgentModal}
                />
              `
            : activePane.type === "agent" && selectedAgent
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
                  publishBusy=${agentPagePublishBusy}
                  saveError=${agentPageError}
                  authStatuses=${cliproxyAuthStatuses}
                  authBusyProvider=${cliproxyAuthBusy}
                  onDraftChange=${setAgentPageDraft}
                  onSave=${saveAgentPage}
                  onPublish=${publishAgentPage}
                  onProviderLogin=${loginCLIProxyProvider}
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
                          <div className="chat-kicker">
                            <span>${isDirectConversation(activeConversation) ? t("directMessagesSection") : t("conversationLabel")}</span>
                            <strong>${selectedMessageCount}</strong>
                          </div>
                          <div className="chat-title truncate">${activeConversation.title}</div>
                          <div ref=${memberMenuRef} className="header-menu">
                            <button
                              className=${`btn btn-secondary-gray btn-sm member-badge-button ${showMemberList ? "active" : ""}`}
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
                                            onClick=${(event) => openParticipantPreview(user, event.currentTarget)}
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
                            className=${`btn btn-secondary-gray btn-sm icon-button ${showChannelTools ? "active" : ""}`}
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
                                  <button className="btn btn-secondary-gray btn-sm tool-menu-row" onClick=${() => setShowToolCalls((value) => !value)}>
                                    <span>${showToolCalls ? t("toggleToolCallsHide") : t("toggleToolCallsShow")}</span>
                                    <strong>${showToolCalls ? t("enabled") : t("disabled")}</strong>
                                  </button>
                                  ${!isDirectConversation(activeConversation)
                                    ? html`
                                        <button
                                          className="btn btn-outline-danger btn-sm tool-menu-row danger"
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
                          type="button"
                          className="btn btn-secondary-gray btn-sm icon-button"
                          aria-label=${inviteActionLabel}
                          title=${inviteActionLabel}
                          onClick=${(event) => {
                            event.preventDefault();
                            event.stopPropagation();
                            handleInviteAction();
                          }}
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
                    ? html`
                        <div className="messages-empty rich-empty">
                          <span aria-hidden="true" className="rich-empty-mark">></span>
                          <strong>${t("noMessages")}</strong>
                        </div>
                      `
                    : visibleMessages.length === 0
                      ? html`
                          <div className="messages-empty rich-empty">
                            <span aria-hidden="true" className="rich-empty-mark">#</span>
                            <strong>${t("noVisibleMessages")}</strong>
                          </div>
                        `
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
                          onClick=${(event) => openParticipantPreview(user, event.currentTarget)}
                        >${user.avatar}</button>
                        <div className="message-card">
                          <div className="message-meta">
                            <span className="message-author">${user.name}</span>
                            <span>${formatTime(message.created_at, locale)}</span>
                          </div>
                          <div className="message-bubble"><${MessageContent} key=${`${message.id}:${theme}`} content=${message.content} message=${message} actionBusy=${messageActionBusy} actionError=${messageActionError} onAction=${handleMessageAction} /></div>
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
                  ${managerProfile && providerNeedsAuth(managerProfile.provider) && cliproxyAuthStatuses[normalizeAuthProviderName(managerProfile.provider)]?.authenticated === false
                    ? html`<${CLIProxyAuthControl}
                        provider=${managerProfile.provider}
                        t=${t}
                        status=${cliproxyAuthStatuses[normalizeAuthProviderName(managerProfile.provider)]}
                        busy=${cliproxyAuthBusy === normalizeAuthProviderName(managerProfile.provider)}
                        onLogin=${loginCLIProxyProvider}
                      />`
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
                          const pasted = event.clipboardData?.getData("text/plain") ?? "";
                          const segments = normalizeTextMentions([{ type: "text", text: pasted }], mentionableUsersByHandle);
                          if (segments.some((segment) => segment.type === "mention")) {
                            insertComposerSegmentsAtSelection(segments);
                          } else {
                            insertPlainTextAtSelection(pasted);
                          }
                          syncComposerFromEditor();
                        }}
                      />
                      <button
                        type="button"
                        className="btn btn-primary btn-sm composer-send-button"
                        aria-label=${t("send")}
                        title=${t("send")}
                        disabled=${managerProfileIncomplete || !draftText.trim()}
                        onClick=${sendMessage}
                      >
                        <span className="composer-send-main" aria-hidden="true">
                          ${IconImage("send")}
                        </span>
                      </button>
                    </div>
                  </div>
                  ${composerError ? html`<div className="form-error composer-error">${composerError}</div>` : null}
                  <div className="composer-tip">${t("composerTip")}</div>
                </footer>
              `
            : html`
                <div className="empty-state shell-empty-state">
                  <span className="rich-empty-mark" aria-hidden="true">></span>
                  <strong>${t("emptyConversation")}</strong>
                </div>
              `}
        </main>
      </div>

      ${profilePreview && (previewAgent || previewUser)
        ? html`
            <${ProfilePreviewPopover}
              previewRef=${profilePreviewRef}
              agent=${previewAgent}
              user=${previewUser}
              anchorRect=${profilePreview.anchorRect}
              t=${t}
              inDirectConversation=${Boolean(selectedConversation && isDirectConversation(selectedConversation))}
              busyKey=${agentActionBusy}
              onClose=${closeProfilePreview}
              onOpenAgent=${(item) => {
                selectAgent(item);
                closeProfilePreview();
              }}
              onOpenDM=${openAgentDirectMessage}
              onDelete=${deletePreviewBot}
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
                  <button className="btn btn-secondary-gray btn-sm modal-close" onClick=${() => setShowCreateRoom(false)}>${t("close")}</button>
                </div>
                <label className="field">
                  ${requiredFieldLabel(t("roomName"))}
                  <input
                    value=${roomTitle}
                    required
                    aria-required="true"
                    onInput=${(event) => setRoomTitle(event.target.value)}
                    placeholder=${t("roomNamePlaceholder")}
                  />
                </label>
                <label className="field">
                  <span>${t("roomDescription")}</span>
                  <textarea value=${roomDescription} onInput=${(event) => setRoomDescription(event.target.value)} placeholder=${t("roomDescriptionPlaceholder")} />
                </label>
                <div className="field">
                  <span>${t("initialMembers")}</span>
                  <div className="selection-list">
                    <label className="selection-item selection-all-item">
                      <input
                        type="checkbox"
                        checked=${allCreateRoomMembersSelected}
                        disabled=${createRoomSelectableMemberIDs.length === 0}
                        onChange=${() => {
                          setRoomMemberIDs((current) => {
                            const allSelected = createRoomCandidateIDs.length > 0 && createRoomCandidateIDs.every((id) => current.includes(id));
                            if (allSelected) {
                              return current.filter((id) => !createRoomSelectableMemberIDs.includes(id));
                            }
                            return Array.from(new Set([...current, ...createRoomSelectableMemberIDs]));
                          });
                        }}
                      />
                      <span>${t("allMembers")}</span>
                      <small>${createRoomSelectedMemberCount}/${createRoomCandidateIDs.length}</small>
                    </label>
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
                  <button className="btn btn-secondary-gray btn-sm secondary-button" onClick=${() => setShowCreateRoom(false)}>${t("cancel")}</button>
                  <button className="btn btn-primary btn-sm send-button" disabled=${isBlank(roomTitle)} onClick=${createRoom}>${t("create")}</button>
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
                  <button className="btn btn-secondary-gray btn-sm modal-close" onClick=${() => setShowInvite(false)}>${t("close")}</button>
                </div>
                <div className="field">
                  <span>${t("inviteCandidates")}</span>
                  <div className="selection-list">
                    ${inviteCandidates.length > 0
                      ? html`
                          <label className="selection-item selection-all-item">
                            <input
                              type="checkbox"
                              checked=${allInviteCandidatesSelected}
                              onChange=${() => {
                                setInviteUserIDs((current) => {
                                  const allSelected = inviteCandidateIDs.length > 0 && inviteCandidateIDs.every((id) => current.includes(id));
                                  if (allSelected) {
                                    return current.filter((id) => !inviteCandidateIDs.includes(id));
                                  }
                                  return Array.from(new Set([...current, ...inviteCandidateIDs]));
                                });
                              }}
                            />
                            <span>${t("allMembers")}</span>
                            <small>${inviteSelectedMemberCount}/${inviteCandidateIDs.length}</small>
                          </label>
                          ${inviteCandidates.map((user) => html`
                            <label key=${user.id} className="selection-item">
                              <input
                                type="checkbox"
                                checked=${inviteUserIDs.includes(user.id)}
                                onChange=${() => setInviteUserIDs((current) => toggleSelection(current, user.id))}
                              />
                              <span>${user.name}</span>
                              <small>@${user.handle}</small>
                            </label>
                          `)}
                        `
                      : html`<div className="selection-empty">${t("noInviteCandidates")}</div>`}
                  </div>
                </div>
                ${submitError ? html`<div className="form-error">${submitError}</div>` : null}
                <div className="modal-actions">
                  <button className="btn btn-secondary-gray btn-sm secondary-button" onClick=${() => setShowInvite(false)}>${t("cancel")}</button>
                  <button className="btn btn-primary btn-sm send-button" disabled=${inviteUserIDs.length === 0} onClick=${inviteUsers}>${t("sendInvite")}</button>
                </div>
              </div>
            </div>
          `
        : null}

      ${showUpgradeModal
        ? html`
            <div className="modal-backdrop" onClick=${() => setShowUpgradeModal(false)}>
              <div className="modal-card upgrade-modal" onClick=${(event) => event.stopPropagation()}>
                <div className="modal-header">
                  <div>
                    <div className="modal-title">${t("upgradeTitle")}</div>
                    <div className="modal-subtitle">${t("upgradeSubtitle")}</div>
                  </div>
                  <button
                    className="modal-close"
                    onClick=${() => setShowUpgradeModal(false)}
                  >
                    ${t("close")}
                  </button>
                </div>
                <div className="upgrade-summary">
                  <div className="upgrade-summary-row">
                    <span>${t("upgradeCurrentVersion")}</span>
                    <strong>${upgradeStatus?.current_version || appVersion || "dev"}</strong>
                  </div>
                  <div className="upgrade-summary-row">
                    <span>${t("upgradeLatestVersion")}</span>
                    <strong>${upgradeStatus?.latest_version || t("upgradeNoLatest")}</strong>
                  </div>
                  <div className="upgrade-summary-row">
                    <span>${t("upgradeStatus")}</span>
                    <strong>${upgradeStatusLabel(upgradePhase, t)}</strong>
                  </div>
                </div>
                <div className=${`upgrade-status-card ${upgradePhase}`}>
                  <span className="upgrade-status-dot" aria-hidden="true"></span>
                  <p>
                    ${upgradePhase === "done"
                      ? t("upgradeDoneBody")
                      : upgradePhase === "restarting" || upgradePhase === "starting" || upgradeBusy || upgradeStatus?.upgrading
                        ? t("upgradeContinueUsing")
                        : t("upgradeConfirmBody")}
                  </p>
                </div>
                ${upgradeError || upgradeStatus?.last_error
                  ? html`<div className="form-error">${upgradeError || upgradeStatus.last_error}</div>`
                  : null}
                <div className="modal-actions">
                  ${upgradePhase === "done"
                    ? html`
                        <button className="send-button" onClick=${() => window.location.reload()}>
                          ${t("upgradeRefresh")}
                        </button>
                      `
                    : html`
                        <button
                          className="secondary-button"
                          onClick=${() => setShowUpgradeModal(false)}
                        >
                          ${upgradeBusy || upgradeStatus?.upgrading ? t("close") : t("upgradeLater")}
                        </button>
                        <button
                          className="send-button"
                          disabled=${upgradeBusy || upgradeStatus?.upgrading || !upgradeStatus?.update_available}
                          onClick=${applyUpgrade}
                        >
                          ${upgradeBusy || upgradeStatus?.upgrading ? t("upgradeActionBusy") : t("upgradeConfirm")}
                        </button>
                      `}
                </div>
              </div>
            </div>
          `
        : null}

      ${showManagerRebuildModal
        ? html`
            <div className="modal-backdrop">
              <div className="modal-card profile-modal" onClick=${(event) => event.stopPropagation()}>
                <div className="modal-header">
                  <div>
                    <div className="modal-title">${t("managerRebuildTitle")}</div>
                    <div className="modal-subtitle">${t("managerRebuildSubtitle")}</div>
                  </div>
                  <button className="btn btn-secondary-gray btn-sm modal-close" onClick=${() => setShowManagerRebuildModal(false)}>${t("close")}</button>
                </div>
                <div className="profile-editor-shell">
                  <section className="profile-section">
                    <div className="profile-grid profile-grid-compact manager-rebuild-grid">
                      <label className="field manager-rebuild-runtime-field">
                        <span>${t("profileRuntimeKind")}</span>
                        <select
                          value=${normalizeRuntimeKind(managerRebuildRuntimeKind)}
                          onChange=${(event) => {
                            const runtimeKind = normalizeRuntimeKind(event.target.value);
                            setManagerRebuildRuntimeKind(runtimeKind);
                            setManagerRebuildImage(runtimeImageForKind(runtimeKind, bootstrapConfig, managerAgent?.image || ""));
                          }}
                        >
                          ${managerRuntimeOptions.map((option) => html`
                            <option key=${option.value} value=${option.value}>${option.value}</option>
                          `)}
                        </select>
                      </label>
                      <label className="field manager-rebuild-image-field">
                        <span>${t("agentImage")}</span>
                        <input value=${managerRebuildImage} onInput=${(event) => setManagerRebuildImage(event.target.value)} placeholder=${t("agentImagePlaceholder")} />
                      </label>
                    </div>
                  </section>
                  ${agentsError ? html`<div className="form-error">${agentsError}</div>` : null}
                  <div className="modal-actions">
                    <button className="secondary-button" disabled=${agentActionBusy === "u-manager:recreate"} onClick=${() => setShowManagerRebuildModal(false)}>
                      ${t("close")}
                    </button>
                    <button className="send-button" disabled=${agentActionBusy === "u-manager:recreate"} onClick=${confirmManagerRebuild}>
                      ${agentActionBusy === "u-manager:recreate" ? t("profileLoadingModels") : t("managerRebuildAction")}
                    </button>
                  </div>
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
                  <button className="btn btn-secondary-gray btn-sm modal-close" onClick=${() => setShowAgentModal(false)}>${t("close")}</button>
                </div>
                <div className="profile-editor-shell">
                  <section className="profile-section">
                    <div className="profile-section-title">${t("profileBasics")}</div>
                    <div className="profile-grid profile-grid-compact">
                      ${agentModalMode === "create"
                        ? html`
                            <label className="field span-2">
                              <span>${t("templateLabel")}</span>
                              <select
                                value=${agentDraft.from_template || ""}
                                onChange=${(event) => {
                                  const nextTemplate = normalizeTemplateSelection(hubTemplates.find((item) => item.id === event.target.value) || null);
                                  setAgentDraft((current) => applyTemplateToDraft(current, nextTemplate, bootstrapConfig, managerAgent?.image || ""));
                                }}
                              >
                                <option value="">${t("templateNone")}</option>
                                ${hubTemplates.map((item) => html`
                                  <option key=${item.id} value=${item.id}>${item.name || item.id}</option>
                                `)}
                              </select>
                            </label>
                          `
                        : null}
                      <label className="field">
                        ${requiredFieldLabel(t("agentName"))}
                        <input
                          value=${agentDraft.name}
                          disabled=${agentModalMode === "edit" && editingAgent?.id === "u-manager"}
                          required
                          aria-required="true"
                          onInput=${(event) => setAgentDraft({ ...agentDraft, name: event.target.value })}
                          placeholder=${t("agentNamePlaceholder")}
                        />
                      </label>
                      ${agentModalMode === "create"
                        ? html`
                            <label className="field">
                              <span>${t("roleLabel")}</span>
                              <input value=${agentDraft.role || "worker"} readOnly disabled />
                            </label>
                          `
                        : null}
                      <label className="field">
                        <span>${t("profileRuntimeKind")}</span>
                        ${agentModalMode === "create"
                          ? html`
                              <select
                                value=${normalizeRuntimeKind(agentDraft.runtime_kind)}
                                onChange=${(event) => {
                                  const runtimeKind = normalizeRuntimeKind(event.target.value);
                                  const currentTemplate = normalizeTemplateSelection(hubTemplates.find((item) => item.id === agentDraft.from_template) || null);
                                  const nextTemplate = templateMatchesRuntime(currentTemplate, runtimeKind)
                                    ? currentTemplate
                                    : pickDefaultAgentTemplate(hubTemplates, runtimeKind, bootstrapConfig);
                                  let nextDraft = {
                                    ...agentDraft,
                                    runtime_kind: runtimeKind,
                                    image: runtimeImageForKind(runtimeKind, bootstrapConfig, agentDraft.default_image || managerAgent?.image || ""),
                                  };
                                  nextDraft = applyTemplateToDraft(nextDraft, nextTemplate, bootstrapConfig, managerAgent?.image || "");
                                  setAgentDraft(nextDraft);
                                }}
                              >
                                ${RUNTIME_KIND_OPTIONS.map((option) => html`
                                  <option key=${option.value} value=${option.value}>${option.label}</option>
                                `)}
                              </select>
                            `
                          : html`<input value=${agentDraft.runtime_kind || editingAgent?.runtime_kind || ""} readOnly disabled />`}
                      </label>
                      <label className="field">
                        <span>${t("agentImage")}</span>
                        ${agentModalMode === "create"
                          ? html`<input value=${agentDraft.image} onInput=${(event) => setAgentDraft({ ...agentDraft, image: event.target.value })} placeholder=${t("agentImagePlaceholder")} />`
                          : html`<input value=${agentDraft.image} readOnly disabled placeholder=${t("agentImagePlaceholder")} />`}
                      </label>
                      <label className="field span-2">
                        <span>${t("agentDescription")}</span>
                        <textarea className="compact-textarea" value=${agentDraft.description} onInput=${(event) => setAgentDraft({ ...agentDraft, description: event.target.value })} />
                      </label>
                    </div>
                  </section>
                  <section className="profile-section">
                    <div className="profile-section-title">${t("profileModelSection")}</div>
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
                        ${requiredFieldLabel(t("profileModel"))}
                        <select
                          value=${agentDraft.model_id}
                          required
                          aria-required="true"
                          onChange=${(event) => setAgentDraft({ ...agentDraft, model_id: event.target.value })}
                        >
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
                    <${CLIProxyAuthControl}
                      provider=${agentDraft.provider}
                      t=${t}
                      status=${cliproxyAuthStatuses[normalizeAuthProviderName(agentDraft.provider)]}
                      busy=${cliproxyAuthBusy === normalizeAuthProviderName(agentDraft.provider)}
                      onLogin=${loginCLIProxyProvider}
                    />
                  </section>
                  ${agentDraft.provider === "api"
                    ? html`
                        <section className="profile-section">
                          <div className="profile-section-title">${t("profileAPIProvider")}</div>
                          <div className="profile-api-grid">
                            <label className="field">
                              ${requiredFieldLabel(t("profileBaseURL"))}
                              <input
                                value=${agentDraft.base_url}
                                required
                                aria-required="true"
                                onInput=${(event) => setAgentDraft({ ...agentDraft, base_url: event.target.value })}
                                placeholder="https://api.openai.com/v1"
                              />
                            </label>
                            <${APIKeyField}
                              value=${agentDraft.api_key}
                              onInput=${(event) => setAgentDraft({ ...agentDraft, api_key: event.target.value })}
                              profile=${agentDraft}
                              t=${t}
                            />
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
                <${AgentCreateProgress} progress=${agentProgress} t=${t} />
                <div className="modal-actions">
                  <button className="btn btn-secondary-gray btn-sm secondary-button" onClick=${() => setShowAgentModal(false)}>${t("cancel")}</button>
                  <button className="btn btn-primary btn-sm send-button" disabled=${agentBusy || isBlank(agentDraft.name) || !agentDraft.model_id || profileBaseURLMissing(agentDraft)} onClick=${saveAgent}>
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
                    <div className="profile-section-title">${t("profileModelSection")}</div>
                    <div className="profile-runtime-grid">
                      <label className="field">
                        <span>${t("profileRuntimeKind")}</span>
                        <select
                          value=${normalizeRuntimeKind(profileDraft.runtime_kind || bootstrapConfig?.runtime_kind)}
                          onChange=${(event) => setProfileDraft({ ...profileDraft, runtime_kind: event.target.value })}
                        >
                          ${GATEWAY_RUNTIME_KIND_OPTIONS.map((option) => html`<option key=${option.value} value=${option.value}>${formatRuntimeKindLabel(option.value, t)}</option>`)}
                        </select>
                      </label>
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
                        ${requiredFieldLabel(t("profileModel"))}
                        <select
                          value=${profileDraft.model_id}
                          required
                          aria-required="true"
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
                    <${CLIProxyAuthControl}
                      provider=${profileDraft.provider}
                      t=${t}
                      status=${cliproxyAuthStatuses[normalizeAuthProviderName(profileDraft.provider)]}
                      busy=${cliproxyAuthBusy === normalizeAuthProviderName(profileDraft.provider)}
                      onLogin=${loginCLIProxyProvider}
                    />
                  </section>
                  ${profileDraft.provider === "api"
                    ? html`
                        <section className="profile-section">
                          <div className="profile-section-title">${t("profileAPIProvider")}</div>
                          <div className="profile-api-grid">
                            <label className="field">
                              ${requiredFieldLabel(t("profileBaseURL"))}
                              <input
                                value=${profileDraft.base_url}
                                required
                                aria-required="true"
                                onInput=${(event) => setProfileDraft({ ...profileDraft, base_url: event.target.value })}
                                placeholder="https://api.openai.com/v1"
                              />
                            </label>
                            <${APIKeyField}
                              value=${profileDraft.api_key}
                              onInput=${(event) => setProfileDraft({ ...profileDraft, api_key: event.target.value })}
                              profile=${profileDraft}
                              t=${t}
                            />
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
                  <button className="btn btn-primary btn-sm send-button" disabled=${profileBusy || !profileDraft.model_id || profileBaseURLMissing(profileDraft)} onClick=${saveManagerProfile}>
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
              className="btn btn-outline-danger btn-sm conversation-delete-button"
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
        <button className="btn btn-secondary-gray btn-sm agent-add-button" aria-label=${t("createAgent")} title=${t("createAgent")} onClick=${onCreate}>
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
        <button className="btn btn-secondary-gray btn-sm agent-icon-button" aria-label=${t("editProfile")} title=${t("editProfile")} onClick=${() => onEdit(item)}>
          <span aria-hidden="true"><${WrenchIcon} /></span>
        </button>
        <button className="btn btn-secondary-gray btn-sm agent-icon-button" aria-label=${running ? t("agentStop") : t("agentStart")} title=${running ? t("agentStop") : t("agentStart")} disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => running ? onStop(item) : onStart(item)}>
          <span aria-hidden="true">${running ? html`<${StopIcon} />` : html`<${PlayIcon} />`}</span>
        </button>
        <button className="btn btn-secondary-gray btn-sm agent-action-text" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>
        ${activeRoom && !isManager
          ? html`<button className="btn btn-secondary-gray btn-sm agent-action-text" disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onInvite(item)}>${t("inviteToRoom")}</button>`
          : null}
        ${!isManager
          ? html`
              <button className="btn btn-outline-danger btn-sm agent-icon-button danger" aria-label=${t("agentDelete")} title=${t("agentDelete")} disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onDelete(item)}>
                <span aria-hidden="true"><${TrashIcon} /></span>
              </button>
            `
          : null}
      </div>
    </div>
  `;
}

function WorkspaceGroup({ id, title, count, collapsed, onToggle, onAdd, addLabel, children }) {
  const itemsID = `workspace-group-items-${id || String(title).toLowerCase().replace(/\s+/g, "-")}`;
  return html`
    <section className=${`workspace-group ${collapsed ? "collapsed" : ""}`}>
      <div className="workspace-group-head">
        <button
          className="workspace-group-toggle"
          type="button"
          aria-expanded=${!collapsed}
          aria-controls=${itemsID}
          onClick=${onToggle}
        >
          <span className="workspace-group-arrow" aria-hidden="true"><${ChevronIcon} /></span>
          <span className="workspace-group-title">
            <span>${title}</span>
            <small>${count}</small>
          </span>
        </button>
        ${onAdd
          ? html`
              <button
                type="button"
                className="btn btn-ghost btn-sm workspace-add-button"
                aria-label=${addLabel || title}
                title=${addLabel || title}
                onClick=${(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  onAdd?.();
                }}
              >
                <span className="icon-button-mark" aria-hidden="true"><${RoomPlusIcon} /></span>
              </button>
            `
          : null}
      </div>
      ${collapsed ? null : html`<div id=${itemsID} className="workspace-group-items">${children}</div>`}
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
          onPreview?.(item, event.currentTarget);
        }}
        onKeyDown=${(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            event.stopPropagation();
            onPreview?.(item, event.currentTarget);
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
              onPreviewUser?.(displayUser, event.currentTarget);
            }
          : undefined}
        onKeyDown=${isDirect && displayUser
          ? (event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                event.stopPropagation();
                onPreviewUser?.(displayUser, event.currentTarget);
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

function hubWorkspaceAncestorDirs(path) {
  const normalized = typeof path === "string" ? path.trim() : "";
  if (!normalized) {
    return [];
  }
  const segments = normalized.split("/").filter(Boolean);
  if (segments.length <= 1) {
    return [];
  }
  const ancestors = [];
  for (let index = 1; index < segments.length; index += 1) {
    ancestors.push(segments.slice(0, index).join("/"));
  }
  return ancestors;
}

function buildVisibleHubWorkspaceEntries(entries, collapsedDirs) {
  const hiddenParents = [];
  return entries.filter((entry) => {
    while (hiddenParents.length && !entry.path.startsWith(`${hiddenParents[hiddenParents.length - 1]}/`)) {
      hiddenParents.pop();
    }
    const visible = hiddenParents.length === 0;
    if (entry.type === "dir" && collapsedDirs[entry.path]) {
      hiddenParents.push(entry.path);
    }
    return visible;
  });
}

function buildInitialCollapsedHubWorkspaceDirs(entries) {
  return (entries || []).reduce((acc, entry) => {
    if (entry?.type === "dir" && entry.path) {
      acc[entry.path] = true;
    }
    return acc;
  }, {});
}

function HubDetailPane({
  t,
  locale,
  templates,
  selectedTemplate,
  selectedTemplateId,
  loaded,
  error,
  detailLoading,
  selectedWorkspacePath,
  workspaceFile,
  workspaceFileLoading,
  workspaceFileError,
  onRetry,
  onSelectTemplate,
  onSelectWorkspaceFile,
  onCreateFromTemplate,
}) {
  const workspaceEntries = selectedTemplate?.workspace?.entries || [];
  const [collapsedWorkspaceDirs, setCollapsedWorkspaceDirs] = useState({});
  const [isTemplateListScrolling, setIsTemplateListScrolling] = useState(false);
  const [isInspectorScrolling, setIsInspectorScrolling] = useState(false);
  const templateListScrollTimerRef = useRef(null);
  const inspectorScrollTimerRef = useRef(null);
  const visibleWorkspaceEntries = useMemo(
    () => buildVisibleHubWorkspaceEntries(workspaceEntries, collapsedWorkspaceDirs),
    [workspaceEntries, collapsedWorkspaceDirs],
  );
  const workspacePreviewText = workspaceFile && !workspaceFile.binary
    ? (workspaceFile.content || t("hubWorkspaceEmptyFile"))
    : "";
  const workspacePreviewLineCount = workspacePreviewText ? workspacePreviewText.split(/\r\n|\r|\n/).length : 0;

  useEffect(() => () => {
    if (templateListScrollTimerRef.current) {
      window.clearTimeout(templateListScrollTimerRef.current);
    }
    if (inspectorScrollTimerRef.current) {
      window.clearTimeout(inspectorScrollTimerRef.current);
    }
  }, []);

  useEffect(() => {
    setCollapsedWorkspaceDirs(buildInitialCollapsedHubWorkspaceDirs(workspaceEntries));
  }, [selectedTemplate?.id, workspaceEntries]);

  useEffect(() => {
    if (!selectedWorkspacePath) {
      return;
    }
    const ancestors = hubWorkspaceAncestorDirs(selectedWorkspacePath);
    if (!ancestors.length) {
      return;
    }
    setCollapsedWorkspaceDirs((current) => {
      let changed = false;
      const next = { ...current };
      ancestors.forEach((path) => {
        if (next[path]) {
          delete next[path];
          changed = true;
        }
      });
      return changed ? next : current;
    });
  }, [selectedWorkspacePath]);

  function toggleWorkspaceDir(path) {
    setCollapsedWorkspaceDirs((current) => ({
      ...current,
      [path]: !current[path],
    }));
  }

  function handleTemplateListScroll() {
    setIsTemplateListScrolling(true);
    if (templateListScrollTimerRef.current) {
      window.clearTimeout(templateListScrollTimerRef.current);
    }
    templateListScrollTimerRef.current = window.setTimeout(() => {
      setIsTemplateListScrolling(false);
      templateListScrollTimerRef.current = null;
    }, 900);
  }

  function handleInspectorScroll() {
    setIsInspectorScrolling(true);
    if (inspectorScrollTimerRef.current) {
      window.clearTimeout(inspectorScrollTimerRef.current);
    }
    inspectorScrollTimerRef.current = window.setTimeout(() => {
      setIsInspectorScrolling(false);
      inspectorScrollTimerRef.current = null;
    }, 900);
  }

  return html`
    <section className="entity-pane hub-detail-pane">
      <header className="hub-page-header">
        <div className="hub-page-heading">
          <h1>${t("hubTitle")}</h1>
          <p>${t("hubSubtitle")}</p>
        </div>
        <button className="btn btn-secondary-gray btn-sm preview-action-button" onClick=${onRetry}>${loaded ? t("hubRefresh") : t("hubLoading")}</button>
      </header>
      ${error ? html`<div className="form-error">${error}</div>` : null}
      ${!loaded && !error
        ? html`<div className="workspace-empty">${t("hubLoading")}</div>`
        : templates.length === 0
          ? html`
              <div className="empty-state shell-empty-state hub-empty-state">
                <span className="rich-empty-mark" aria-hidden="true">*</span>
                <strong>${t("hubEmpty")}</strong>
              </div>
            `
          : html`
              <div className="hub-workbench">
                <div className="hub-catalog-panel">
                  <div className="hub-filter-tabs">
                    <button type="button" className="hub-filter-tab active">${t("hubAllTab")}</button>
                  </div>
                  <div className="hub-catalog-meta">${formatHubTemplateCount(templates.length, locale, t)}</div>
                  <div
                    className=${`hub-template-list ${isTemplateListScrolling ? "is-scrolling" : ""}`}
                    onScroll=${handleTemplateListScroll}
                  >
                    ${templates.map((item) => html`
                      <button
                        key=${item.id}
                        type="button"
                        className=${`hub-template-card ${selectedTemplateId === item.id ? "active" : ""}`}
                        onClick=${() => onSelectTemplate?.(item)}
                      >
                        <div className="hub-template-card-icon"><${HubIcon} /></div>
                        <div className="hub-template-card-body">
                          <div className="hub-template-card-title-row">
                            <h2>${item.name || item.id}</h2>
                          </div>
                          <p>${item.description || item.id}</p>
                          <div className="hub-template-card-meta">
                            <span className="mini-badge template-runtime-badge">${item.runtime_kind || item.workspace?.kind || "-"}</span>
                            <span className="mini-badge template-source-badge"><span className="template-source-badge-dot" aria-hidden="true"></span>${localizeTemplateSourceTag(item.source?.name, locale)}</span>
                            <span className="hub-template-card-updated">${t("hubUpdatedAtLabel")} ${formatHubDate(item.updated_at, locale)}</span>
                          </div>
                        </div>
                      </button>
                    `)}
                  </div>
                </div>

                <div
                  className=${`hub-inspector-panel ${isInspectorScrolling ? "is-scrolling" : ""}`}
                  onScroll=${handleInspectorScroll}
                >
                  ${selectedTemplate ? html`
                    <div className="hub-inspector-hero">
                      <div className="hub-inspector-hero-row">
                        <div className="hub-inspector-brand">
                          <div className="hub-inspector-icon"><${HubIcon} /></div>
                          <div className="hub-inspector-copy">
                            <h2>${selectedTemplate.name || selectedTemplate.id}</h2>
                            <p>${selectedTemplate.description || selectedTemplate.id}</p>
                            <span className="mini-badge template-runtime-badge">${selectedTemplate.runtime_kind || selectedTemplate.workspace?.kind || "-"}</span>
                            <span className="mini-badge template-source-badge"><span className="template-source-badge-dot" aria-hidden="true"></span>${localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}</span>
                          </div>
                        </div>
                        <div className="hub-template-actions">
                          <button
                            type="button"
                            className="btn btn-primary btn-sm preview-action-button preview-action-button-primary"
                            onClick=${() => onCreateFromTemplate?.(selectedTemplate)}
                          >
                            <span>${t("createAgent")}</span>
                          </button>
                        </div>
                      </div>
                    </div>

                    <div className="hub-inspector-grid">
                      <div className="hub-inspector-field">
                        <span>${t("hubSourceLabel")}</span>
                        <strong>${localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}</strong>
                      </div>
                      <div className="hub-inspector-field">
                        <span>${t("hubRuntimeLabel")}</span>
                        <strong>${selectedTemplate.runtime_kind || "-"}</strong>
                      </div>
                      <div className="hub-inspector-field">
                        <span>${t("hubImageLabel")}</span>
                        <strong className="hub-field-value-multiline">${selectedTemplate.image || "-"}</strong>
                      </div>
                      <div className="hub-inspector-field">
                        <span>${t("hubUpdatedAtLabel")}</span>
                        <strong>${formatHubDateTime(selectedTemplate.updated_at, locale)}</strong>
                      </div>
                    </div>

                    <div className="hub-description-block">
                      <span className="hub-section-label">${t("hubDescriptionLabel")}</span>
                      <p>${selectedTemplate.description || selectedTemplate.id}</p>
                    </div>

                    <div className="hub-workspace-block">
                      <span className="hub-section-label">${t("hubWorkspaceTemplateLabel")}</span>
                      <div className="hub-workspace-panels">
                        <div className="hub-workspace-tree">
                          ${detailLoading
                            ? html`<div className="workspace-empty">${t("hubWorkspaceLoading")}</div>`
                            : workspaceEntries.length === 0
                              ? html`<div className="workspace-empty">${t("hubWorkspacePreviewHint")}</div>`
                              : visibleWorkspaceEntries.map((entry) => {
                                  const collapsed = entry.type === "dir" && Boolean(collapsedWorkspaceDirs[entry.path]);
                                  return html`
                                    <button
                                      key=${entry.path}
                                      type="button"
                                      className=${`hub-tree-row ${entry.type} ${entry.type === "dir" ? "toggleable" : ""} ${entry.type === "file" && selectedWorkspacePath === entry.path ? "active" : ""}`.trim()}
                                      style=${{ "--hub-tree-depth": entry.depth }}
                                      onClick=${() => entry.type === "dir"
                                        ? toggleWorkspaceDir(entry.path)
                                        : onSelectWorkspaceFile?.(entry.path)}
                                      aria-expanded=${entry.type === "dir" ? String(!collapsed) : undefined}
                                    >
                                      <span className=${`hub-tree-toggle ${entry.type === "file" ? "spacer" : ""} ${collapsed ? "collapsed" : ""}`.trim()} aria-hidden="true"></span>
                                      <span className="hub-tree-glyph" aria-hidden="true">
                                        ${entry.type === "dir" ? html`<${WorkspaceDirIcon} />` : html`<${WorkspaceFileIcon} />`}
                                      </span>
                                      <span className="hub-tree-label">${entry.name}</span>
                                    </button>
                                  `;
                                })}
                        </div>
                        <div className="hub-workspace-preview">
                          ${workspaceFileError
                            ? html`<div className="workspace-empty">${workspaceFileError}</div>`
                            : workspaceFileLoading
                              ? html`<div className="workspace-empty">${t("hubWorkspaceFileLoading")}</div>`
                              : !workspaceFile
                                ? html`
                                    <div className="hub-preview-empty-state">
                                      <${HubPreviewEmptyIcon} />
                                      <strong>${t("hubWorkspacePreviewTitle")}</strong>
                                      <p>${t("hubWorkspacePreviewHint")}</p>
                                    </div>
                                  `
                                : html`
                                    <div className="hub-preview-file-header">
                                      <strong>${workspaceFile.path}</strong>
                                      <span>${workspaceFile.binary ? t("hubWorkspaceBinary") : `${workspaceFile.size || 0} B`}</span>
                                    </div>
                                    <div className="hub-preview-body">
                                      ${workspaceFile.binary
                                        ? html`<div className="workspace-empty">${t("hubWorkspaceBinary")}</div>`
                                        : html`
                                            <div className="hub-preview-code-shell">
                                              <pre className="hub-preview-line-numbers">${Array.from({ length: workspacePreviewLineCount }, (_, index) => index + 1).join("\n")}</pre>
                                              <pre className="hub-preview-code">${workspacePreviewText}</pre>
                                            </div>
                                          `}
                                    </div>
                                  `}
                        </div>
                      </div>
                    </div>
                  ` : null}
                </div>
              </div>
            `}
    </section>
  `;
}

function AgentDetailPane({ item, t, activeRoom, busyKey, error, draft, models, modelBusy, saving, publishBusy, saveError, authStatuses, authBusyProvider, onDraftChange, onSave, onPublish, onProviderLogin, onStart, onStop, onRecreate, onDelete, onInvite, onOpenDM }) {
  const isManager = item.role === "manager" || item.id === "u-manager";
  const running = isAgentRunning(item);
  const incomplete = isAgentIncomplete(item);
  const restartNeeded = isAgentRestartNeeded(item);
  const busyPrefix = `${item.id}:`;
  const provider = item.provider || item.agent_profile?.provider;
  const runtimeKind = normalizeRuntimeKind(item.runtime_kind);
  const canPublish = runtimeKind === "picoclaw_sandbox" || runtimeKind === "openclaw_sandbox";
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
        <button
          className="btn btn-primary btn-sm preview-action-button preview-action-button-primary"
          disabled=${saving || isBlank(draft?.name) || !draft?.model_id || profileBaseURLMissing(draft)}
          onClick=${onSave}
        >
          ${saving ? t("profileLoadingModels") : t("agentUpdateSave")}
        </button>
        ${activeRoom && !isManager
          ? html`<button className="btn btn-secondary-gray btn-sm preview-action-button" disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onInvite(item)}>${t("inviteToRoom")}</button>`
          : null}
        <button className="btn btn-secondary-gray btn-sm preview-action-button" onClick=${() => onOpenDM(item)}>${t("openDM")}</button>
        ${isManager
          ? html`<button className="btn btn-outline-danger btn-sm preview-action-button preview-action-button-danger" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>`
          : null}
        ${!isManager
          ? html`<button className="btn btn-outline-danger btn-sm preview-action-button preview-action-button-danger" disabled=${busyKey.startsWith(busyPrefix)} onClick=${() => onDelete(item)}>${t("agentDelete")}</button>`
          : null}
        ${canPublish
          ? html`
              <button
                className="btn btn-primary btn-sm preview-action-button preview-action-button-primary entity-toolbar-publish"
                disabled=${publishBusy}
                onClick=${onPublish}
              >
                ${publishBusy ? t("agentPublishing") : t("agentPublish")}
              </button>
            `
          : null}
      </div>
      ${error ? html`<div className="form-error">${error}</div>` : null}
      ${saveError ? html`<div className="form-error">${saveError}</div>` : null}
      ${!draft
        ? html`
            <div className="entity-grid">
              <div className="entity-field">
                <span>${t("profileRuntimeKind")}</span>
                <strong>${formatRuntimeKindLabel(item.runtime_kind, t)}</strong>
              </div>
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
                    ${requiredFieldLabel(t("agentName"))}
                    <input
                      value=${draft.name}
                      required
                      aria-required="true"
                      onInput=${(event) => updateDraft({ name: event.target.value })}
                      placeholder=${t("agentNamePlaceholder")}
                    />
                  </label>
                  <label className="field">
                    <span>${t("profileRuntimeKind")}</span>
                    <input value=${draft.runtime_kind || item.runtime_kind || ""} readOnly disabled />
                  </label>
                  <label className="field">
                    <span>${t("agentImage")}</span>
                    <input value=${draft.image} readOnly disabled placeholder=${t("agentImagePlaceholder")} />
                  </label>
                  <label className="field span-2">
                    <span>${t("agentDescription")}</span>
                    <textarea className="compact-textarea" value=${draft.description} onInput=${(event) => updateDraft({ description: event.target.value })} />
                  </label>
                </div>
              </section>

              <section className="profile-section">
                <div className="profile-section-title">${t("profileModelSection")}</div>
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
                    ${requiredFieldLabel(t("profileModel"))}
                    <select
                      value=${draft.model_id}
                      required
                      aria-required="true"
                      onChange=${(event) => updateDraft({ model_id: event.target.value })}
                    >
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
                <${CLIProxyAuthControl}
                  provider=${draft.provider}
                  t=${t}
                  status=${authStatuses?.[normalizeAuthProviderName(draft.provider)]}
                  busy=${authBusyProvider === normalizeAuthProviderName(draft.provider)}
                  onLogin=${onProviderLogin}
                />
              </section>

              ${draft.provider === "api"
                ? html`
                    <section className="profile-section">
                      <div className="profile-section-title">${t("profileAPIProvider")}</div>
                      <div className="profile-api-grid">
                        <label className="field">
                          ${requiredFieldLabel(t("profileBaseURL"))}
                          <input
                            value=${draft.base_url}
                            required
                            aria-required="true"
                            onInput=${(event) => updateDraft({ base_url: event.target.value })}
                            placeholder="https://api.openai.com/v1"
                          />
                        </label>
                        <${APIKeyField}
                          value=${draft.api_key}
                          onInput=${(event) => updateDraft({ api_key: event.target.value })}
                          profile=${draft}
                          t=${t}
                        />
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

function profilePreviewStyle(anchorRect, cardHeight = 420) {
  const offset = 12;
  const viewportPadding = 12;
  const width = Math.min(360, window.innerWidth - 24);
  const preferRight = anchorRect ? anchorRect.right + offset + width <= window.innerWidth - viewportPadding : true;
  const left = anchorRect
    ? preferRight
      ? Math.max(viewportPadding, anchorRect.right + offset)
      : Math.max(viewportPadding, anchorRect.left - width - offset)
    : viewportPadding;
  const maxTop = Math.max(viewportPadding, window.innerHeight - viewportPadding - Math.min(cardHeight, window.innerHeight - viewportPadding * 2));
  const top = anchorRect
    ? Math.min(Math.max(viewportPadding, anchorRect.top - 12), maxTop)
    : viewportPadding;
  return { top: `${top}px`, left: `${left}px`, width: `${width}px` };
}

function ProfilePreviewPopover({ previewRef, agent, user, anchorRect, t, inDirectConversation, busyKey, onClose, onOpenAgent, onOpenDM, onDelete }) {
  const running = agent ? isAgentRunning(agent) : false;
  const incomplete = agent ? isAgentIncomplete(agent) : false;
  const restartNeeded = agent ? isAgentRestartNeeded(agent) : false;
  const provider = agent?.provider || agent?.agent_profile?.provider;
  const displayName = agent?.name || user?.name || "";
  const displayRole = agent ? (agent.role || "worker") : user?.role;
  const deleteBusy = agent ? busyKey === `${agent.id}:delete-bot` : false;
  const canOpenDM = !inDirectConversation;
  const [cardHeight, setCardHeight] = useState(420);

  useLayoutEffect(() => {
    const preview = previewRef?.current;
    if (!preview) {
      return;
    }
    const nextHeight = Math.ceil(preview.getBoundingClientRect().height);
    if (nextHeight > 0 && nextHeight !== cardHeight) {
      setCardHeight(nextHeight);
    }
  }, [previewRef, cardHeight, agent?.id, user?.id, inDirectConversation]);

  return html`
    <aside
      ref=${previewRef}
      className="profile-preview-popover"
      style=${profilePreviewStyle(anchorRect, cardHeight)}
      aria-label=${t("profilePreview")}
    >
      <div className="preview-header">
        <div className="preview-title">${agent ? t("profilePreview") : t("personProfile")}</div>
        <button className="btn btn-secondary-gray btn-sm modal-close" aria-label=${t("close")} onClick=${onClose}>
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
              <button className="btn btn-primary btn-sm preview-action-button preview-action-button-primary" onClick=${() => onOpenAgent(agent)}>${t("openProfile")}</button>
              ${canOpenDM
                ? html`<button className="btn btn-secondary-gray btn-sm preview-action-button" onClick=${() => onOpenDM(agent)}>${t("openDM")}</button>`
                : null}
              ${agent.role !== "manager" && agent.id !== "u-manager"
                ? html`<button className="btn btn-outline-danger btn-sm preview-action-button preview-action-button-danger preview-actions-delete" disabled=${deleteBusy} onClick=${() => onDelete(agent)}>${t("agentDelete")}</button>`
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
        <button className="btn btn-primary btn-sm send-button compact" onClick=${onCreateAgent}>${t("createAgent")}</button>
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
                  className="btn btn-secondary-gray btn-sm agent-icon-button"
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
          <button type="button" className="btn btn-ghost btn-sm env-remove-button" aria-label=${t("profileEnvRemove")} title=${t("profileEnvRemove")} onClick=${() => remove(index)}>
            ×
          </button>
        </div>
      `)}
      <button type="button" className="btn btn-secondary-gray btn-sm secondary-button env-add-button" onClick=${() => onChange([...items, { key: "", value: "" }])}>
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

function localizeTemplateSourceTag(source, locale) {
  const value = String(source ?? "").trim();
  if (!value) {
    return "-";
  }
  if (locale === "zh") {
    if (value === "builtin") {
      return "内建";
    }
    if (value === "local") {
      return "本地";
    }
  }
  return value;
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

function formatHubDate(value, locale) {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    timeZone: "UTC",
  }).format(new Date(value));
}

function formatHubDateTime(value, locale) {
  if (!value) {
    return "-";
  }
  return `${new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    timeZone: "UTC",
  }).format(new Date(value))} (UTC)`;
}

function formatHubTemplateCount(count, locale, t) {
  if (locale === "zh") {
    return `共 ${count} ${t("hubTemplateCountSuffix")}`;
  }
  return `${count} ${t("hubTemplateCountSuffix")}`;
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
    runtime_kind: normalizeRuntimeKind(profile?.runtime_kind),
    provider: profile?.provider || "csghub_lite",
    base_url: profile?.base_url || "",
    api_key: "",
    api_key_set: Boolean(profile?.api_key_set),
    api_key_preview: profile?.api_key_preview || "",
    model_id: profile?.model_id || "",
    reasoning_effort: profile?.reasoning_effort || "medium",
    enable_fast_mode: Boolean(profile?.enable_fast_mode),
    headersText: stringifyJSON(profile?.headers || {}),
    requestOptionsText: stringifyJSON(profile?.request_options || {}),
    envRows: mapToEnvRows(profile?.env || {}),
  };
}

function modelRequestKey(draft) {
  if (!draft) {
    return "";
  }
  return JSON.stringify({
    agent_id: draft.agent_id || "",
    provider: draft.provider || "",
    base_url: draft.base_url || "",
    api_key: draft.api_key || "",
    headersText: draft.headersText || "",
  });
}

function agentToDraft(agent) {
  const profile = agent?.agent_profile || agent || {};
  return {
    agent_id: agent?.id || "",
    name: agent?.name || "",
    role: agent?.role || "worker",
    description: agent?.description || profile.description || "",
    default_image: agent?.image || "",
    image: agent?.image || "",
    from_template: agent?.from_template || "",
    template_name: agent?.template_name || "",
    ...profileToDraft(profile),
    runtime_kind: normalizeRuntimeKind(agent?.runtime_kind || profile.runtime_kind),
  };
}

function normalizeTemplateSelection(template) {
  return template && typeof template === "object" ? template : null;
}

function templateMatchesRuntime(template, runtimeKind) {
  const requestedRuntime = normalizeRuntimeKind(runtimeKind);
  if (!template || !requestedRuntime) {
    return true;
  }
  const templateRuntime = normalizeRuntimeKind(template.runtime_kind);
  return !templateRuntime || templateRuntime === requestedRuntime;
}

function pickDefaultAgentTemplate(templates, runtimeKind = "", bootstrapConfig = null) {
  if (!Array.isArray(templates) || templates.length === 0) {
    return null;
  }
  const requestedRuntime = normalizeRuntimeKind(runtimeKind || bootstrapConfig?.runtime_kind);
  const candidates = requestedRuntime
    ? templates.filter((item) => templateMatchesRuntime(item, requestedRuntime))
    : templates.slice();
  if (!candidates.length) {
    return null;
  }
  const configuredDefault = String(bootstrapConfig?.default_worker_template || "").trim();
  if (configuredDefault) {
    const configured = candidates.find((item) => item.id === configuredDefault);
    if (configured) {
      return configured;
    }
  }
  if (requestedRuntime === "openclaw_sandbox") {
    return candidates.find((item) => item.id === "builtin/openclaw-worker")
      || candidates.find((item) => item.name === "openclaw-worker")
      || candidates.find((item) => String(item.id || "").endsWith("/openclaw-worker"))
      || candidates[0];
  }
  if (requestedRuntime === "picoclaw_sandbox" || !requestedRuntime) {
    return candidates.find((item) => item.id === "builtin/picoclaw-worker")
      || candidates.find((item) => item.name === "picoclaw-worker")
      || candidates.find((item) => String(item.id || "").endsWith("/picoclaw-worker"))
      || candidates[0];
  }
  return candidates[0];
}

function applyTemplateToDraft(draft, template, bootstrapConfig, fallbackImage = "") {
  if (!draft) {
    return draft;
  }
  if (!template) {
    return {
      ...draft,
      from_template: "",
      template_name: "",
    };
  }
  const runtimeKind = normalizeRuntimeKind(template.runtime_kind || draft.runtime_kind || bootstrapConfig?.runtime_kind);
  return {
    ...draft,
    from_template: template.id || "",
    template_name: template.name || template.id || "",
    runtime_kind: runtimeKind,
    image: template.image || runtimeImageForKind(runtimeKind, bootstrapConfig, fallbackImage || draft.default_image || ""),
    description: template.description || draft.description || "",
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

function normalizeAuthProviderName(provider) {
  const value = String(provider ?? "").trim().toLowerCase();
  if (value === "claude" || value === "claude-code") {
    return "claude_code";
  }
  return value;
}

function providerNeedsAuth(provider) {
  return CLIPROXY_AUTH_PROVIDERS.has(normalizeAuthProviderName(provider));
}

function CLIProxyAuthControl({ provider, t, status, busy, onLogin }) {
  const normalized = normalizeAuthProviderName(provider);
  if (!providerNeedsAuth(normalized)) {
    return null;
  }
  const connected = Boolean(status?.authenticated);
  const message = connected
    ? `${formatProviderLabel(normalized)} ${t("authConnected")}`
    : (status?.message || `${formatProviderLabel(normalized)} ${t("authMissing")}`);
  return html`
    <div className=${`auth-status-row ${connected ? "connected" : "missing"}`}>
      <span className="auth-status-dot" aria-hidden="true"></span>
      <span className="auth-status-message">${message}</span>
      ${connected
        ? null
        : html`
            <button type="button" className="btn btn-secondary-gray btn-sm secondary-button compact" disabled=${busy || !onLogin} onClick=${() => onLogin?.(normalized)}>
              ${busy ? t("authConnecting") : `${t("authConnect")} ${formatProviderLabel(normalized)}`}
            </button>
          `}
    </div>
  `;
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

function AgentCreateProgress({ progress, t }) {
  if (!progress) {
    return null;
  }
  const steps = progress.steps || [];
  const currentStep = steps[Math.min(progress.index || 0, Math.max(steps.length - 1, 0))];
  const failed = progress.status === "failed";
  const done = progress.status === "done";
  const label = failed
    ? t("agentCreateProgressFailed")
    : done
      ? t("agentCreateProgressDone")
      : t(currentStep?.label || "agentCreateProgressPreparing");
  const percent = Math.max(0, Math.min(100, Math.round(progress.percent || 0)));
  return html`
    <div className=${`agent-create-progress ${failed ? "failed" : ""} ${done ? "done" : ""}`.trim()} role="status" aria-live="polite">
      <div className="agent-create-progress-header">
        <span>${label}</span>
        <strong>${percent}%</strong>
      </div>
      <div className="agent-create-progress-track" aria-hidden="true">
        <div className="agent-create-progress-fill" style=${{ width: `${percent}%` }} />
      </div>
      <div className="agent-create-progress-steps">
        ${steps.map((step, index) => html`
          <span key=${`${step.label}-${index}`} className=${index < progress.index || done ? "complete" : index === progress.index && !failed ? "active" : ""}>
            ${t(step.label)}
          </span>
        `)}
      </div>
    </div>
  `;
}

function normalizeRuntimeKind(kind) {
  const value = String(kind ?? "").trim().toLowerCase();
  switch (value) {
    case "openclaw_sandbox":
      return "openclaw_sandbox";
    case "codex":
      return "codex";
    case "picoclaw_sandbox":
    default:
      return "picoclaw_sandbox";
  }
}

function normalizeRuntimeImageMap(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const out = {};
  for (const [key, image] of Object.entries(value)) {
    const runtimeKind = normalizeRuntimeKind(key);
    const trimmed = String(image ?? "").trim();
    if (runtimeKind && trimmed) {
      out[runtimeKind] = trimmed;
    }
  }
  return out;
}

function runtimeImageForKind(kind, bootstrapConfig, fallbackImage = "") {
  const runtimeKind = normalizeRuntimeKind(kind);
  if (runtimeKind === "codex") {
    return "";
  }
  const images = normalizeRuntimeImageMap(bootstrapConfig?.runtime_default_images);
  if (images[runtimeKind]) {
    return images[runtimeKind];
  }
  if (normalizeRuntimeKind(bootstrapConfig?.runtime_kind) === runtimeKind && bootstrapConfig?.effective_manager_image) {
    return String(bootstrapConfig.effective_manager_image).trim();
  }
  return String(fallbackImage ?? "").trim();
}

function availableManagerRuntimeOptions(bootstrapConfig) {
  const configuredKinds = Array.isArray(bootstrapConfig?.supported_runtime_kinds)
    ? bootstrapConfig.supported_runtime_kinds
    : [];
  const gatewayKinds = (configuredKinds.length ? configuredKinds : ["picoclaw_sandbox", "openclaw_sandbox"])
    .map((kind) => normalizeRuntimeKind(kind))
    .filter((kind, index, array) => kind && kind !== "codex" && array.indexOf(kind) === index);
  return RUNTIME_KIND_OPTIONS.filter((option) => gatewayKinds.includes(option.value));
}

function agentCreateProgressSteps(runtimeKind) {
  const kind = normalizeRuntimeKind(runtimeKind);
  if (kind === "openclaw_sandbox" || kind === "picoclaw_sandbox") {
    return [
      { label: "agentCreateProgressSandboxConfig", target: 16 },
      { label: "agentCreateProgressImage", target: 42 },
      { label: "agentCreateProgressRuntime", target: 72 },
      { label: "agentCreateProgressStart", target: 88 },
      { label: "agentCreateProgressFinishing", target: 96 },
    ];
  }
  return [
    { label: "agentCreateProgressPreparing", target: 24 },
    { label: "agentCreateProgressRuntime", target: 66 },
    { label: "agentCreateProgressStart", target: 88 },
    { label: "agentCreateProgressFinishing", target: 96 },
  ];
}

function startAgentCreateProgress(runtimeKind) {
  const steps = agentCreateProgressSteps(runtimeKind);
  return {
    steps,
    index: 0,
    percent: 4,
    status: "running",
    startedAt: Date.now(),
  };
}

function advanceAgentProgress(current) {
  if (!current || current.status !== "running" || !current.steps?.length) {
    return current;
  }
  const step = current.steps[Math.min(current.index, current.steps.length - 1)];
  const target = step?.target ?? 96;
  if (current.percent < target) {
    const delta = Math.max(1, Math.ceil((target - current.percent) / 3));
    return { ...current, percent: Math.min(target, current.percent + delta) };
  }
  if (current.index < current.steps.length - 1) {
    return { ...current, index: current.index + 1 };
  }
  return { ...current, percent: Math.min(96, current.percent) };
}

function formatRuntimeKindLabel(kind, t) {
  switch (normalizeRuntimeKind(kind)) {
    case "openclaw_sandbox":
      return t("runtimeOpenclaw");
    case "codex":
      return "Codex";
    case "picoclaw_sandbox":
    default:
      return t("runtimePicoclaw");
  }
}

function toggleSelection(current, id) {
  return current.includes(id) ? current.filter((item) => item !== id) : [...current, id];
}

function renderMarkdown(content) {
  const raw = marked.parse(decorateMentionMarkup(content));
  const sanitized = DOMPurify.sanitize(raw, {
    USE_PROFILES: { html: true },
    ADD_ATTR: ["target", "rel", "class", "data-user-id"],
  });
  const template = document.createElement("template");
  template.innerHTML = sanitized;
  template.content.querySelectorAll("a[href]").forEach((link) => {
    link.setAttribute("target", "_blank");
    link.setAttribute("rel", "noopener noreferrer");
  });
  return template.innerHTML;
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

function appendComposerSegments(parent, segments) {
  if (!parent) {
    return;
  }
  for (const segment of segments ?? []) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      parent.append(createMentionTokenElement({
        id: segment.userId,
        name: segment.userName,
        handle: segment.userName,
      }));
      continue;
    }
    const parts = String(segment.text ?? "").split("\n");
    parts.forEach((part, index) => {
      if (part) {
        parent.append(document.createTextNode(part));
      }
      if (index < parts.length - 1) {
        parent.append(document.createElement("br"));
      }
    });
  }
}

function renderComposerSegments(root, segments) {
  if (!root) {
    return;
  }
  root.replaceChildren();
  appendComposerSegments(root, segments);
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

function splitTextSegmentByMentions(text, mentionableUsersByHandle) {
  const content = String(text ?? "");
  if (!content || !mentionableUsersByHandle || mentionableUsersByHandle.size === 0) {
    return content ? [{ type: "text", text: content }] : [];
  }
  const mentionPattern = /(^|[^\w])@([a-zA-Z0-9._-]+)/g;
  const segments = [];
  let lastIndex = 0;
  let match;
  while ((match = mentionPattern.exec(content)) !== null) {
    const prefix = match[1] ?? "";
    const handle = match[2] ?? "";
    const user = mentionableUsersByHandle.get(handle.toLowerCase());
    if (!user) {
      continue;
    }
    const mentionStart = match.index + prefix.length;
    if (mentionStart > lastIndex) {
      segments.push({ type: "text", text: content.slice(lastIndex, mentionStart) });
    }
    segments.push({
      type: "mention",
      userId: user.id,
      userName: user.name || user.handle || user.id,
    });
    lastIndex = mentionStart + handle.length + 1;
  }
  if (lastIndex < content.length) {
    segments.push({ type: "text", text: content.slice(lastIndex) });
  }
  return segments;
}

function normalizeTextMentions(segments, mentionableUsersByHandle) {
  const normalized = [];
  for (const segment of segments ?? []) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      normalized.push(segment);
      continue;
    }
    normalized.push(...splitTextSegmentByMentions(segment.text ?? "", mentionableUsersByHandle));
  }
  return normalizeComposerSegments(normalized);
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

function insertComposerSegmentsAtSelection(segments) {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  range.deleteContents();
  const marker = document.createTextNode("");
  const fragment = document.createDocumentFragment();
  appendComposerSegments(fragment, segments);
  fragment.append(marker);
  range.insertNode(fragment);
  const nextRange = document.createRange();
  nextRange.setStart(marker, 0);
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
  if (parsed && isActionCardPayload(parsed)) {
    return buildActionCardPayload(parsed);
  }
  if (parsed && isStructuredPayload(parsed)) {
    return buildStructuredPayload(parsed);
  }

  const extracted = extractTopLevelJSONObject(cleaned);
  const extractedParsed = tryParseJSON(extracted);
  if (extractedParsed && isActionCardPayload(extractedParsed)) {
    return buildActionCardPayload(extractedParsed);
  }
  if (extractedParsed && isStructuredPayload(extractedParsed)) {
    return buildStructuredPayload(extractedParsed);
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

function extractTopLevelJSONObject(input) {
  if (!input) {
    return null;
  }
  const firstBrace = input.indexOf("{");
  if (firstBrace < 0) {
    return null;
  }

  let depth = 0;
  let inString = false;
  let escaped = false;
  for (let index = firstBrace; index < input.length; index += 1) {
    const char = input[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (inString) {
      if (char === "\\") {
        escaped = true;
        continue;
      }
      if (char === '"') {
        inString = false;
      }
      continue;
    }
    if (char === '"') {
      inString = true;
      continue;
    }
    if (char === "{") {
      depth += 1;
      continue;
    }
    if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        return input.slice(firstBrace, index + 1);
      }
    }
  }

  return null;
}

function isActionCardPayload(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  return value.type === CSGCLAW_ACTION_CARD_TYPE && Array.isArray(value.actions);
}

function buildActionCardPayload(value) {
  return {
    kind: "action_card",
    title: firstNonEmptyString(value.title, value.name, "Action required"),
    subtitle: firstNonEmptyString(value.subtitle, value.bot_id),
    badge: firstNonEmptyString(value.badge, value.status),
    summary: firstNonEmptyString(value.summary, value.message, value.description),
    fallback: firstNonEmptyString(value.fallback),
    actions: normalizeActionCardActions(value.actions),
  };
}

function normalizeActionCardActions(actions) {
  return (actions ?? [])
    .filter((action) => action && action.id === ACTION_REBUILD_MANAGER)
    .slice(0, 1)
    .map((action) => ({
      id: ACTION_REBUILD_MANAGER,
      label: firstNonEmptyString(action.label, "重建 Manager"),
      style: action.style === "danger" ? "danger" : "default",
      confirm: firstNonEmptyString(action.confirm),
    }));
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
          <button className="btn btn-secondary-gray btn-sm secondary-button" onClick=${() => window.location.reload()}>Reload</button>
        </div>
      `;
    }
    return this.props.children;
  }
}

createRoot(document.getElementById("root")).render(html`<${AppErrorBoundary}><${App} /><//>`);
