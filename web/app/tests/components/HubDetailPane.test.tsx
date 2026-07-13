import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
          mcpServers: [],
          selectedMCPServer: null,
          selectedMCPServerName: "",
          selectedResourceType,
          selectedSkill: null,
          selectedSkillPath: "",
          selectedTemplate: template,
          selectedTemplateId: template.id,
          selectedWorkspacePath: "README.md",
          skillFile: null,
          skillFileError: "",
          skillFileLoading: false,
          skills: [],
          skillTree: null,
          skillTreeError: "",
          skillTreeLoading: false,
          templates: [template],
          workspaceFile: {
            binary: false,
            content: "# Summary\n\nDescribe the problem.",
            path: "README.md",
            size: 33,
          },
          workspaceFileError: "",
          workspaceFileLoading: false,
          deleteBusy: false,
          onDeleteTemplate: vi.fn(),
        },
      }}
    />,
  );
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
  it("keeps the MCP empty state visible when templates are available", () => {
    renderHubDetailPane("mcp");

    expect(screen.getByText("No MCP servers available yet.")).toBeInTheDocument();
    expect(screen.queryByText("demo-template")).not.toBeInTheDocument();
  });

  it("opens markdown files in a dialog with preview and code modes", async () => {
    const user = userEvent.setup();
    renderHubDetailPane();

    expect(screen.getByText("# Summary", { exact: false })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Preview" }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Preview" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("heading", { name: "Summary" })).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "Code" }));

    expect(screen.getByRole("tab", { name: "Code" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getAllByText("# Summary", { exact: false }).length).toBeGreaterThan(0);
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
