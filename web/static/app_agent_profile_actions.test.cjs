const fs = require('fs');
const path = require('path');
const assert = require('assert');

const appPath = path.join(__dirname, 'app.js');
const source = fs.readFileSync(appPath, 'utf8');
const agentDetailPaneSource = source.slice(
  source.indexOf('function AgentDetailPane('),
  source.indexOf('function ProfilePreviewPopover('),
);

assert(
  source.includes('openDM: "私信"'),
  'Chinese locale must label the DM action as 私信',
);
assert(
  source.includes('const SHOW_AGENT_LIFECYCLE_ACTIONS = false;'),
  'agent lifecycle controls must be hidden by default in the web UI',
);
assert(
  agentDetailPaneSource.includes('SHOW_AGENT_LIFECYCLE_ACTIONS') &&
    agentDetailPaneSource.includes('onClick=${() => onOpenDM(item)}>${t("openDM")}</button>'),
  'agent detail pane must keep DM while gating start/stop/recreate behind SHOW_AGENT_LIFECYCLE_ACTIONS',
);
assert(
  agentDetailPaneSource.includes('btn btn-outline-danger btn-sm preview-action-button preview-action-button-danger') &&
    !agentDetailPaneSource.includes('btn btn-secondary-gray btn-sm preview-action-button" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}') &&
    agentDetailPaneSource.includes('<button className="btn btn-outline-danger btn-sm preview-action-button preview-action-button-danger" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>'),
  'agent detail pane must expose recreate for both manager and worker with the same red danger styling',
);

const agentRowSource = source.slice(
  source.indexOf('function AgentRow('),
  source.indexOf('function WorkspaceAgentsPanel('),
);

assert(
  agentRowSource.includes('SHOW_AGENT_LIFECYCLE_ACTIONS') &&
    agentRowSource.includes('<button className="btn btn-outline-danger btn-sm agent-action-text danger" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>'),
  'agent rows must keep recreate visible with red danger styling even when start/stop controls are hidden',
);
assert(
  source.includes('function openManagerRebuildModal(item = managerAgent)') &&
    source.includes('setShowManagerRebuildModal(true);') &&
    source.includes('setManagerRebuildImage(') &&
    source.includes('image,') &&
    source.includes('runtime_kind: runtimeKind,') &&
    source.includes('function availableManagerRuntimeOptions(bootstrapConfig)') &&
    source.includes('kind !== "notifier"'),
  'manager rebuild flow must open a runtime picker, allow image edits, send runtime_kind plus image to the replace request, and exclude notifier runtime',
);
