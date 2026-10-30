import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { fireEvent, render as rtlRender, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { useState, type ReactNode } from "react";
import { HubDetailPane } from "@/components/business/HubDetailPane";
import type { HubTemplate } from "@/models/hubWorkspace";

function render(ui: ReactNode) {
  return rtlRender(<MemoryRouter>{ui}</MemoryRouter>);
}
beforeEach(() =>
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items: [] })),
  ),
);
afterEach(() => vi.unstubAllGlobals());

function t(key: string, params: Record<string, string | number> = {}) {
  const messages: Record<string, string> = {
    cancel: "Cancel",
    close: "Close",
    iaTemplateCreateAgent: "Create agent",
    agentPublishCommunity: "Publish to community",
    resourcesTemplatePublished: "Published",
    resourcesTemplatePublishedOnHub: "Published on AgenticHub",
    templateCommunityVersion: "Community version {version}",
    templateLastPublishedVersion: "Last published {version}",
    templateViewCommunity: "View community template",
    templateCurrentVersionPublished: "Current version published",
    templateVersionPublished: "{version} published",
    templateCommunityVersionDifferent: "Differs from community version",
    templateCommunityVersionUnconfirmed: "Community version unconfirmed",
    agentPublishCommunityAndDeploy: "Publish and deploy",
    agentPublishCommunityTemplateOnly: "Publish template only",
    templatePublishCommunitySubtitle: "Publish the latest saved local template version to the community.",
    templatePublishVersionLabel: "Version to publish",
    templatePublishLegacyVersion: "Legacy template (unversioned)",
    templatePublishLatestHint:
      "Only the active version is published; history is not included. The cloud keeps one current copy.",
    templatePublishVersionAction: "Publish {version} to community",
    agentPublishLoginRequired: "Sign in first",
    agentPublishTemplateCommunitySubtitle: "Publish remotely",
    agentPublishTemplateTitle: "Publish agent template",
    agentPublishIncludeMemory: "Include agent memory",
    agentPublishIncludeMemoryWarning: "Memory may contain private information.",
    agentPublishing: "Publishing",
    resourcesDeleteSkill: "Delete skill",
    resourcesDeleteSkillConfirmAction: "Delete",
    resourcesDeleteSkillConfirmMessage: 'Delete skill "{name}"? This action cannot be undone.',
    resourcesSkillAdd: "Add",
    resourcesSkillAllFilter: "All",
    resourcesSkillEnabled: "Enabled",
    resourcesSkillInstalledTitle: "Installed",
    resourcesSkillListSubtitle: "Extend CSGClaw with task-specific skills.",
    resourcesSkillLocalFilter: "Local",
    resourcesSkillOptions: "More actions",
    resourcesSkillRemoteFilter: "Remote",
    resourcesSkillSearchPlaceholder: "Search skills",
    resourcesSkillTryNow: "Try now",
    resourcesAllTab: "All",
    resourcesEmpty: "No templates",
    resourcesImageLabel: "Image",
    resourcesTemplateEnvLabel: "Environment variables",
    resourcesTemplateListSubtitle: "Create reusable CSGClaw agents from templates.",
    resourcesTemplateSearchPlaceholder: "Search templates by name, description, or source",
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
    resourcesMCPListSubtitle: "Manage MCP server configurations that agents can use.",
    resourcesMCPSearchPlaceholder: "Search MCP servers by name, description, or config",
    resourcesMCPServerDocumentObjectRequired: "MCP server definition must be a JSON object.",
    resourcesMCPServerDocumentInvalidShape:
      "MCP server definition must be an mcpServers JSON object with exactly one server.",
    resourcesMCPCreateTitle: "Add MCP Server",
    resourcesMCPCreateAction: "Add",
    resourcesMCPCreating: "Adding...",
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
    resourcesKnowledgeBasesLabel: "Knowledge bases",
    resourcesKnowledgeBasesDescription: "Remote knowledge bases",
    resourcesKnowledgeBaseSearchPlaceholder: "Search knowledge bases by name or description",
    resourcesKnowledgeBaseAdded: "Added to MCP",
    resourcesKnowledgeBaseAddMCP: "Add to MCP",
    resourcesKnowledgeBaseAgenticHub: "AgenticHub knowledge base",
    resourcesKnowledgeBaseAvailable: "Available",
    resourcesKnowledgeBaseConfiguredDescription: "Managed in AgenticHub and enabled through agent MCP settings.",
    resourcesKnowledgeBaseGuideContinue: "Add",
    resourcesKnowledgeBaseGuideDescription:
      'MCP management will open with "{name}" filled in automatically. After adding it, you can enable the knowledge base from an agent\'s MCP configuration.',
    resourcesKnowledgeBaseGuideLater: "Later",
    resourcesKnowledgeBaseGuideTitle: "Add knowledge base to MCP",
    resourcesKnowledgeBaseHowToDescription: "Save it, then enable it for an agent.",
    resourcesKnowledgeBaseHowToTitle: "Enable for an agent",
    resourcesKnowledgeBaseMCPConfigLabel: "Knowledge base MCP configuration",
    resourcesKnowledgeBaseMCPNameLabel: "MCP name ·",
    resourcesKnowledgeBaseMCPServerBadge: "MCP Server",
    resourcesKnowledgeBaseRemoveMCP: "Remove from MCP",
    resourcesKnowledgeBaseRemoveMCPConfirm: 'Remove "{name}" from MCP?',
    resourcesKnowledgeBaseViewMCP: "View in MCP",
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
    agentAppsTab: "Connectors",
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
  runtime_kind: "dsh",
  source: { name: "builtin" },
  updated_at: "2026-05-29T03:10:23Z",
  workspace: {
    entries: [{ name: "README.md", path: "README.md", type: "file" }],
    kind: "dsh",
  },
};

function renderHubDetailPane(
  selectedResourceType: "mcp" | "skill" | "template" = "template",
  options: {
    selectedTemplate?: HubTemplate;
    publishedTemplate?: HubTemplate;
    agenticHubSiteURL?: string;
    templateUsername?: string;
    templates?: HubTemplate[];
    onPublishTemplate?: (
      item: HubTemplate | null | undefined,
      includeMemory?: boolean,
    ) => Promise<{ status: "success" } | null>;
    publishDisabled?: boolean;
    publishError?: string;
    onResetPublishError?: () => void;
    onSelectTemplate?: (item: HubTemplate | null | undefined) => void;
    queryClient?: QueryClient;
    lazyMemory?: boolean;
    loaded?: boolean;
    inlineTemplateDetail?: boolean;
  } = {},
) {
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
    const selectedTemplate = options.selectedTemplate ?? template;
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
        agenticHubSiteURL={options.agenticHubSiteURL}
        templateUsername={options.templateUsername}
        inlineTemplateDetail={options.inlineTemplateDetail}
        locale="en"
        t={t}
        onCreateFromTemplate={vi.fn()}
        hub={{
          detailPaneProps: {
            detailLoading: false,
            error: "",
            loaded: options.loaded ?? true,
            onRetry: vi.fn(),
            onSelectSkill: vi.fn(),
            onSelectSkillFile: vi.fn(),
            onSelectTemplate: options.onSelectTemplate || vi.fn(),
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
            templates: options.templates ?? [selectedTemplate],
            workspaceFile,
            workspaceFiles,
            workspaceFileError: "",
            workspaceFileLoading: false,
            workspaceEntries,
            deleteBusy: false,
            onDeleteTemplate: vi.fn(),
            publishedTemplate: options.publishedTemplate,
            onPublishTemplate: options.onPublishTemplate,
            publishDisabled: options.publishDisabled,
            publishError: options.publishError,
            onResetPublishError: options.onResetPublishError,
          },
        }}
      />
    );
  }
  const client = options.queryClient || new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const content = () => (
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>
  );
  const view = render(content());
  return {
    ...view,
    rerenderOptions: (patch: Partial<typeof options>) => {
      Object.assign(options, patch);
      view.rerender(content());
    },
  };
}

function renderHubSkillDetailPane(onDeleteSkill = vi.fn().mockResolvedValue(true)) {
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
                sourceAvailable: true,
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

function renderMCPCreateDialog(initialOpen = true) {
  const onInstallRemoteMCP = vi.fn().mockResolvedValue(true);
  const onRemoteMCPVisibleChange = vi.fn();
  const remoteMCP = {
    description: "Calendar tools",
    id: "builtin:calendar",
    name: "calendar",
    protocol: "streamable-http",
    url: "https://mcp.example.test/calendar",
  };
  function Harness() {
    const [open, setOpen] = useState(initialOpen);
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
            mcpCreateDialogOpen: open,
            mcpServers: [],
            onInstallRemoteMCP,
            onMCPCreateDialogOpenChange: setOpen,
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
      />
    );
  }
  const result = render(<Harness />);
  return { ...result, onInstallRemoteMCP, onRemoteMCPVisibleChange };
}

function renderKnowledgeBaseDetail(configured: boolean) {
  const knowledgeBase = {
    availability: "available" as const,
    configuredMCPName: configured ? "kb-investment" : undefined,
    contentID: "kb-investment",
    description: "Investment knowledge base",
    id: "143",
    name: "Investment handbook",
  };
  const mcp = {
    name: "kb-investment",
    description: "Investment knowledge base",
    config: {
      description: "Investment handbook | AgenticHub knowledge base",
      type: "remote",
      url: "https://example.test/mcp",
      headers: { Authorization: "Bearer saved-token" },
    },
  };
  const requestMCPConfig = vi.fn();
  const confirmMCPConfig = vi.fn().mockResolvedValue(true);
  const cancelMCPConfig = vi.fn();
  const onDeleteMCP = vi.fn().mockResolvedValue(true);
  const onSelectMCP = vi.fn();

  function KnowledgeBaseDetailHarness() {
    const [pendingMCPKnowledgeBase, setPendingMCPKnowledgeBase] = useState<typeof knowledgeBase | null>(null);
    return (
      <HubDetailPane
        locale="en"
        t={t}
        hub={{
          detailPaneProps: {
            detailLoading: false,
            error: "",
            knowledgeBases: {
              cancelMCPConfig: () => {
                cancelMCPConfig();
                setPendingMCPKnowledgeBase(null);
              },
              confirmMCPConfig,
              copyBusyID: "",
              copyError: "",
              discoveryHasMore: false,
              discoveryItems: [],
              discoveryLoadError: "",
              discoveryLoadMore: async () => {},
              discoveryLoading: false,
              discoveryLoadingMore: false,
              discoveryRefetch: async () => {},
              items: [knowledgeBase],
              loadError: "",
              loading: false,
              loginRequired: false,
              pendingMCPKnowledgeBase,
              requestMCPConfig: async (id) => {
                requestMCPConfig(id);
                setPendingMCPKnowledgeBase(knowledgeBase);
                return true;
              },
              search: "",
              selected: knowledgeBase,
              setSearch: vi.fn(),
            },
            loaded: true,
            mcpServers: configured ? [mcp] : [],
            onDeleteMCP,
            onRetry: vi.fn(),
            onSelectMCP,
            onSelectSkillFile: vi.fn(),
            onSelectWorkspaceFile: vi.fn(),
            selectedMCPServer: null,
            selectedMCPServerName: "",
            selectedResourceType: "knowledge",
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
      />
    );
  }

  return {
    ...render(<KnowledgeBaseDetailHarness />),
    confirmMCPConfig,
    mcp,
    onDeleteMCP,
    onSelectMCP,
    requestMCPConfig,
  };
}

async function openKnowledgeBaseDetail(user: ReturnType<typeof userEvent.setup>) {
  if (screen.queryByRole("dialog")) return;
  await user.click(screen.getByRole("button", { name: /Investment handbook/i }));
}

async function openTemplateDetail(user: ReturnType<typeof userEvent.setup>, name = /demo-template/i) {
  if (screen.queryByRole("dialog")) return;
  await user.click(screen.getByRole("button", { name }));
}

async function openMCPDetail(user: ReturnType<typeof userEvent.setup>, name = /grafana/i) {
  if (screen.queryByRole("dialog")) return;
  await user.click(screen.getByRole("button", { name }));
}

describe("HubDetailPane", () => {
  it("shows the template display name in the list and omits the ID subtitle", async () => {
    const user = userEvent.setup();
    const selected: HubTemplate = {
      ...template,
      id: "local.internal-template-id",
      name: "internal-template-id",
      display_name: "展示名称",
      namespace: "alice",
      description: "",
      source: { kind: "local" },
    };
    renderHubDetailPane("template", { selectedTemplate: selected, templates: [selected] });
    await user.click(screen.getByRole("button", { name: "Close" }));
    const card = screen.getByRole("button", { name: /展示名称/ });
    expect(card).not.toHaveTextContent("alice/展示名称");
    await user.click(card);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "alice/展示名称" })).toBeInTheDocument();
    expect(within(dialog).queryByText("local.internal-template-id")).not.toBeInTheDocument();
  });

  it.each(["local", "remote"] as const)("reveals a selected %s template in My templates", (kind) => {
    const selected = {
      ...template,
      id: kind === "local" ? "local.saved" : "alice/saved",
      name: "saved",
      source: { kind },
    };
    renderHubDetailPane("template", {
      selectedTemplate: selected,
      templateUsername: "alice",
      templates: [selected, template],
    });
    expect(screen.getByRole("tab", { name: "taskEntryOverview_personal", hidden: true })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(document.querySelector('button[aria-current="true"]')).toHaveTextContent("saved");
  });

  it.each(["local", "remote"])("keeps %s template details closed while creating an agent", async (kind) => {
    const user = userEvent.setup();
    const selectedTemplate: HubTemplate = {
      ...template,
      id: `${kind}/demo`,
      source: { name: kind, kind },
    };
    const onCreate = vi.fn();
    function Harness() {
      const [creating, setCreating] = useState(false);
      return (
        <>
          <HubDetailPane
            t={t}
            onCreateFromTemplate={async (item) => {
              onCreate(item);
              await Promise.resolve();
              setCreating(true);
            }}
            hub={{
              detailPaneProps: {
                selectedResourceType: "template",
                selectedTemplate: { ...selectedTemplate },
                selectedTemplateId: selectedTemplate.id || "",
                error: "",
                onRetry: vi.fn(),
                onSelectWorkspaceFile: vi.fn(),
                skills: [],
                selectedSkill: null,
                selectedSkillPath: "",
                selectedWorkspacePath: "",
                skillFile: null,
                skillFileError: "",
                skillFileLoading: false,
                skillTree: null,
                skillTreeError: "",
                skillTreeLoading: false,
                workspaceFile: null,
                workspaceFileError: "",
                workspaceFileLoading: false,
                workspaceEntries: [],
                templates: [selectedTemplate],
                loaded: true,
              },
            }}
          />
          {creating && <input aria-label="Agent name" />}
        </>
      );
    }
    render(<Harness />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create agent" }));
    const name = await screen.findByRole("textbox", { name: "Agent name" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(onCreate).toHaveBeenCalledWith(selectedTemplate);
    await user.type(name, "New agent");
    expect(name).toHaveValue("New agent");
    await openTemplateDetail(user, /demo-template/i);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("opens the shared login flow instead of knowledge-base discovery while signed out", async () => {
    const user = userEvent.setup();
    const onKnowledgeBaseLogin = vi.fn();
    render(
      <HubDetailPane
        locale="en"
        t={t}
        hub={{
          detailPaneProps: {
            detailLoading: false,
            error: "",
            knowledgeBases: {
              cancelMCPConfig: vi.fn(),
              confirmMCPConfig: async () => false,
              copyBusyID: "",
              copyError: "",
              discoveryHasMore: false,
              discoveryItems: [],
              discoveryLoadError: "",
              discoveryLoadMore: async () => {},
              discoveryLoading: false,
              discoveryLoadingMore: false,
              discoveryRefetch: async () => {},
              items: [],
              loadError: "",
              loading: false,
              loginRequired: true,
              pendingMCPKnowledgeBase: null,
              requestMCPConfig: async () => false,
              search: "",
              selected: null,
              setSearch: vi.fn(),
            },
            loaded: true,
            mcpServers: [],
            onKnowledgeBaseLogin,
            onRetry: vi.fn(),
            onSelectWorkspaceFile: vi.fn(),
            selectedMCPServer: null,
            selectedMCPServerName: "",
            selectedResourceType: "knowledge",
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

    await user.click(screen.getByRole("button", { name: /resourcesMCPAdd/i }));

    expect(onKnowledgeBaseLogin).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("renders the initial resource loading state as an accessible status", () => {
    renderHubDetailPane("template", { loaded: false });

    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("Loading resources");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status.querySelector("svg")).toBeInTheDocument();
  });

  it("explains the automatic handoff before opening MCP management", async () => {
    const user = userEvent.setup();
    const { confirmMCPConfig, requestMCPConfig } = renderKnowledgeBaseDetail(false);

    await openKnowledgeBaseDetail(user);
    await user.click(screen.getByRole("button", { name: "Add to MCP" }));

    expect(requestMCPConfig).toHaveBeenCalledWith("143");
    expect(screen.getAllByRole("dialog").at(-1)!).toHaveTextContent(
      'MCP management will open with "Investment handbook" filled in automatically. After adding it, you can enable the knowledge base from an agent\'s MCP configuration.',
    );
    expect(confirmMCPConfig).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Add" }));

    expect(confirmMCPConfig).toHaveBeenCalledTimes(1);
  });

  it("shows the configured MCP with safe preview, navigation, and removal", async () => {
    const user = userEvent.setup();
    const { mcp, onDeleteMCP, onSelectMCP } = renderKnowledgeBaseDetail(true);

    await openKnowledgeBaseDetail(user);
    expect(screen.getByText("kb-investment")).toBeInTheDocument();
    const preview = screen.getByLabelText("Knowledge base MCP configuration");
    expect(preview).toHaveTextContent("Bearer ${OPENCSG_TOKEN}");
    expect(preview).toHaveTextContent('"description": "Investment handbook | Investment knowledge base"');
    expect(preview).not.toHaveTextContent("AgenticHub knowledge base");
    expect(preview).not.toHaveTextContent("saved-token");

    await user.click(screen.getByRole("button", { name: "View in MCP" }));
    expect(onSelectMCP).toHaveBeenCalledWith("kb-investment");

    await user.click(screen.getByRole("button", { name: "Remove from MCP" }));
    expect(screen.getAllByRole("dialog").at(-1)!).toHaveTextContent('Remove "kb-investment" from MCP?');
    await user.click(screen.getAllByRole("button", { name: "Remove from MCP" }).at(-1)!);

    expect(onDeleteMCP).toHaveBeenCalledWith(mcp);
  });

  it("offers cloud creation only after publishing succeeds, using the complete returned template ID", async () => {
    const user = userEvent.setup();
    renderHubDetailPane("template", {
      agenticHubSiteURL: "https://opencsg-stg.com/account",
      selectedTemplate: {
        ...template,
        id: "local.repo-id",
        name: "api-template",
        display_name: "中文展示名",
        runtime_kind: "codex",
        source: { kind: "local", name: "local" },
      },
      onPublishTemplate: vi
        .fn()
        .mockResolvedValue({ status: "success", template: { id: "alice/repo-id", name: "api-template" } }),
    });
    await openTemplateDetail(user);
    expect(screen.queryByRole("link", { name: "agentTemplateCreateInHub" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Publish to community" }));
    expect(screen.queryByRole("link", { name: "agentTemplateCreateInHub" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Publish template only" }));
    const success = await screen.findByRole("dialog", { name: "Published successfully" });
    const link = within(success).getByRole("link", { name: "agentTemplateCreateInHub" });
    expect(link).toHaveAttribute(
      "href",
      "https://opencsg-stg.com/agentichub?action=create-agent&template_name=alice%2Frepo-id",
    );
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("shows success for an agent publication and uses its complete returned template ID", async () => {
    renderHubDetailPane("template", {
      agenticHubSiteURL: "https://opencsg.com",
      publishedTemplate: {
        id: "alice/repo",
        name: "returned-name",
        display_name: "展示名",
        source: { kind: "remote" },
      },
    });
    const success = await screen.findByRole("dialog", { name: "Published successfully" });
    const link = within(success).getByRole("link", { name: "agentTemplateCreateInHub" });
    expect(link).toHaveAttribute(
      "href",
      "https://opencsg.com/agentichub?action=create-agent&template_name=alice%2Frepo",
    );
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("shows the saved local avatar and no source badge in signed-out My templates", async () => {
    const user = userEvent.setup();
    renderHubDetailPane("template", {
      selectedTemplate: {
        ...template,
        avatar: "data:image/png;base64,aGVsbG8=",
        source: { kind: "local", name: "local" },
      },
    });
    await screen.findByRole("dialog");
    await user.click(screen.getByRole("button", { name: "Close" }));
    await user.click(screen.getByRole("tab", { name: "taskEntryOverview_personal" }));
    const card = screen.getByRole("button", { name: /demo-template/ });
    expect(screen.queryByRole("link", { name: "agentTemplateCreateInHub" })).not.toBeInTheDocument();
    expect(card.querySelector("img")).toHaveAttribute("src", "data:image/png;base64,aGVsbG8=");
    expect(card.querySelector('[class*="hub-template-source-badge"]')).toBeNull();
    expect(screen.getByText("resourcesMyTemplatesLoginHint")).toBeInTheDocument();
  });

  it.each([false, true])(
    "leaves same-name cloud versions unconfirmed and allows publishing (inline: %s)",
    async (inlineTemplateDetail) => {
      const localTemplate = {
        ...template,
        id: "local.demo-template",
        runtime_kind: "codex",
        source: { kind: "local" },
      };
      renderHubDetailPane("template", {
        selectedTemplate: localTemplate,
        templateUsername: "alice",
        agenticHubSiteURL: "https://custom.example:8443/login",
        inlineTemplateDetail,
        templates: [
          localTemplate,
          { ...template, id: "alice/demo-template", namespace: "alice", source: { kind: "remote" } },
        ],
      });
      if (!inlineTemplateDetail) await openTemplateDetail(userEvent.setup());
      expect(screen.getByRole("link", { name: "View community template" })).toHaveAttribute(
        "href",
        "https://custom.example:8443/agentichub?view=resources&subview=template",
      );
      expect(screen.getByRole("button", { name: "Publish to community" })).toBeEnabled();
      expect(screen.getAllByText("Community version unconfirmed").length).toBeGreaterThan(0);
    },
  );

  it.each([false, true])(
    "shows verified community versions and rollback differences (inline: %s)",
    async (inlineTemplateDetail) => {
      let remoteRevision = 2;
      vi.stubGlobal(
        "fetch",
        vi.fn(async () =>
          Response.json({
            ...template,
            id: "alice/original",
            uid: "identity",
            revision: remoteRevision,
            source: { kind: "remote" },
          }),
        ),
      );
      const localTemplate: HubTemplate = {
        ...template,
        id: "local.renamed",
        name: "renamed",
        uid: "identity",
        revision: 3,
        runtime_kind: "codex",
        source: { kind: "local" },
        publications: [
          {
            site: "https://hub.example",
            namespace: "alice",
            template_id: "alice/original",
            revision: 1,
            published_at: "2026-10-29T00:00:00Z",
          },
        ],
      };
      const view = renderHubDetailPane("template", {
        selectedTemplate: localTemplate,
        templates: [localTemplate],
        templateUsername: "alice",
        agenticHubSiteURL: "https://hub.example",
        inlineTemplateDetail,
      });
      if (!inlineTemplateDetail) await openTemplateDetail(userEvent.setup());
      expect(await screen.findByRole("link", { name: "Community version V2" })).toBeInTheDocument();
      expect(screen.getByText("Differs from community version")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Publish V3 to community" })).toBeEnabled();
      remoteRevision = 3;
      view.rerenderOptions({ selectedTemplate: { ...localTemplate, revision: 1 } });
      expect(await screen.findByRole("link", { name: "Community version V3" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Publish V1 to community" })).toBeEnabled();
      expect(screen.getByText("Differs from community version")).toBeInTheDocument();
      view.rerenderOptions({ selectedTemplate: { ...localTemplate, revision: 3 } });
      expect(await screen.findByText("Current version published")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "V3 published" })).toBeDisabled();
      expect(screen.queryByRole("button", { name: "Publish V3 to community" })).not.toBeInTheDocument();
      view.rerenderOptions({ selectedTemplate: { ...localTemplate, revision: 4 } });
      expect(await screen.findByText("Differs from community version")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Publish V4 to community" })).toBeEnabled();
    },
  );

  it("falls back to the last published version when community lookup fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("offline");
      }),
    );
    const localTemplate: HubTemplate = {
      ...template,
      id: "local.demo-template",
      uid: "identity",
      revision: 3,
      runtime_kind: "codex",
      source: { kind: "local" },
      publications: [
        {
          site: "https://hub.example",
          namespace: "alice",
          template_id: "alice/original",
          revision: 2,
          published_at: "2026-10-29T00:00:00Z",
        },
      ],
    };
    renderHubDetailPane("template", {
      selectedTemplate: localTemplate,
      templates: [localTemplate],
      templateUsername: "alice",
      agenticHubSiteURL: "https://hub.example",
      inlineTemplateDetail: true,
    });
    expect(screen.getByRole("link", { name: "Last published V2" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish V3 to community" })).toBeEnabled();
    expect(screen.queryByText("Current version published")).not.toBeInTheDocument();
  });

  it("switches versions in place without invoking template navigation or opening a detail dialog", async () => {
    const user = userEvent.setup();
    const localTemplate: HubTemplate = {
      ...template,
      id: "local.demo-template",
      revision: 2,
      latest_revision: 2,
      source: { kind: "local" },
    };
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const onSelectTemplate = vi.fn();
    const restored = { ...localTemplate, revision: 1 };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const url = String(input);
        if (url.endsWith("/restore")) return Response.json(restored);
        if (url.endsWith("/versions"))
          return Response.json([
            { revision: 1, created_at: "2026-10-29T00:00:00Z" },
            { revision: 2, created_at: "2026-10-29T01:00:00Z" },
          ]);
        return Response.json({ kind: "dir", path: "", entries: [] });
      }),
    );
    const view = renderHubDetailPane("template", {
      selectedTemplate: localTemplate,
      templates: [localTemplate],
      inlineTemplateDetail: true,
      onSelectTemplate,
      queryClient: client,
    });
    const details = view.container.querySelector("details")!;
    details.open = true;
    fireEvent(details, new Event("toggle"));
    await user.click(await screen.findByRole("button", { name: "V1" }));
    await user.click(screen.getByRole("button", { name: "templateRestoreVersion" }));
    await user.click(screen.getByRole("button", { name: "templateVersionRollbackConfirm" }));
    await waitFor(() =>
      expect(client.getQueryData(workspaceQueryKeys.hubTemplate(localTemplate.id))).toEqual(restored),
    );
    expect(onSelectTemplate).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    view.rerenderOptions({ selectedTemplate: restored });
    expect(screen.getAllByText("templateVersionActive V1").length).toBeGreaterThan(0);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("publishes a local template to the community", async () => {
    const user = userEvent.setup();
    const onPublishTemplate = vi.fn().mockResolvedValue({ status: "success" });
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      revision: 3,
      version: "V3",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    const view = renderHubDetailPane("template", {
      selectedTemplate: localTemplate,
      onPublishTemplate,
      templateUsername: "alice",
      agenticHubSiteURL: "https://qa.opencsg.com/account",
    });

    await openTemplateDetail(user);
    await user.click(screen.getByRole("button", { name: "Publish V3 to community" }));
    expect(screen.getByRole("dialog", { name: "Publish agent template" })).toBeInTheDocument();
    expect(screen.getByText("Version to publish")).toBeInTheDocument();
    expect(screen.getByText("V3", { selector: "strong" })).toBeInTheDocument();
    expect(
      screen.getByText(
        "Only the active version is published; history is not included. The cloud keeps one current copy.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish V3 to community" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Publish and deploy" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Publish V3 to community" }));

    expect(onPublishTemplate).toHaveBeenCalledWith(localTemplate, false);
    expect(await screen.findByRole("dialog", { name: "Published successfully" })).toBeInTheDocument();
    expect(screen.getByText("The template has been published to the community.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "OK" }));
    expect(screen.queryByRole("dialog", { name: "Published successfully" })).not.toBeInTheDocument();
    view.rerenderOptions({
      templates: [
        localTemplate,
        { ...localTemplate, id: "alice/demo-template", namespace: "alice", source: { kind: "remote" } },
      ],
    });
    expect(screen.getByRole("link", { name: "View community template" })).toHaveAttribute(
      "href",
      "https://qa.opencsg.com/agentichub?view=resources&subview=template",
    );
    expect(screen.getByRole("button", { name: "Publish V3 to community" })).toBeEnabled();
    expect(screen.getAllByText("Community version unconfirmed").length).toBeGreaterThan(0);
    view.rerenderOptions({ templates: [localTemplate] });
    expect(screen.queryByRole("link", { name: "View community template" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish V3 to community" })).toBeEnabled();
    expect(screen.queryByText("Community version unconfirmed")).not.toBeInTheDocument();
  });

  it("publishes a local DSH template to the community", async () => {
    const user = userEvent.setup();
    const onPublishTemplate = vi.fn().mockResolvedValue({ status: "success" });
    const localTemplate = {
      ...template,
      id: "local.dsh-template",
      runtime_kind: "dsh",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "dsh" },
    };
    renderHubDetailPane("template", { selectedTemplate: localTemplate, onPublishTemplate });

    await openTemplateDetail(user);
    await user.click(screen.getByRole("button", { name: "Publish to community" }));
    expect(screen.getByRole("button", { name: "Publish template only" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Publish and deploy" })).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Include agent memory" })).not.toBeChecked();
    await user.click(screen.getByRole("checkbox", { name: "Include agent memory" }));
    await user.click(screen.getByRole("button", { name: "Publish template only" }));

    expect(onPublishTemplate).toHaveBeenCalledWith(localTemplate, true);
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

    await openTemplateDetail(user);
    await user.click(screen.getByRole("button", { name: "Publish to community" }));
    await user.click(screen.getByRole("button", { name: "Publish template only" }));

    expect(screen.queryByRole("dialog", { name: "Published successfully" })).not.toBeInTheDocument();
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

    await openTemplateDetail(user);
    await user.click(screen.getByRole("button", { name: "Publish to community" }));

    expect(screen.getByRole("dialog", { name: "Publish agent template" })).toBeInTheDocument();
    expect(screen.getByText("Template was published, but deployment failed.")).toBeInTheDocument();
  });

  it.each([false, true])(
    "resets previous publishing errors when opening the publish dialog (inline: %s)",
    async (inlineTemplateDetail) => {
      const reset = vi.fn();
      renderHubDetailPane("template", {
        selectedTemplate: { ...template, id: "local.new-template", runtime_kind: "codex", source: { kind: "local" } },
        inlineTemplateDetail,
        onResetPublishError: reset,
      });
      const user = userEvent.setup();
      if (!inlineTemplateDetail) await openTemplateDetail(user);
      await user.click(screen.getByRole("button", { name: "Publish to community" }));
      expect(reset).toHaveBeenCalledOnce();
      await user.click(screen.getByRole("button", { name: "Cancel" }));
      await user.click(screen.getByRole("button", { name: "Publish to community" }));
      expect(reset).toHaveBeenCalledTimes(2);
    },
  );

  it("requires sign-in before publishing a local template to the community", async () => {
    const localTemplate = {
      ...template,
      id: "local.demo-template",
      runtime_kind: "codex",
      source: { name: "local", kind: "local" },
      workspace: { ...template.workspace, kind: "codex" },
    };
    renderHubDetailPane("template", { selectedTemplate: localTemplate, publishDisabled: true });

    await openTemplateDetail(userEvent.setup());
    const publish = screen.getByRole("button", { name: "Publish to community" });
    expect(publish).toBeDisabled();
    expect(publish).toHaveAttribute("title", "Sign in first");
  });

  it("allows agent creation for remote Codex templates", async () => {
    const user = userEvent.setup();
    renderHubDetailPane("template", {
      selectedTemplate: {
        ...template,
        id: "Agentic/feishu-assistant",
        name: "feishu-assistant",
        source: { name: "official", kind: "remote" },
      },
    });

    await openTemplateDetail(user, /feishu-assistant/i);
    expect(screen.getByRole("button", { name: "Create agent" })).toBeInTheDocument();
  });

  it.each([false, true])("groups template details into working tabs (inline: %s)", async (inlineTemplateDetail) => {
    const user = userEvent.setup();
    const { container } = renderHubDetailPane("template", {
      selectedTemplate: { ...template, runtime_kind: "codex" },
      inlineTemplateDetail,
    });

    if (!inlineTemplateDetail) await openTemplateDetail(user);
    else expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
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

    await user.click(screen.getByRole("button", { name: /^Connectors/ }));
    expect(screen.getByText("context7")).toBeInTheDocument();
    expect(screen.getByText("Context lookup")).toBeInTheDocument();
  });

  it("selects the memory summary after the memories directory is lazily loaded", async () => {
    const user = userEvent.setup();
    renderHubDetailPane("template", {
      selectedTemplate: { ...template, runtime_kind: "codex" },
      lazyMemory: true,
    });

    await openTemplateDetail(user);
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

  it("presents managed MCP source check failures as an actionable alert", async () => {
    const user = userEvent.setup();
    const { onCheckMCPSource } = renderMCPDetailPane({
      managedSource: true,
      sourceError: "Source check failed",
    });

    await openMCPDetail(user, /kb-investment/i);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("resourcesKnowledgeMCPCheckFailed");
    expect(alert).toHaveTextContent("resourcesKnowledgeMCPCheckFailedHint");

    await user.click(screen.getByRole("button", { name: "resourcesMCPSourceRetry" }));
    expect(onCheckMCPSource).toHaveBeenCalledTimes(1);
  });

  it("shows template profile instructions as a readonly preview", async () => {
    const user = userEvent.setup();
    renderHubDetailPane();

    await openTemplateDetail(user);
    await user.click(screen.getByRole("button", { name: "Instructions" }));
    expect(screen.getByText("No custom instructions")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "View complete AGENTS.md" }));

    expect(screen.getByRole("button", { name: /^Skills/ })).toBeInTheDocument();
    expect(screen.getByLabelText("Instructions")).toHaveTextContent("# Instructions Follow the template rules.");
    expect(screen.getByText("Effective AGENTS.md")).toBeInTheDocument();
    expect(screen.queryByText("AGENTS.md")).not.toBeInTheDocument();
  });

  it("renders the skill list and opens selected skill details in a dialog", async () => {
    const user = userEvent.setup();
    renderHubSkillDetailPane();
    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(screen.getByRole("heading", { name: "Skills" })).toBeInTheDocument();
    expect(screen.getByRole("tablist", { name: "Skills" })).toBeInTheDocument();
    expect(screen.getAllByText("Demo skill").length).toBeGreaterThan(0);
    expect(screen.queryByText("demo-template")).not.toBeInTheDocument();
    expect(screen.queryByText("# Skill", { exact: false })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /demo-skill/i }));

    expect(screen.getAllByRole("dialog").at(-1)!).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "demo-skill" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete skill" })).toBeInTheDocument();
    expect(screen.getByText("# Skill", { exact: false })).toBeInTheDocument();
    expect(screen.queryByText("Description")).not.toBeInTheDocument();
  });

  it("requires confirmation before deleting a skill from its detail dialog", async () => {
    const user = userEvent.setup();
    const onDeleteSkill = vi.fn().mockResolvedValue(true);
    renderHubSkillDetailPane(onDeleteSkill);

    expect(screen.getAllByRole("dialog").at(-1)!).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete skill" })).toBeInTheDocument();
    expect(screen.queryByText('Delete skill "demo-skill"? This action cannot be undone.')).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Delete skill" }));
    expect(screen.getByText('Delete skill "demo-skill"? This action cannot be undone.')).toBeInTheDocument();
    expect(onDeleteSkill).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(onDeleteSkill).toHaveBeenCalledWith(expect.objectContaining({ name: "demo-skill" }));
  });

  it("highlights and validates MCP JSON configs before saving", async () => {
    const user = userEvent.setup();
    const { onUpdateMCP } = renderMCPDetailPane();

    await openMCPDetail(user);
    const editor = await screen.findByRole("textbox", { name: "MCP server definition" });
    expect(document.querySelector(".cm-editor")).toBeInTheDocument();
    expect(document.body.textContent).toContain("mcpServers");
    expect(document.body.textContent).toContain("grafana-mcp");

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

    await openMCPDetail(user);
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

    await openMCPDetail(user, /kb-investment/i);
    expect(screen.getByText("resourcesKnowledgeMCPBadge")).toBeInTheDocument();
    expect(screen.getByText("resourcesKnowledgeMCPUpdateAvailable")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "resourcesMCPSourceRetry" })).not.toBeInTheDocument();
    expect(onCheckMCPSource).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "resourcesMCPSourceUpdate" }));
    expect(onSyncMCPSource).toHaveBeenCalledTimes(1);
  });

  it("keeps a healthy knowledge base MCP status out of the way", async () => {
    const user = userEvent.setup();
    renderMCPDetailPane({ managedSource: true });

    await openMCPDetail(user, /kb-investment/i);
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

    expect(screen.getAllByRole("dialog").at(-1)!).toHaveTextContent("mcp server already exists: filesystem");
    expect(screen.getAllByText("mcp server already exists: filesystem")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Add" })).toBeInTheDocument();
  });

  it("installs a remote MCP through the Hub install flow", async () => {
    const user = userEvent.setup();
    const { onInstallRemoteMCP, onRemoteMCPVisibleChange } = renderMCPCreateDialog();

    await user.click(screen.getByRole("tab", { name: "Remote install" }));

    expect(onRemoteMCPVisibleChange).toHaveBeenCalledWith(true);
    await user.click(screen.getByRole("tab", { name: "Remote install" }));
    expect(onRemoteMCPVisibleChange.mock.calls.filter(([visible]) => visible)).toHaveLength(2);
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

describe("Unified connectors", () => {
  it("keeps all three creation entries available with an empty catalog and opens remote install directly", async () => {
    const user = userEvent.setup();
    const { onRemoteMCPVisibleChange } = renderMCPCreateDialog(false);
    expect(screen.getByRole("heading", { name: "connectorsApplications" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "connectorsTools" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "connectorsAdd" }));
    const menu = screen.getByRole("menu");
    expect(within(menu).getAllByRole("menuitem")).toHaveLength(3);
    await user.click(within(menu).getByRole("menuitem", { name: /Remote install/ }));
    expect(screen.getByRole("tab", { name: "Remote install" })).toHaveAttribute("aria-selected", "true");
    expect(onRemoteMCPVisibleChange).toHaveBeenCalledWith(true);
    expect(screen.getByText("Calendar tools")).toBeVisible();
  });

  it("searches applications and MCP tools together while transport filters affect only tools", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) =>
        Response.json({
          items: String(url).endsWith("catalog")
            ? [{ app_id: "gitlab", name: "GitLab", interface: {}, config_schema: {}, description: "Projects" }]
            : [
                {
                  installation_id: "work",
                  app_id: "gitlab",
                  name: "Work projects",
                  enabled: true,
                  status: "configured",
                  config: {},
                  credentials_set: {},
                  tools: [],
                },
              ],
        }),
      ),
    );
    const user = userEvent.setup();
    renderMCPDetailPane();
    await user.click(screen.getByRole("button", { name: "Close" }));
    const app = await screen.findByRole("button", { name: /^Work projects/ });
    expect(app).toBeVisible();
    await user.click(screen.getByRole("tab", { name: "Remote" }));
    expect(app).toBeVisible();
    expect(screen.queryByRole("button", { name: /grafana.*Grafana/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "All" }));
    const search = screen.getByRole("searchbox", { name: "connectorsSearch" });
    await user.type(search, "Work projects");
    expect(app).toBeVisible();
    expect(screen.queryByRole("button", { name: /grafana.*Grafana/ })).not.toBeInTheDocument();
    await user.clear(search);
    await user.type(search, "grafana");
    expect(screen.queryByRole("button", { name: /^Work projects/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /grafana/i })).toBeVisible();
  });
});
