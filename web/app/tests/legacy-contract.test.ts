import { readdirSync, readFileSync, statSync } from "node:fs";
import { resolve } from "node:path";

function readTree(dir: string, pattern: RegExp): string {
  return readdirSync(dir)
    .flatMap((entry) => {
      const path = resolve(dir, entry);
      if (statSync(path).isDirectory()) {
        return readTree(path, pattern);
      }
      return pattern.test(entry) ? readFileSync(path, "utf8") : "";
    })
    .join("\n");
}

const source: string = readTree(resolve(process.cwd(), "src"), /\.(js|jsx|ts|tsx)$/);
const styles: string = readTree(resolve(process.cwd(), "src"), /\.css$/);

function styleRule(selector: string): string {
  const pattern = new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\s*\\{([^}]*)\\}`);
  return styles.match(pattern)?.[1] ?? "";
}

describe("legacy UI contract", () => {
  it("keeps the manager rebuild action card contract", () => {
    expect(source).toContain('const CSGCLAW_ACTION_CARD_TYPE = "csgclaw.action_card";');
    expect(source).toContain('const CSGCLAW_NOTIFY_CARD_TYPE = "csgclaw.notify_card";');
    expect(source).toContain("function ActionCard");
    expect(source).toContain("function isNotifyCardPayload");
    expect(source).toContain("function rebuildManagerFromBrowser");
    expect(source).toContain("function ManagerRebuildModal");
    expect(source).toContain("function createManagerAgentRequest");
    expect(source).toContain('post("api/v1/agents", payload)');
    expect(source).toContain('id: "u-manager",');
    expect(source).toContain("replace: true,");
    expect(source).toContain(
      "const runtimeSelection = resolveRuntimeSelection({ runtime_kind: options.runtime_kind });",
    );
    expect(source).toContain("payload.runtime = {");
    expect(source).not.toContain("payload.image = options.image;");
    expect(source).toContain("const rebuiltAgent = await createManagerAgentRequest");
    expect(source).toContain("await refreshAgentsWithUpdatedAgent(rebuiltAgent);");
    expect(source).not.toContain('request("api/v1/agents/u-manager/recreate"');
    expect(source).not.toContain("saved.profile_complete");
    expect(source).not.toContain("if (saved.profile_complete)");
    expect(source).toContain('link.setAttribute("target", "_blank");');
    expect(source).toContain('link.setAttribute("rel", "noopener noreferrer");');
  });

  it("keeps hub template creation behavior", () => {
    expect(source).toContain("onCreateFromTemplate: agent.openCreateAgentModal");
    expect(source).toContain('from_template: agentDraft.from_template || ""');
    expect(source).toContain('templateLabel: "模板"');
    expect(source).toContain("onClick={() => onCreateFromTemplate?.(selectedTemplate)}");
    expect(source).toContain("function localizeTemplateSourceTag(source, locale)");
    expect(source).toContain('if (value === "builtin")');
    expect(source).toContain('return "内建";');
    expect(source).toContain('if (value === "local")');
    expect(source).toContain('return "本地";');
    expect(source).toContain('if (value === "official")');
    expect(source).toContain('return "官方";');
    expect(source).toContain("localizeTemplateSourceTag(item.source?.name, locale)");
    expect(source).toContain("localizeTemplateSourceTag(selectedTemplate.source?.name, locale)");
    expect(source).toContain("pickDefaultAgentTemplate(workerTemplates, defaultSandboxRuntimeKind, bootstrapConfig)");
    expect(source).toContain('applyTemplateToDraft(current, nextTemplate, bootstrapConfig, managerAgent?.image || "")');
    expect(source).toContain("function templateMatchesRuntime(template, runtimeKind)");
    expect(source).toContain("pickDefaultAgentTemplate(workerTemplates, runtimeKind, bootstrapConfig)");
  });

  it("keeps channel-scoped participant and notification frontend contracts", () => {
    expect(source).toContain('get<AgentLike[]>("api/v1/agents?include_participants=true")');
    expect(source).toContain('post<ParticipantLike>("api/v1/channels/csgclaw/participants"');
    expect(source).toContain('"api/v1/channels/csgclaw/participants?type=notification"');
    expect(source).toContain("patchNotificationBotRequest");
    expect(source).toContain("createNotificationBotRequest");
    expect(source).toContain("deleteAgentRequest(item.id)");
    expect(source).toContain("del(`api/v1/agents/${encodeURIComponent(agentID)}`)");
    expect(source).toContain('params.set("delete_agent", "if_unreferenced");');
    expect(source).toContain("const SHOW_AGENT_LIFECYCLE_ACTIONS = false;");
    expect(source).toContain('export const BOT_TYPE_NOTIFICATION = "notification"');
    expect(source).toContain("function NotifierControls");
    expect(source).toContain("function draftNotifierRuntimeOptionsForSave");
    expect(source).toContain("notifier_remote_subscription_id");
    expect(source).toContain("/api/v1/channels/csgclaw/participants/${encodeURIComponent(id)}/notifications");
  });

  it("keeps the agent publish action contract", () => {
    expect(source).toContain('agentPublish: "Publish"');
    expect(source).toContain('agentPublish: "发布"');
    expect(source).toContain("async function publishAgentPage()");
    expect(source).toContain("function publishAgentTemplateRequest");
    expect(source).toContain('const HUB_TEMPLATES_PATH = "/api/v1/hub/templates";');
    expect(source).toContain("post<HubTemplate>(HUB_TEMPLATES_PATH, payload)");
    expect(source).toContain("publishAgentTemplateRequest(selectedAgentForPage.id)");
    expect(source).toContain('get("api/v1/agents/image-candidates")');
    expect(source).toContain("agent_id: agentID");
    expect(source).toContain("setSelectedHubTemplateId(published.id);");
    expect(source).toContain('className="agent-actions-menu"');
    expect(source).toContain("onSelect={() => onPublish?.()}");
    expect(source).toContain(
      'const canPublish = runtimeKind === "picoclaw_sandbox" || runtimeKind === "openclaw_sandbox";',
    );
  });

  it("keeps thread context hidden and shows the thread affordance as a message hover toolbar", () => {
    expect(source).toContain("message-hover-actions");
    expect(source).toContain("thread-hover-button");
    expect(source).toContain('data-tooltip={t("replyInThread")}');
    expect(source).toContain('threads: "threads"');
    expect(source).toContain("WorkspaceThreadRow");
    expect(source).toContain('threadsTab: "Threads"');
    expect(source).toContain('noThreads: "No threads yet."');
    expect(source).not.toContain('className="thread-strip"');
    expect(source).not.toContain('className="thread-context"');
    expect(source).not.toContain('t("threadContext")');
    expect(styles).toContain(".message-row:hover .message-hover-actions");
    expect(styles).toMatch(/\.message-hover-actions\s*\{[\s\S]*top:\s*calc\(100% \+ 2px\);/);
    expect(styles).toMatch(/\.thread-message-actions\s*\{[\s\S]*top:\s*calc\(100% \+ 2px\);/);
    expect(styles).not.toContain("bottom: -15px");
    expect(styles).toContain("[data-tooltip]:hover::after");
    expect(source).toContain("message-thread-actions has-thread-summary");
    expect(source).toContain("const threadBodyRef = useRef<HTMLDivElement | null>(null);");
    expect(source).toContain("threadBody.scrollTop = threadBody.scrollHeight;");
    expect(source).toContain("const visibleReplies = showToolCalls ? replies : replies.filter");
    expect(source).toContain("[root, visibleReplies.length, latestReplyID, loading]");
    expect(source).toContain("mentionableUsers={conversationMembers}");
    expect(source).toContain("thread-mention-picker");
  });

  it("keeps the message timeline from exposing horizontal scroll", () => {
    expect(styles).toMatch(/\.messages\s*\{[\s\S]*overflow-x:\s*hidden;/);
    expect(styles).toContain(".chat-panel.has-thread-panel > .messages");
    expect(styles).toMatch(/\.chat-panel\.has-thread-panel > \.messages[\s\S]*min-width:\s*0;/);
    expect(styles).toMatch(/\.message-row\s*\{[\s\S]*max-width:\s*min\(100%, 76%, 840px\);/);
  });

  it("uses one transient scrollbar contract across the app", () => {
    expect(styles).toContain("*::-webkit-scrollbar");
    expect(styles).toContain("width: 6px;");
    expect(styles).toContain("height: 6px;");
    expect(styles).toContain('*[data-scrollbar-active="true"]::-webkit-scrollbar-thumb');
    expect(styles).toContain("scrollbar-width: thin;");
    expect(source).toContain('document.addEventListener("scroll", handleScroll, true);');
    expect(source).toContain("const SCROLLBAR_HIDE_DELAY_MS = 700;");
  });

  it("keeps the full exec_command activity label visible", () => {
    const activityListRule = styleRule(".agent-activity-list");
    const activityRowRule = styleRule(".agent-activity-row-main");
    const activityBadgeRule = styleRule(".agent-activity-type-badge");

    expect(activityListRule).toContain("--agent-activity-type-column-width: 128px;");
    expect(activityRowRule).toContain("var(--agent-activity-type-column-width)");
    expect(activityBadgeRule).toContain("width: var(--agent-activity-type-column-width);");
    expect(activityBadgeRule).toContain("max-width: var(--agent-activity-type-column-width);");
    expect(styles).toMatch(
      /:root\[data-theme="dark"\] \.agent-activity-row:hover\s*\{[^}]*background:\s*color-mix\(in srgb, var\(--gray-800\) 72%, var\(--panel\)\);/,
    );
  });

  it("keeps the mention picker layered and compact", () => {
    expect(styles).toMatch(/\.mention-picker\s*\{[\s\S]*z-index:\s*var\(--z-portal-popover\);/);
    expect(styles).toMatch(/\.mention-picker\s*\{[\s\S]*bottom:\s*calc\(100% \+ 8px\);/);
    expect(styles).toMatch(/\.mention-option\s*\{[\s\S]*min-height:\s*56px;/);
  });

  it("keeps profile dialogs from clipping header or actions", () => {
    expect(styles).toMatch(/\.profile-modal\s*\{[\s\S]*display:\s*flex;[\s\S]*overflow:\s*hidden;/);
    expect(styles).toMatch(/\.profile-modal \.modal-header\s*\{[\s\S]*position:\s*relative;/);
    expect(styles).not.toContain("top: -24px;");
    expect(styles).toMatch(/\.profile-editor-shell\s*\{[\s\S]*overflow-y:\s*auto;/);
    expect(styles).toMatch(/\.agent-modal > \.modal-actions\s*\{[\s\S]*flex:\s*0 0 auto;/);
  });

  it("keeps the model provider page scrollable inside the fixed workspace panel", () => {
    const panelRule = styleRule(".chat-panel:has(> .model-provider-page)");
    const pageRule = styleRule(".model-provider-page");

    expect(panelRule).toContain("grid-template-rows: minmax(0, 1fr);");
    expect(pageRule).toContain("height: auto;");
    expect(pageRule).toContain("min-height: 100%;");
    expect(pageRule).toContain("overflow-x: hidden;");
    expect(pageRule).toContain("overflow-y: auto;");
  });

  it("shows agent running status dots in direct message rows", () => {
    expect(source).toContain(
      "const directAgent = isDirect && displayUser ? agents.find((item) => agentMatchesUser(item, displayUser)) : null;",
    );
    expect(source).toContain("className={classNames(styles.statusDot, directAgentRunning && styles.online)}");
    expect(source).toMatch(/visibleDirectMessages\.map[\s\S]*agents=\{agentItems\}/);
    expect(source).toContain("const messageAgent = resolveMessageAgent(agents, user, message.sender_id);");
    expect(source).toContain("return agentMatchesUser(item, { ...user, id: senderIdentity, user_id: user.id });");
    expect(source).toContain('className={`message-avatar-status ${messageAgentRunning ? "online" : ""}`}');
    expect(source).toContain("agents: agent.agentItems,");
    expect(styles).toContain(".message-avatar-status.online");
  });
});
