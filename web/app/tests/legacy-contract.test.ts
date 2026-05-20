import { readdirSync, readFileSync, statSync } from "node:fs";
import { resolve } from "node:path";

function readSourceTree(dir: string): string {
  return readdirSync(dir)
    .flatMap((entry) => {
      const path = resolve(dir, entry);
      if (statSync(path).isDirectory()) {
        return readSourceTree(path);
      }
      return /\.(js|jsx|ts|tsx)$/.test(entry) ? readFileSync(path, "utf8") : "";
    })
    .join("\n");
}

const source = readSourceTree(resolve(process.cwd(), "src"));

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

  it("keeps channel-scoped bot and notifier frontend contracts", () => {
    expect(source).toContain('get("api/v1/channels/csgclaw/bots")');
    expect(source).toContain('post("api/v1/channels/csgclaw/bots", payload)');
    expect(source).toContain("del(`api/v1/channels/csgclaw/bots/${encodeURIComponent(botID)}`)");
    expect(source).toContain("const SHOW_AGENT_LIFECYCLE_ACTIONS = false;");
    expect(source).toContain('{ value: "notifier", label: "notifier" }');
    expect(source).toContain("function NotifierControls");
    expect(source).toContain("function draftNotifierRuntimeOptionsForSave");
    expect(source).toContain("notifier_remote_subscription_id");
  });

  it("keeps the agent publish action contract", () => {
    expect(source).toContain('agentPublish: "Publish"');
    expect(source).toContain('agentPublish: "发布"');
    expect(source).toContain("async function publishAgentPage()");
    expect(source).toContain("function publishAgentTemplateRequest");
    expect(source).toContain('post("/api/v1/hub/templates", {');
    expect(source).toContain("publishAgentTemplateRequest(selectedAgentForPage.id)");
    expect(source).toContain("agent_id: agentID");
    expect(source).toContain("setSelectedHubTemplateId(published.id);");
    expect(source).toContain("preview-action-button-primary entity-toolbar-publish");
    expect(source).toContain(
      'const canPublish = runtimeKind === "picoclaw_sandbox" || runtimeKind === "openclaw_sandbox";',
    );
  });
});
