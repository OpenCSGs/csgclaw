import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { HubDetailPane } from "@/pages/HubPage/components";

function t(key: string, params: Record<string, string | number> = {}) {
  const messages: Record<string, string> = {
    cancel: "Cancel",
    close: "Close",
    createAgent: "Create",
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
    resourcesTemplateMCPServersTitle: "MCP Servers",
    resourcesTemplateMCPServersDescription: "Template MCP configs.",
    resourcesLoading: "Loading resources",
    resourcesMCPServerDocumentInvalid: "MCP server definition must be valid JSON.",
    resourcesMCPServerDocumentJSONLabel: "MCP server JSON",
    resourcesMCPServerDocumentLabel: "MCP server definition",
    resourcesMCPServerDocumentObjectRequired: "MCP server definition must be a JSON object.",
    resourcesMCPServerDocumentInvalidShape:
      "MCP server definition must be an mcpServers JSON object with exactly one server.",
    resourcesMCPDelete: "Delete",
    resourcesMCPDeleteConfirmMessage: 'Delete MCP server "{name}"?',
    resourcesMCPEmpty: "No MCP servers available yet.",
    resourcesMCPLoading: "Loading MCP servers",
    resourcesMCPSave: "Save",
    resourcesMCPSaving: "Saving...",
    resourcesRefresh: "Refresh templates",
    resourcesSkillsEmpty: "No skills",
    resourcesSkillsLabel: "Skills",
    resourcesRuntimeLabel: "Runtime",
    agentProfileTab: "Profile",
    agentInstructions: "Instructions",
    agentInstructionsDefaultMode: "Default",
    agentInstructionsAdvancedMode: "Advanced",
    agentInstructionsViewMode: "Instructions view",
    agentInstructionsPlaceholder: "Describe how this agent should work.",
    resourcesTemplateInstructionsDefaultHint: "Default mode shows only user-defined instructions.",
    agentProfileSkillsTab: "Skills",
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

function renderHubDetailPane(selectedResourceType: "mcp" | "skill" | "template" = "template") {
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
        mcpServers: {
          context7: {
            command: "npx",
            args: ["-y", "context7-mcp"],
            description: "Context lookup",
          },
        },
      }),
      path: "mcps/mcp.json",
      size: 120,
    },
  };
  function Harness() {
    const [selectedWorkspacePath, setSelectedWorkspacePath] = useState("instructions/AGENTS.md");
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
            onToggleWorkspaceDir: vi.fn(),
            mcpServers: [],
            selectedMCPServer: null,
            selectedMCPServerName: "",
            selectedResourceType,
            selectedSkill: null,
            selectedSkillPath: "",
            selectedTemplate: template,
            selectedTemplateId: template.id,
            selectedWorkspacePath,
            skillFile: null,
            skillFileError: "",
            skillFileLoading: false,
            skills: [],
            skillTree: null,
            skillTreeError: "",
            skillTreeLoading: false,
            templates: [template],
            workspaceFile,
            workspaceFiles,
            workspaceFileError: "",
            workspaceFileLoading: false,
            workspaceEntries: [
              { name: "instructions", path: "instructions", type: "dir", depth: 0 },
              { name: "AGENTS.md", path: "instructions/AGENTS.md", type: "file", depth: 1 },
              { name: "skills", path: "skills", type: "dir", depth: 0 },
              { name: "demo", path: "skills/demo", type: "dir", depth: 1 },
              { name: "SKILL.md", path: "skills/demo/SKILL.md", type: "file", depth: 2 },
              { name: "mcps", path: "mcps", type: "dir", depth: 0 },
              { name: "mcp.json", path: "mcps/mcp.json", type: "file", depth: 1 },
            ],
            deleteBusy: false,
            onDeleteTemplate: vi.fn(),
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

function renderMCPDetailPane() {
  const onUpdateMCP = vi.fn().mockResolvedValue(true);
  const mcp = {
    name: "grafana",
    description: "Grafana",
    config: {
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
          mcpMutationBusy: false,
          mcpMutationError: "",
          mcpStateError: "",
          mcpStateLoading: false,
          onDeleteMCP: vi.fn(),
          onUpdateMCP,
          workspaceFile: null,
          workspaceFileError: "",
          workspaceFileLoading: false,
        },
      }}
    />,
  );
  return { ...result, onUpdateMCP };
}

describe("HubDetailPane", () => {
  it("groups template details into runtime, instructions, skills, and MCP tabs", async () => {
    const user = userEvent.setup();
    renderHubDetailPane();

    expect(screen.getByRole("button", { name: "Profile" })).toHaveAttribute("aria-current", "location");
    expect(screen.getByDisplayValue("demo:latest")).toBeInTheDocument();
    expect(screen.getAllByText("Environment variables").length).toBeGreaterThan(0);
    expect(screen.getByText("GITLAB_TOKEN")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Instructions" }));
    expect(screen.getByRole("button", { name: "Profile" })).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Describe how this agent should work.")).toHaveValue("");
    expect(screen.getByText("Default mode shows only user-defined instructions.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Advanced" }));
    expect(screen.getByPlaceholderText("Describe how this agent should work.")).toHaveValue(
      "# Instructions\n\nFollow the template rules.",
    );
    await user.click(screen.getByRole("button", { name: /^Skills/ }));
    expect(screen.getByText("demo")).toBeInTheDocument();
    expect(screen.getByText("Demo template skill")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "MCP" }));
    expect(screen.getByText("context7")).toBeInTheDocument();
    expect(screen.getByText("Context lookup")).toBeInTheDocument();
  });

  it("keeps the MCP empty state visible when templates are available", () => {
    renderHubDetailPane("mcp");

    expect(screen.getByText("No MCP servers available yet.")).toBeInTheDocument();
    expect(screen.queryByText("demo-template")).not.toBeInTheDocument();
  });

  it("shows template profile instructions as a readonly profile textarea", async () => {
    const user = userEvent.setup();
    renderHubDetailPane();

    await user.click(screen.getByRole("button", { name: "Instructions" }));
    await user.click(screen.getByRole("button", { name: "Advanced" }));

    expect(screen.getByRole("button", { name: /^Skills/ })).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Describe how this agent should work.")).toBeDisabled();
    expect(screen.getByPlaceholderText("Describe how this agent should work.")).toHaveValue(
      "# Instructions\n\nFollow the template rules.",
    );
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
});
