import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HubDetailPane } from "@/pages/HubPage/components";

function t(key: string, params: Record<string, string | number> = {}) {
  const messages: Record<string, string> = {
    cancel: "Cancel",
    close: "Close",
    createAgent: "Create",
    hubDeleteSkill: "Delete skill",
    hubDeleteSkillConfirmAction: "Delete",
    hubDeleteSkillConfirmMessage: 'Delete skill "{name}"? This action cannot be undone.',
    hubAllTab: "All",
    hubEmpty: "No templates",
    hubImageLabel: "Image",
    hubLoading: "Loading Hub",
    hubRefresh: "Refresh templates",
    hubSkillsEmpty: "No skills",
    hubSkillsLabel: "Skills",
    hubRuntimeLabel: "Runtime",
    hubSourceLabel: "Source",
    hubSubtitle: "Browse templates.",
    hubTemplateCountSuffix: "Agent templates",
    hubTitle: "Hub",
    hubUpdatedAtLabel: "Updated",
    hubWorkspaceBinary: "Binary file",
    hubWorkspaceEmptyFile: "Empty file",
    hubWorkspaceLoading: "Loading workspace",
    hubWorkspacePreviewHint: "Choose a file",
    hubWorkspacePreviewTitle: "Select a file",
    hubWorkspaceTemplateLabel: "Workspace",
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

function renderHubDetailPane() {
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
          selectedResourceType: "template",
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

describe("HubDetailPane", () => {
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
});
