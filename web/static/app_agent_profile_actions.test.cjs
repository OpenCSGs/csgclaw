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
  agentDetailPaneSource.includes('${activeRoom && !isManager') &&
    agentDetailPaneSource.includes('onClick=${() => onOpenDM(item)}>${t("openDM")}</button>') &&
    agentDetailPaneSource.includes('${isManager') &&
    agentDetailPaneSource.includes('preview-action-button-danger" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>') &&
    agentDetailPaneSource.includes('onClick=${() => (running ? onStop(item) : onStart(item))}') &&
    agentDetailPaneSource.includes('${running ? t("agentStop") : t("agentStart")}') &&
    agentDetailPaneSource.includes(
      'btn btn-secondary-gray btn-sm preview-action-button" disabled=${busyKey.startsWith(busyPrefix) || incomplete} onClick=${() => onRecreate(item)}>${t("agentRecreate")}</button>',
    ),
  'agent detail pane must show start/stop labels, worker recreate (gray), and red manager recreate next to DM',
);
assert(
  source.includes('function openManagerRebuildModal(item = managerAgent)') &&
    source.includes('setShowManagerRebuildModal(true);') &&
    source.includes('setManagerRebuildImage(') &&
    source.includes('image,') &&
    source.includes('runtime_kind: runtimeKind,') &&
    source.includes('function availableManagerRuntimeOptions(bootstrapConfig)'),
  'manager rebuild flow must open a runtime picker, allow image edits, and send runtime_kind plus image to the replace request',
);
