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
  agentDetailPaneSource.includes('isManager') &&
    agentDetailPaneSource.includes('btn btn-outline-danger btn-sm preview-action-button preview-action-button-danger') &&
    agentDetailPaneSource.includes('onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>'),
  'manager profile must expose the red recreate button next to DM even when lifecycle actions are otherwise hidden',
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
