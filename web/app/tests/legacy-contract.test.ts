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
    expect(source).toContain("payload.runtime_kind = options.runtime_kind;");
    expect(source).toContain("payload.image = options.image;");
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
    expect(source).toContain("pickDefaultAgentTemplate(hubTemplates, preferredRuntimeKind, bootstrapConfig)");
    expect(source).toContain('applyTemplateToDraft(current, nextTemplate, bootstrapConfig, managerAgent?.image || "")');
    expect(source).toContain("function templateMatchesRuntime(template, runtimeKind)");
    expect(source).toContain("pickDefaultAgentTemplate(hubTemplates, runtimeKind, bootstrapConfig)");
  });

  it("keeps channel-scoped bot and notification bot frontend contracts", () => {
    expect(source).toContain('get<AgentLike[]>("api/v1/channels/csgclaw/bots")');
    expect(source).toContain('post("api/v1/channels/csgclaw/bots", { ...payload, type: BOT_TYPE_NOTIFICATION })');
    expect(source).toContain("patchNotificationBotRequest");
    expect(source).toContain("createNotificationBotRequest");
    expect(source).toContain("del(`api/v1/channels/csgclaw/bots/${encodeURIComponent(botID)}`)");
    expect(source).toContain("const SHOW_AGENT_LIFECYCLE_ACTIONS = false;");
    expect(source).toContain('export const BOT_TYPE_NOTIFICATION = "notification"');
    expect(source).toContain("function NotifierControls");
    expect(source).toContain("function draftNotifierRuntimeOptionsForSave");
    expect(source).toContain("notifier_remote_subscription_id");
    expect(source).toContain("/api/v1/channels/csgclaw/bots/${encodeURIComponent(id)}/notifications");
  });

  it("keeps the agent publish action contract", () => {
    expect(source).toContain('agentPublish: "Publish"');
    expect(source).toContain('agentPublish: "发布"');
    expect(source).toContain("async function publishAgentPage()");
    expect(source).toContain("function publishAgentTemplateRequest");
    expect(source).toContain('const HUB_TEMPLATES_PATH = "/api/v1/hub/templates";');
    expect(source).toContain("post<HubTemplate>(HUB_TEMPLATES_PATH, payload)");
    expect(source).toContain("publishAgentTemplateRequest(selectedAgentForPage.id)");
    expect(source).toContain("agent_id: agentID");
    expect(source).toContain("setSelectedHubTemplateId(published.id);");
    expect(source).toContain('className="entity-toolbar-publish"');
    expect(source).toContain(
      'const canPublish = runtimeKind === "picoclaw_sandbox" || runtimeKind === "openclaw_sandbox";',
    );
  });

  it("keeps thread context hidden and shows the thread affordance as a message hover toolbar", () => {
    expect(source).toContain("message-hover-actions");
    expect(source).toContain("thread-hover-button");
    expect(source).toContain("thread-action-tooltip");
    expect(source).toContain('threads: "threads"');
    expect(source).toContain("WorkspaceThreadRow");
    expect(source).toContain('threadsTab: "Threads"');
    expect(source).toContain('noThreads: "No threads yet."');
    expect(source).not.toContain('className="thread-strip"');
    expect(source).not.toContain('className="thread-context"');
    expect(source).not.toContain('t("threadContext")');
    expect(styles).toContain(".message-row:hover .message-hover-actions");
    expect(styles).toContain(".thread-hover-button:hover .thread-action-tooltip");
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
    expect(styles).toMatch(/\.messages\s*\{[\s\S]*scrollbar-width:\s*thin;/);
    expect(styles).toContain(".messages::-webkit-scrollbar");
    expect(styles).toContain("width: 6px;");
  });

  it("keeps the mention picker scrollbar slim", () => {
    expect(styles).toMatch(/\.mention-picker\s*\{[\s\S]*scrollbar-width:\s*thin;/);
    expect(styles).toContain(".mention-picker::-webkit-scrollbar");
    expect(styles).toContain(".mention-picker::-webkit-scrollbar-thumb");
  });

  it("shows agent running status dots in direct message rows", () => {
    expect(source).toContain(
      "const directAgent = isDirect && displayUser ? agents.find((item) => agentMatchesUser(item, displayUser)) : null;",
    );
    expect(source).toContain('className={`workspace-status-dot ${directAgentRunning ? "online" : ""}`}');
    expect(source).toMatch(/directMessages\.map[\s\S]*agents=\{agentItems\}/);
  });
});
