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
