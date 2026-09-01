import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { HubDetailPane } from "@/pages/HubPage/components";
import type { HubTemplate } from "@/models/hubWorkspace";

function t(key: string, params: Record<string, string | number> = {}) {
  const messages: Record<string, string> = {
    cancel: "Cancel",
    close: "Close",
    createAgent: "Create",
    agentPublishCommunity: "Publish to community",
    agentPublishCommunityAndDeploy: "Publish and deploy",
    agentPublishCommunityTemplateOnly: "Publish template only",
    agentPublishLoginRequired: "Sign in first",
    agentPublishTemplateCommunitySubtitle: "Publish remotely",
    agentPublishTemplateTitle: "Publish agent template",
    agentPublishIncludeMemory: "Include agent memory",
    agentPublishIncludeMemoryWarning: "Memory may contain private information.",
    agentPublishing: "Publishing",
    resourcesDeleteSkill: "Delete skill",
    resourcesDeleteSkillConfirmAction: "Delete",
    resourcesDeleteSkillConfirmMessage: 'Delete skill "{name}"? This action cannot be undone.',
    resourcesAllTab: "All",
    resourcesEmpty: "No templates",
    resourcesImageLabel: "Image",
    resourcesTemplateEnvLabel: "Environment variables",
    resourcesTemplateEnvNotRequired: "Not required",
    resourcesTemplateEnvOptional: "Optional",
    resourcesTemplateEnvRequired: "Required",
    resourcesTemplateEnvRequiredBadge: "Required",
    resourcesTemplateSkillsDescription: "Template skills.",
    resourcesTemplateMemoryDescription: "Template memory summary.",
    resourcesTemplateMemoryEmptyTitle: "No memory summary",
    resourcesTemplateMemoryEmptyDescription: "Republish from an agent with memory.",
    resourcesTemplateMCPServersTitle: "MCP Servers",
    resourcesTemplateMCPServersDescription: "Template MCP configs.",
    resourcesLoading: "Loading resources",
    resourcesMCPServerDocumentInvalid: "MCP server definition must be valid JSON.",
    resourcesMCPServerDocumentJSONLabel: "MCP server JSON",
    resourcesMCPServerDocumentLabel: "MCP server definition",
    resourcesMCPServerDocumentObjectRequired: "MCP server definition must be a JSON object.",
    resourcesMCPServerDocumentInvalidShape:
      "MCP server definition must be an mcpServers JSON object with exactly one server.",
    resourcesMCPCreateTitle: "Add MCP Server",
    resourcesMCPDelete: "Delete",
    resourcesMCPDeleteConfirmMessage: 'Delete MCP server "{name}"?',
    resourcesMCPEmpty: "No MCP servers available yet.",
    resourcesMCPLoading: "Loading MCP servers",
    resourcesMCPSave: "Save",
    resourcesMCPSaving: "Saving...",
    resourcesMCPTest: "Test connection",
    resourcesMCPTesting: "Testing...",
    resourcesMCPTestSuccess: "Connection successful",
    resourcesMCPTestSuccessSummary: "MCP negotiation completed and {count} tools were discovered.",
    resourcesMCPTestDuration: "{duration} ms",
    resourcesMCPProtocolVersion: "Protocol {version}",
    resourcesMCPToolsTitle: "Available tools",
    resourcesMCPToolsEmpty: "The connection works, but no tools are currently available.",
    resourcesMCPToolsUnsupported: "The connection works, but the server does not advertise the tools capability.",
    resourcesMCPToolsTruncated: "This server exposes many tools. Showing the first {count}.",
    resourcesMCPToolParameterRequired: "Required",
    resourcesMCPToolParameterOptional: "Optional",
    resourcesMCPManualTab: "Manual configuration",
    resourcesMCPRemoteInstallAction: "Install",
    resourcesMCPRemoteInstallTab: "Remote install",
    resourcesMCPRemoteInstalling: "Installing...",
    resourcesMCPRemoteReplaceAction: "Replace",
    resourcesMCPRemoteServersEmpty: "No remote MCP servers yet.",
    resourcesMCPRemoteServersLoading: "Loading remote MCP servers...",
    resourcesMCPRemoteServersRefresh: "Refresh",
    resourcesMCPRemoteServersSearchPlaceholder: "Search remote MCP servers",
    resourcesRefresh: "Refresh templates",
    resourcesPublishCommunitySuccessTitle: "Published successfully",
    resourcesPublishCommunityDeployFailedTitle: "Template published, but deployment failed",
    resourcesPublishCommunitySuccessMessage: "The template has been published to the community.",
    resourcesPublishCommunitySuccessDismiss: "OK",
    resourcesSkillsEmpty: "No skills",
    resourcesSkillsLabel: "Skills",
    resourcesRuntimeLabel: "Runtime",
    agentProfileTab: "Profile",
    agentInstructions: "Instructions",
    agentInstructionsDefaultMode: "Default",
    agentInstructionsAdvancedMode: "Advanced",
    agentInstructionsEffective: "Effective AGENTS.md",
    agentInstructionsViewMode: "Instructions view",
    agentInstructionsPlaceholder: "Describe how this agent should work.",
    resourcesTemplateInstructionsDefaultHint: "Default mode shows only user-defined instructions.",
    resourcesTemplateInstructionsAdvancedHint: "View the complete AGENTS.md content applied by this template.",
    resourcesTemplateInstructionsEmptyTitle: "No custom instructions",
    resourcesTemplateInstructionsEmptyDescription:
      "This template only contains generated defaults. Switch to Advanced to view the full content.",
    resourcesTemplateInstructionsViewAdvancedAction: "View complete AGENTS.md",
    agentProfileSkillsTab: "Skills",
    agentMemoryTab: "Memory",
    agentMemoryDocumentLabel: "Read-only memory summary",
    agentProfileMCPTab: "MCP",
    agentProfileSectionNavLabel: "Template sections",
    agentSkillsTitle: "Skills",
    agentSkillsDescription: "Manage skills.",
    profileMCPServers: "MCP Servers",
    profileMCPServersHubHint: "Manage MCP servers.",
    profileRuntimeSection: "Runtime environment",
    profileRuntimeSectionDescription: "Select runtime.",
    resourcesSourceLabel: "Source",
    resourcesSubtitle: "Browse templates.",
    resourcesTemplateCountSuffix: "Agent templates",
    resourcesTitle: "Resources",
    resourcesUpdatedAtLabel: "Updated",
    resourcesWorkspaceBinary: "Binary file",
    resourcesWorkspaceEmptyFile: "Empty file",
    resourcesWorkspaceLoading: "Loading workspace",
    resourcesWorkspacePreviewHint: "Choose a file",
    resourcesWorkspacePreviewTitle: "Select a file",
    resourcesWorkspaceTemplateLabel: "Workspace",
    roleLabel: "Role",
    "roles.manager": "manager",
    workspacePreviewCodeTab: "Code",
    workspacePreviewPreviewTab: "Preview",
    workspacePreviewTruncated: "truncated",
    workspacePreviewViewMode: "View",
  };
  return (messages[key] || key).replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
}

const template = {
  id: "builtin/demo",
  name: "demo-template",
  description: "Demo template",
  image: "demo:latest",
  image_env: [
    {
      name: "GITLAB_TOKEN",
      required: true,
      secret: true,
      description: "GitLab API token",
    },
  ],
  role: "manager",
  runtime_kind: "openclaw_sandbox",
  source: { name: "builtin" },
  updated_at: "2026-05-29T03:10:23Z",
  workspace: {
    entries: [{ name: "README.md", path: "README.md", type: "file" }],
    kind: "openclaw_sandbox",
  },
};

function renderHubDetailPane(
  selectedResourceType: "mcp" | "skill" | "template" = "template",
  options: {
    selectedTemplate?: HubTemplate;
    onPublishTemplate?: (
      item: HubTemplate | null | undefined,
      deploy?: boolean,
      includeMemory?: boolean,
    ) => Promise<{ status: "success" } | { status: "partial"; message: string } | null>;
    publishDisabled?: boolean;
    publishError?: string;
    lazyMemory?: boolean;
  } = {},
) {
  const selectedTemplate = options.selectedTemplate ?? template;
  const workspaceFiles = {
    "instructions/AGENTS.md": {
      binary: false,
      content: "# Instructions\n\nFollow the template rules.",
      path: "instructions/AGENTS.md",
      size: 33,
    },
    "skills/demo/SKILL.md": {
      binary: false,
      content: "---\nname: demo\ndescription: Demo template skill\n---\n\nUse it carefully.",
      path: "skills/demo/SKILL.md",
      size: 70,
    },
    "mcps/mcp.json": {
      binary: false,
      content: JSON.stringify({
        context7: {
          command: "npx",
          args: ["-y", "context7-mcp"],
          description: "Context lookup",
        },
      }),
      path: "mcps/mcp.json",
      size: 120,
    },
    "memories/memory_summary.md": {
      binary: false,
      content: "# Memory Summary\n\nThe user prefers concise Chinese replies.",
      path: "memories/memory_summary.md",
      size: 58,
    },
  };
  function Harness() {
    const [selectedWorkspacePath, setSelectedWorkspacePath] = useState("instructions/AGENTS.md");
    const [workspaceEntries, setWorkspaceEntries] = useState([
      { name: "instructions", path: "instructions", type: "dir" as const, depth: 0 },
      { name: "AGENTS.md", path: "instructions/AGENTS.md", type: "file" as const, depth: 1 },
      { name: "memories", path: "memories", type: "dir" as const, depth: 0 },
      ...(!options.lazyMemory
        ? [{ name: "memory_summary.md", path: "memories/memory_summary.md", type: "file" as const, depth: 1 }]
        : []),
      { name: "skills", path: "skills", type: "dir" as const, depth: 0 },
      { name: "demo", path: "skills/demo", type: "dir" as const, depth: 1 },
      { name: "SKILL.md", path: "skills/demo/SKILL.md", type: "file" as const, depth: 2 },
      { name: "mcps", path: "mcps", type: "dir" as const, depth: 0 },
      { name: "mcp.json", path: "mcps/mcp.json", type: "file" as const, depth: 1 },
    ]);
    const workspaceFile = workspaceFiles[selectedWorkspacePath as keyof typeof workspaceFiles] || null;
    return (
      <HubDetailPane
        locale="en"
        t={t}
        onCreateFromTemplate={vi.fn()}
        hub={{
          detailPaneProps: {
            detailLoading: false,
            error: "",
            loaded: true,
            onRetry: vi.fn(),
            onSelectSkill: vi.fn(),
            onSelectSkillFile: vi.fn(),
            onSelectTemplate: vi.fn(),
            onSelectWorkspaceFile: setSelectedWorkspacePath,
            onToggleWorkspaceDir: (path) => {
              if (options.lazyMemory && path === "memories") {
                setWorkspaceEntries((entries) =>
                  entries.some((entry) => entry.path === "memories/memory_summary.md")
                    ? entries
                    : [
                        ...entries,
                        {
                          name: "memory_summary.md",
                          path: "memories/memory_summary.md",
                          type: "file" as const,
                          depth: 1,
                        },
                      ],
                );
              }
            },
            mcpServers: [],
            selectedMCPServer: null,
            selectedMCPServerName: "",
            selectedResourceType,
            selectedSkill: null,
            selectedSkillPath: "",
            selectedTemplate,
            selectedTemplateId: selectedTemplate.id || "",
            selectedWorkspacePath,
            skillFile: null,
            skillFileError: "",
            skillFileLoading: false,
            skills: [],
            skillTree: null,
            skillTreeError: "",
            skillTreeLoading: false,
            templates: [selectedTemplate],
            workspaceFile,
            workspaceFiles,
            workspaceFileError: "",
            workspaceFileLoading: false,
            workspaceEntries,
            deleteBusy: false,
            onDeleteTemplate: vi.fn(),
            onPublishTemplate: options.onPublishTemplate,
            publishDisabled: options.publishDisabled,
            publishError: options.publishError,
          },
        }}
      />
    );
  }
  return render(<Harness />);
}

function renderHubSkillDetailPane() {
  const onDeleteSkill = vi.fn().mockResolvedValue(true);
  return render(
    <HubDetailPane
      locale="en"
      t={t}
      onCreateFromTemplate={vi.fn()}
      hub={{
        detailPaneProps: {
          detailLoading: false,
          error: "",
          loaded: true,
          onRetry: vi.fn(),
          onSelectSkill: vi.fn(),
          onSelectSkillFile: vi.fn(),
          onSelectTemplate: vi.fn(),
          onSelectWorkspaceFile: vi.fn(),
          selectedResourceType: "skill",
          selectedSkill: {
            name: "demo-skill",
            description: "Demo skill",
          },
          selectedSkillPath: "SKILL.md",
          selectedTemplate: template,
          selectedTemplateId: template.id,
          selectedWorkspacePath: "",
          skillFile: {
            binary: false,
            content: "# Skill\n\nUse it carefully.",
            path: "SKILL.md",
            size: 26,
          },
          skillFileError: "",
          skillFileLoading: false,
          skillDeleteBusy: false,
          skills: [
            {
              name: "demo-skill",
              description: "Demo skill",
            },
          ],
          skillTree: {
            entries: [{ name: "SKILL.md", path: "SKILL.md", type: "file" }],
          },
          skillTreeError: "",
          skillTreeLoading: false,
          templates: [template],
          workspaceFile: null,
          workspaceFileError: "",
          workspaceFileLoading: false,
          deleteBusy: false,
          onDeleteSkill,
          onDeleteTemplate: vi.fn(),
        },
      }}
    />,
  );
}

function renderMCPDetailPane({
  mcpCreateDialogOpen = false,
  mcpCreateError = "",
  mcpProbeResult = null,
  managedSource = false,
  sourceError = "",
  sourceUpdateAvailable = false,
}: {
  mcpCreateDialogOpen?: boolean;
  mcpCreateError?: string;
  managedSource?: boolean;
  sourceError?: string;
  sourceUpdateAvailable?: boolean;
  mcpProbeResult?: {
    connected: boolean;
    durationMs: number;
    protocolVersion?: string;
    serverInfo?: { name?: string; title?: string; version?: string };
    tools: Array<{
      description?: string;
      inputSchema?: Record<string, unknown>;
      name: string;
      title?: string;
    }>;
    toolsSupported: boolean;
    truncated: boolean;
  } | null;
} = {}) {
  const onUpdateMCP = vi.fn().mockResolvedValue(true);
  const onProbeMCP = vi.fn().mockResolvedValue(mcpProbeResult);
  const onCheckMCPSource = vi.fn().mockResolvedValue(undefined);
  const onSyncMCPSource = vi.fn().mockResolvedValue(true);
  const mcp = {
    name: managedSource ? "kb-investment" : "grafana",
    description: "Grafana",
    config: managedSource
      ? {
          type: "remote",
          transport: "streamable-http",
          url: "https://old.example.test/mcp",
          headers: { Authorization: "Bearer saved-token" },
          _meta: {
            "com.opencsg/mcp": {
              auth_type: "csghub_access_token",
              content_id: "kb-investment",
              resource_id: "143",
              type: "llm_wiki",
            },
          },
        }
      : {
          command: "grafana-mcp",
          args: ["--transport", "stdio"],
          startup_timeout_sec: 120,
        },
  };
  const result = render(
    <HubDetailPane
      locale="en"
      t={t}
      onCreateFromTemplate={vi.fn()}
      hub={{
        detailPaneProps: {
          detailLoading: false,
          error: "",
          loaded: true,
          onRetry: vi.fn(),
          onSelectSkill: vi.fn(),
          onSelectSkillFile: vi.fn(),
          onSelectTemplate: vi.fn(),
          onSelectWorkspaceFile: vi.fn(),
          selectedResourceType: "mcp",
          selectedMCPServer: mcp,
          selectedMCPServerName: mcp.name,
          selectedSkill: null,
          selectedSkillPath: "",
          selectedTemplate: null,
          selectedTemplateId: "",
          selectedWorkspacePath: "",
          skillFile: null,
          skillFileError: "",
          skillFileLoading: false,
          skills: [],
          skillTree: null,
          skillTreeError: "",
          skillTreeLoading: false,
          templates: [],
          mcpServers: [mcp],
          mcpCreateDialogOpen,
          mcpCreateError,
          mcpMutationBusy: false,
          mcpMutationError: "",
          mcpProbeBusy: false,
          mcpProbeError: "",
          mcpProbeResult,
          mcpSourceBusy: false,
          mcpSourceError: sourceError,
          mcpSourceStatus: managedSource
            ? {
                authType: "csghub_access_token",
                configuredEndpointURL: "https://old.example.test/mcp",
                contentID: "kb-investment",
                kind: "llm_wiki",
                latestEndpointURL: sourceUpdateAvailable
                  ? "https://current.example.test/mcp"
                  : "https://old.example.test/mcp",
                resourceID: "143",
                updateAvailable: sourceUpdateAvailable,
              }
            : null,
          mcpSourceSyncBusy: false,
          mcpStateError: "",
          mcpStateLoading: false,
          onDeleteMCP: vi.fn(),
          onCheckMCPSource,
          onProbeMCP,
          onSyncMCPSource,
          onUpdateMCP,
          workspaceFile: null,
          workspaceFileError: "",
          workspaceFileLoading: false,
        },
      }}
    />,
  );
  return { ...result, onCheckMCPSource, onProbeMCP, onSyncMCPSource, onUpdateMCP };
}

function renderMCPCreateDialog() {
  const onInstallRemoteMCP = vi.fn().mockResolvedValue(true);
  const onRemoteMCPVisibleChange = vi.fn();
  const remoteMCP = {
    description: "Calendar tools",
    id: "builtin:calendar",
    name: "calendar",
    protocol: "streamable-http",
    url: "https://mcp.example.test/calendar",
  };
  const result = render(
    <HubDetailPane
      locale="en"
      t={t}
      onCreateFromTemplate={vi.fn()}
      hub={{
        detailPaneProps: {
          detailLoading: false,
          error: "",
          loaded: true,
          mcpCreateDialogOpen: true,
          mcpServers: [],
          onInstallRemoteMCP,
          onMCPCreateDialogOpenChange: vi.fn(),
          onRemoteMCPVisibleChange,
          onRetry: vi.fn(),
          onSelectSkillFile: vi.fn(),
          onSelectWorkspaceFile: vi.fn(),
          remoteMCPInstallBusy: "",
          remoteMCPServers: [remoteMCP],
          remoteMCPServersError: "",
          remoteMCPServersHasMore: false,
          remoteMCPServersLoading: false,
          remoteMCPServersLoadingMore: false,
          remoteMCPServersSearch: "",
          selectedMCPServer: null,
          selectedMCPServerName: "",
          selectedResourceType: "mcp",
          selectedSkill: null,
          selectedSkillPath: "",
          selectedTemplate: null,
          selectedTemplateId: "",
          selectedWorkspacePath: "",
          skillFile: null,
          skillFileError: "",
          skillFileLoading: false,
          skills: [],
          skillTree: null,
          skillTreeError: "",
          skillTreeLoading: false,
          templates: [],
          workspaceFile: null,
          workspaceFileError: "",
          workspaceFileLoading: false,
        },
      }}
    />,
  );
  return { ...result, onInstallRemoteMCP, onRemoteMCPVisibleChange };
}

describe("HubDetailPane", () => {
  it("publishes a local template to the community", async () => {
    const user = userEvent.setup();
    const onPublishTemplate = vi.fn().mockResolvedValue({ status: "success" });
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    renderHubDetailPane("template", { selectedTemplate: localTemplate, onPublishTemplate });

    await user.click(screen.getByRole("button", { name: "Publish to community" }));
    expect(screen.getByRole("dialog", { name: "Publish agent template" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish template only" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish and deploy" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Publish template only" }));

    expect(onPublishTemplate).toHaveBeenCalledWith(localTemplate, false, false);
    expect(await screen.findByRole("dialog", { name: "Published successfully" })).toBeInTheDocument();
    expect(screen.getByText("The template has been published to the community.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "OK" }));
    expect(screen.queryByRole("dialog", { name: "Published successfully" })).not.toBeInTheDocument();
  });

  it("does not show publishing success when publishing fails", async () => {
    const user = userEvent.setup();
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    renderHubDetailPane("template", {
      selectedTemplate: localTemplate,
      onPublishTemplate: vi.fn().mockResolvedValue(null),
    });

    await user.click(screen.getByRole("button", { name: "Publish to community" }));
    await user.click(screen.getByRole("button", { name: "Publish template only" }));

    expect(screen.queryByRole("dialog", { name: "Published successfully" })).not.toBeInTheDocument();
  });

  it("publishes and deploys a local template when selected", async () => {
    const user = userEvent.setup();
    const onPublishTemplate = vi.fn().mockResolvedValue({ status: "success" });
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    renderHubDetailPane("template", { selectedTemplate: localTemplate, onPublishTemplate });

    await user.click(screen.getByRole("button", { name: "Publish to community" }));
    await user.click(screen.getByRole("checkbox", { name: "Include agent memory" }));
    await user.click(screen.getByRole("button", { name: "Publish and deploy" }));

    expect(onPublishTemplate).toHaveBeenCalledWith(localTemplate, true, true);
  });

  it("closes the publish form and shows deployment details after partial success", async () => {
    const user = userEvent.setup();
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    renderHubDetailPane("template", {
      selectedTemplate: localTemplate,
      onPublishTemplate: vi.fn().mockResolvedValue({ status: "partial", message: "Review failed\nUnsafe content" }),
    });

    await user.click(screen.getByRole("button", { name: "Publish to community" }));
    await user.click(screen.getByRole("button", { name: "Publish and deploy" }));

    expect(screen.queryByRole("dialog", { name: "Publish agent template" })).not.toBeInTheDocument();
    expect(
      await screen.findByRole("dialog", { name: "Template published, but deployment failed" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Unsafe content/)).toBeInTheDocument();
  });

  it("shows publishing failures inside the publish choice dialog", async () => {
    const user = userEvent.setup();
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    renderHubDetailPane("template", {
      selectedTemplate: localTemplate,
      publishError: "Template was published, but deployment failed.",
    });

    await user.click(screen.getByRole("button", { name: "Publish to community" }));

    expect(screen.getByRole("dialog", { name: "Publish agent template" })).toBeInTheDocument();
    expect(screen.getByText("Template was published, but deployment failed.")).toBeInTheDocument();
  });

  it("requires sign-in before publishing a local template to the community", () => {
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    renderHubDetailPane("template", { selectedTemplate: localTemplate, publishDisabled: true });

    const publish = screen.getByRole("button", { name: "Publish to community" });
    expect(publish).toBeDisabled();
    expect(publish).toHaveAttribute("title", "Sign in first");
  });

  it("does not show community publishing for a local OpenClaw template", () => {
    const localOpenClawTemplate = {
      ...template,
      id: "local.openclaw-template",
      source: { name: "local", kind: "local" },
    };
    renderHubDetailPane("template", { selectedTemplate: localOpenClawTemplate });

    expect(screen.queryByRole("button", { name: "Publish to community" })).not.toBeInTheDocument();
  });

  it("groups template details into runtime, instructions, memory, skills, and MCP tabs", async () => {
    const user = userEvent.setup();
    const { container } = renderHubDetailPane("template", {
      selectedTemplate: { ...template, runtime_kind: "codex" },
    });

    expect(screen.getByRole("button", { name: "Profile" })).toHaveAttribute("aria-current", "location");
    expect(screen.getByDisplayValue("demo:latest")).toBeInTheDocument();
    expect(screen.getAllByText("Environment variables").length).toBeGreaterThan(0);
    expect(screen.getByText("GITLAB_TOKEN")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Instructions" }));
    expect(screen.getByRole("button", { name: "Profile" })).toBeInTheDocument();
    expect(screen.getByText("No custom instructions")).toBeInTheDocument();
    expect(
      screen.getByText("This template only contains generated defaults. Switch to Advanced to view the full content."),
    ).toBeInTheDocument();
    expect(screen.getByText("Default mode shows only user-defined instructions.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Advanced" }));
    expect(screen.getByLabelText("Instructions")).toHaveTextContent("# Instructions Follow the template rules.");
    await user.click(screen.getByRole("button", { name: /^Skills/ }));
    expect(screen.getByText("demo")).toBeInTheDocument();
    expect(screen.getByText("Demo template skill")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Memory" }));
    expect(screen.getByRole("textbox", { name: "Read-only memory summary" })).toHaveValue(
      "# Memory Summary\n\nThe user prefers concise Chinese replies.",
    );
    expect(container.querySelector(".workspace-file-tree")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^MCP/ }));
    expect(screen.getByText("context7")).toBeInTheDocument();
    expect(screen.getByText("Context lookup")).toBeInTheDocument();
  });

  it("selects the memory summary after the memories directory is lazily loaded", async () => {
    const user = userEvent.setup();
    renderHubDetailPane("template", {
      selectedTemplate: { ...template, runtime_kind: "codex" },
      lazyMemory: true,
    });

    await user.click(screen.getByRole("button", { name: "Memory" }));

    expect(await screen.findByRole("textbox", { name: "Read-only memory summary" })).toHaveValue(
      "# Memory Summary\n\nThe user prefers concise Chinese replies.",
    );
  });

  it("keeps the MCP empty state visible when templates are available", () => {
    renderHubDetailPane("mcp");

    expect(screen.getByText("No MCP servers available yet.")).toBeInTheDocument();
    expect(screen.queryByText("demo-template")).not.toBeInTheDocument();
  });

  it("shows template profile instructions as a readonly preview", async () => {
    const user = userEvent.setup();
    renderHubDetailPane();

    await user.click(screen.getByRole("button", { name: "Instructions" }));
    expect(screen.getByText("No custom instructions")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "View complete AGENTS.md" }));

    expect(screen.getByRole("button", { name: /^Skills/ })).toBeInTheDocument();
    expect(screen.getByLabelText("Instructions")).toHaveTextContent("# Instructions Follow the template rules.");
    expect(screen.getByText("Effective AGENTS.md")).toBeInTheDocument();
    expect(screen.queryByText("AGENTS.md")).not.toBeInTheDocument();
  });

  it("renders the selected skill with file tree but without template details", () => {
    renderHubSkillDetailPane();

    expect(screen.getAllByRole("heading", { name: "demo-skill" }).length).toBeGreaterThan(0);
    expect(screen.getAllByText("Demo skill").length).toBeGreaterThan(0);
    expect(screen.queryByText("demo-template")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete skill" })).toBeInTheDocument();
    expect(screen.getAllByText("SKILL.md").length).toBeGreaterThan(0);
    expect(screen.getByText("# Skill", { exact: false })).toBeInTheDocument();
    expect(screen.queryByText("Description")).not.toBeInTheDocument();
  });

  it("opens a confirmation dialog before deleting a skill", async () => {
    const user = userEvent.setup();
    renderHubSkillDetailPane();

    await user.click(screen.getByRole("button", { name: "Delete skill" }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText('Delete skill "demo-skill"? This action cannot be undone.')).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Delete" }).length).toBeGreaterThan(0);
  });

  it("highlights and validates MCP JSON configs before saving", async () => {
    const user = userEvent.setup();
    const { container, onUpdateMCP } = renderMCPDetailPane();

    expect(container.querySelector(".cm-editor")).toBeInTheDocument();
    expect(container.textContent).toContain("mcpServers");
    expect(container.textContent).toContain("grafana-mcp");

    const editor = screen.getByRole("textbox", { name: "MCP server definition" });
    await user.click(editor);
    await user.keyboard("{Control>}a{/Control}");
    await user.keyboard("not json");

    expect(screen.queryByText("MCP server definition must be valid JSON.")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(onUpdateMCP).not.toHaveBeenCalled();
  });

  it("tests the current MCP draft and renders discovered tools with parameters", async () => {
    const user = userEvent.setup();
    const { onProbeMCP } = renderMCPDetailPane({
      mcpProbeResult: {
        connected: true,
        durationMs: 24,
        protocolVersion: "2025-11-25",
        serverInfo: { title: "Grafana MCP", version: "1.0.0" },
        toolsSupported: true,
        truncated: false,
        tools: [
          {
            name: "search_dashboards",
            title: "Search dashboards",
            description: "Find matching dashboards.",
            inputSchema: {
              type: "object",
              properties: { query: { type: "string", description: "Search query" } },
              required: ["query"],
            },
          },
        ],
      },
    });

    expect(screen.getByText("Connection successful")).toBeInTheDocument();
    expect(screen.getByText("Search dashboards")).toBeInTheDocument();
    expect(screen.getByText("search_dashboards")).toBeInTheDocument();
    expect(screen.getByText("query")).toBeInTheDocument();
    expect(screen.getByText("Required")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Test connection" }));
    expect(onProbeMCP).toHaveBeenCalledWith({
      name: "grafana",
      config: {
        command: "grafana-mcp",
        args: ["--transport", "stdio"],
        startup_timeout_sec: 120,
      },
    });
  });

  it("shows only actionable knowledge base MCP updates", async () => {
    const user = userEvent.setup();
    const { onCheckMCPSource, onSyncMCPSource } = renderMCPDetailPane({
      managedSource: true,
      sourceUpdateAvailable: true,
    });

    expect(screen.getByText("resourcesKnowledgeMCPBadge")).toBeInTheDocument();
    expect(screen.getByText("resourcesKnowledgeMCPUpdateAvailable")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "resourcesMCPSourceRetry" })).not.toBeInTheDocument();
    expect(onCheckMCPSource).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "resourcesMCPSourceUpdate" }));
    expect(onSyncMCPSource).toHaveBeenCalledTimes(1);
  });

  it("keeps a healthy knowledge base MCP status out of the way", () => {
    renderMCPDetailPane({ managedSource: true });

    expect(screen.getByText("resourcesKnowledgeMCPBadge")).toBeInTheDocument();
    expect(screen.queryByText("resourcesKnowledgeMCPUpdateAvailable")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "resourcesMCPSourceUpdate" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "resourcesMCPSourceRetry" })).not.toBeInTheDocument();
  });

  it("shows MCP creation errors only inside the creation dialog", () => {
    renderMCPDetailPane({
      mcpCreateDialogOpen: true,
      mcpCreateError: "mcp server already exists: filesystem",
    });

    expect(screen.getByRole("dialog")).toHaveTextContent("mcp server already exists: filesystem");
    expect(screen.getAllByText("mcp server already exists: filesystem")).toHaveLength(1);
  });

  it("installs a remote MCP through the Hub install flow", async () => {
    const user = userEvent.setup();
    const { onInstallRemoteMCP, onRemoteMCPVisibleChange } = renderMCPCreateDialog();

    await user.click(screen.getByRole("tab", { name: "Remote install" }));

    expect(onRemoteMCPVisibleChange).toHaveBeenCalledWith(true);
    expect(screen.getByText("calendar")).toBeInTheDocument();
    expect(screen.getByText("Calendar tools")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Install" }));

    expect(onInstallRemoteMCP).toHaveBeenCalledWith({
      description: "Calendar tools",
      id: "builtin:calendar",
      name: "calendar",
      protocol: "streamable-http",
      url: "https://mcp.example.test/calendar",
    });
  });
});
