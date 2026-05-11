package agent

import "embed"

// workspaceTemplateFS contains the bundled runtime workspace templates.
//
//go:generate ../../scripts/sync-agent-runtimes.sh
//go:embed embed/runtimes/picoclaw/manager/workspace embed/runtimes/picoclaw/worker/workspace
//go:embed embed/runtimes/openclaw/worker/workspace
var workspaceTemplateFS embed.FS
