const fs = require('fs');
const path = require('path');
const assert = require('assert');

const appPath = path.join(__dirname, 'app.js');
const source = fs.readFileSync(appPath, 'utf8');

assert(
  source.includes('const CSGCLAW_ACTION_CARD_TYPE = "csgclaw.action_card";'),
  'frontend must define the CSGClaw action-card payload type',
);
assert(
  source.includes('const CSGCLAW_NOTIFY_CARD_TYPE = "csgclaw.notify_card";'),
  'frontend must define the CSGClaw notifier notify-card payload type',
);
assert(
  source.includes('function isNotifyCardPayload'),
  'frontend must recognize notifier notify-card payloads',
);
assert(
  source.includes('function ActionCard'),
  'frontend must render structured action cards as React components',
);
assert(
  source.includes('function rebuildManagerFromBrowser'),
  'frontend must provide a browser-owned manager rebuild function',
);
assert(
  source.includes('fetch("api/v1/agents", {'),
  'manager rebuild must call the bootstrap create/replace API, not generic recreate',
);
assert(
  source.includes('id: "u-manager",'),
  'manager rebuild request must target u-manager',
);
assert(
  source.includes('replace: true,'),
  'manager rebuild request must use replace=true',
);
assert(
  !source.includes('fetch("api/v1/agents/u-manager/recreate"'),
  'frontend must not call the hazardous generic manager recreate route',
);
assert(
  !source.includes('saved.profile_complete') && !source.includes('if (saved.profile_complete)'),
  'saving the manager profile must not auto-trigger manager rebuild; rebuilds require an explicit window button click',
);
assert(
  source.includes('const payload = {\n        name: agentPageDraft.name,\n        description: agentPageDraft.description,\n        agent_profile: profile,\n      };') &&
    source.includes('body: JSON.stringify(isCreate ? payload : {\n          name: payload.name,\n          description: payload.description,\n          agent_profile: payload.agent_profile,\n        }),'),
  'ordinary agent save flows must keep runtime_kind and image out of PATCH payloads',
);
assert(
  source.includes('link.setAttribute("target", "_blank");'),
  'markdown links must open in a new browser tab',
);
assert(
  source.includes('link.setAttribute("rel", "noopener noreferrer");'),
  'markdown links must use a safe rel attribute when opening a new tab',
);
assert(
  source.includes('hubUseTemplate: "使用此模板"'),
  'hub detail must expose a localized "use this template" action',
);
assert(
  source.includes('onCreateFromTemplate=${openCreateAgentModal}'),
  'hub detail must wire the template action into the existing create-agent modal',
);
assert(
  source.includes('from_template: agentDraft.from_template || ""'),
  'creating an agent from hub detail must pass from_template to the create API',
);
assert(
  source.includes('templateLabel: "模板"'),
  'create-agent modal must expose a template selector label',
);
assert(
  source.includes('function localizeTemplateSourceTag(source, locale)'),
  'frontend must define a locale-aware template source tag formatter',
);
assert(
  source.includes('if (value === "builtin")') &&
    source.includes('return "内建";') &&
    source.includes('if (value === "local")') &&
    source.includes('return "本地";'),
  'frontend must localize builtin/local template source tags in Chinese',
);
assert(
  source.includes('localizeTemplateSourceTag(item.source?.name, locale)') &&
    source.includes('localizeTemplateSourceTag(selectedTemplate.source?.name, locale)'),
  'frontend must use localized template source tags in both hub list and hub detail views',
);
assert(
  source.includes('pickDefaultAgentTemplate(hubTemplates, preferredRuntimeKind, bootstrapConfig)'),
  'normal create-agent flow must preselect a runtime-compatible default worker template',
);
assert(
  source.includes('applyTemplateToDraft(current, nextTemplate, bootstrapConfig, managerAgent?.image || "")'),
  'changing the template selector must update the draft with template defaults',
);
assert(
  source.includes('function templateMatchesRuntime(template, runtimeKind)'),
  'frontend must be able to detect whether a template matches the requested runtime',
);
assert(
  source.includes('pickDefaultAgentTemplate(hubTemplates, runtimeKind, bootstrapConfig)'),
  'changing the runtime must re-pick a compatible default template when needed',
);
