import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { AgentProfileModal } from "./AgentProfileModal";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";
import { agentToDraft, type AgentDraft } from "@/models/agents";
import * as directoryPicker from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";
import { createQueryWrapper } from "../../../../../tests/helpers/queryClient";

vi.mock("@/api/skills", () => ({ fetchSkills: vi.fn().mockResolvedValue([]) }));
vi.mock("@/api/mcp", () => ({ fetchMCPServers: vi.fn().mockResolvedValue({}) }));
vi.mock("@/api/apps", () => ({ fetchAppResources: vi.fn().mockResolvedValue([]) }));

const labels: Record<string, string> = {
  agentDescription: "Description",
  agentInstructions: "Instructions",
  agentImage: "Image",
  agentName: "Name",
  agentNamePlaceholder: "For example: dev",
  agentCreateSave: "Create",
  close: "Close",
  iaCreateAgentBrowseTemplates: "Browse agent templates",
  iaCreateAgentSwitchTemplate: "Change template",
  createAgentTemplateSectionDescription: "Selecting a template inherits runtime and default settings.",
  createAgentTemplateSectionTitle: "Template setup",
  createAgentKindNotification: "Notification bot",
  createAgentKindNotificationDescription: "Send events to an external webhook endpoint.",
  createAgentKindWorker: "Worker",
  createAgentKindWorkerDescription: "Run on your local runtime using built-in connectors.",
  editAgentSubtitle: "Change runtime settings.",
  editAgentTitle: "Edit Agent",
  profileBasics: "Basics",
  profileModelSection: "Model",
  profileModel: "Model",
  modelProviderModelSearch: "Search models",
  modelProviderNoModels: "No models",
  profileRuntimeOptions: "Runtime Options",
  profileMCPServers: "MCP Servers",
  profileMCPServersHint: 'Enter MCP servers as {"server-name": {...}}.',
  profileMCPServersPlaceholder: '{\n  "context7": {}\n}',
  profileMCPServersUseExample: "Use example",
  profileMCPServersClear: "Clear servers",
  profileMCPServersInvalidJSON: "Enter a valid JSON object.",
  profileMCPServersObjectRequired: "MCP servers must be a JSON object.",
  profileProvider: "Provider",
  profileRuntimeKind: "Runtime",
  profileEnv: "Environment",
  profileSandboxEnabled: "Sandbox",
  profileSandboxEnabledHelp:
    "When enabled, the agent runs tasks in an isolated environment. When disabled, it uses the local runtime.",
  runtimeOpenclaw: "OpenClaw",
  runtimePicoclaw: "PicoClaw",
  runtimeCodexCLI: "Codex CLI",
  runtimeSandboxUnavailable: "Current sandbox is unavailable: {reason}",
  runtimeSandboxUnavailableReason: "Check the current sandbox configuration.",
  runtimeInstallRequiredTitle: "DeepSeek Harness is required",
  runtimeInstallRequiredDescription: "Install DSH 0.1.5-rc.3 to continue creating the agent.",
  computerRuntimeInstall: "Install",
  computerRuntimeInstalling: "Installing...",
  statusEnabled: "Enabled",
  statusDisabled: "Disabled",
  templateLabel: "Template",
  templateNone: "No template",
  agentUpdateSave: "Save",
};

function t(key: string, params?: Record<string, string | number>): string {
  const value = labels[key] ?? key;
  return value.replace(/\{(\w+)\}/g, (_, name) => `${params?.[name] ?? ""}`);
}

const worker = {
  id: "worker-1",
  name: "Worker",
  role: "worker",
  runtime_kind: "picoclaw_sandbox",
  status: "running",
  provider: "api",
  model_id: "gpt-test",
  image: "worker:latest",
};

describe("AgentProfileModal", () => {
  it("defaults to the custom form without creation mode tabs", () => {
    render(
      <AgentProfileModal
        t={t}
        locale="en"
        agentModalMode="create"
        agentCreateBotKind="worker"
        agentDraft={agentToDraft(worker)}
        onAgentCreateBotKindChange={vi.fn()}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );
    expect(screen.queryByRole("tab", { name: /From template/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /Custom/ })).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText("For example: dev")).toBeInTheDocument();
  });
  it("hides failed CLI providers and keeps an existing selection read-only", async () => {
    const user = userEvent.setup();
    const draft = { ...agentToDraft(worker), provider: "codex", model_provider_id: "codex", model_id: "saved-model" };
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={draft}
        modelProviders={normalizeModelProviderCatalog({
          providers: [
            { id: "codex", display_name: "Codex", status: "failed", models: ["stale-model"] },
            { id: "claude_code", display_name: "Claude Code", status: "failed", models: ["stale-model"] },
          ],
        })}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );
    expect(screen.getByRole("combobox", { name: /^Model$/ })).toBeDisabled();
    expect(screen.getByRole("combobox", { name: /^Model$/ })).toHaveTextContent("saved-model");
    await user.click(screen.getByRole("combobox", { name: "profileModelProvider" }));
    expect(screen.queryByRole("option", { name: /Codex/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Claude Code/ })).not.toBeInTheDocument();
  });

  it("does not preselect an avatar in create mode", () => {
    const draft = { ...agentToDraft(worker), avatar: "" };

    const { container } = render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={draft}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(container.querySelectorAll(".agent-avatar-option.selected")).toHaveLength(0);
  });

  it("allows the agent name to be edited in edit mode", async () => {
    const user = userEvent.setup();
    const onAgentDraftChange = vi.fn();
    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>(agentToDraft(worker));
      return (
        <AgentProfileModal
          t={t}
          agentModalMode="edit"
          editingAgent={worker}
          agentDraft={draft}
          onAgentDraftChange={(update) => {
            onAgentDraftChange(update);
            setDraft((current) => {
              const next = typeof update === "function" ? update(current) : update;
              return next ?? current;
            });
          }}
          onAgentModelsReset={vi.fn()}
          hubTemplates={[]}
          bootstrapConfig={{}}
          managerAgent={null}
          agentModels={[]}
          agentModelBusy={false}
          locale="en"
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          onAgentCreateBotKindChange={vi.fn()}
          notifierWebhookPublicOrigin="http://127.0.0.1:18080"
          onProviderLogin={vi.fn()}
          agentError=""
          agentProgress={null}
          agentBusy={false}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />
      );
    }
    render(<TestModal />);

    const nameInput = screen.getByDisplayValue("Worker");
    expect(nameInput).not.toBeDisabled();
    expect(nameInput).not.toHaveAttribute("readonly");
    await user.type(nameInput, " QA");
    expect(onAgentDraftChange).toHaveBeenLastCalledWith(expect.objectContaining({ name: "Worker QA" }));
  });

  it("shows the instructions field for worker agents after the model section", () => {
    const { container } = render(
      <AgentProfileModal
        t={t}
        agentModalMode="edit"
        editingAgent={worker}
        agentDraft={{ ...agentToDraft(worker), instructions: "reply in Chinese" }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    const modelSectionTitle = screen.getAllByText("Model")[0];
    const instructionSectionTitle = screen.getAllByText("Instructions")[0];
    expect(instructionSectionTitle).toBeInTheDocument();
    expect(screen.getByDisplayValue("reply in Chinese")).toBeInTheDocument();
    expect(modelSectionTitle.compareDocumentPosition(instructionSectionTitle)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(container.querySelector(".profile-section .profile-section-title")?.textContent).toBe("Basics");
  });

  it("renders runtime option fields from the selected runtime schema", () => {
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          runtime_kind: "codex",
          runtime_options: { local_workspace_dir: "/tmp/project" },
        }}
        locale="zh"
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{
          runtime_option_schemas: {
            codex: [
              {
                key: "local_workspace_dir",
                path: "local_workspace_dir",
                label: "Local Workspace Dir",
                label_zh: "本地工作目录",
                label_en: "Local Workspace Dir",
                description: "Leave empty to use the default agent workspace.",
                description_zh: "留空时使用默认 Agent 工作目录。",
                description_en: "Leave empty to use the default agent workspace.",
                type: "directory",
              },
            ],
          },
        }}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByDisplayValue("/tmp/project")).toBeInTheDocument();
    expect(screen.getByText("本地工作目录")).toBeInTheDocument();
    expect(screen.getByText("留空时使用默认 Agent 工作目录。")).toBeInTheDocument();
  });

  it("shows the two DSH permission modes during agent creation", async () => {
    const user = userEvent.setup();
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          runtime_kind: "dsh",
          runtime_name: "dsh",
          sandbox_enabled: false,
          runtime_options: {},
        }}
        locale="zh"
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{
          worker_runtime_choices: [
            { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
            { name: "dsh", sandbox_enabled: false, installed: true, label: "DeepSeek Harness" },
          ],
          runtime_option_schemas: {
            dsh: [
              {
                key: "permission_mode",
                path: "permission_mode",
                label: "Permission Mode",
                label_zh: "运行模式",
                type: "select",
                options: ["workspace-write", "read-only"],
                choices: [
                  { value: "workspace-write", label_zh: "工作区内修改" },
                  { value: "read-only", label_zh: "仅可查看" },
                ],
                default_value: "workspace-write",
                presentation: "dsh_permission_mode",
              },
            ],
          },
        }}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    const permissionMode = screen.getByRole("combobox", { name: "运行模式" });
    expect(permissionMode).toHaveTextContent("工作区内修改");
    await user.click(permissionMode);
    expect(screen.getByRole("option", { name: "工作区内修改" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "仅可查看" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "完全访问" })).not.toBeInTheDocument();
  });

  it("shows sandbox help text in worker create mode", () => {
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={agentToDraft(worker)}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("opens custom creation without mode tabs and offers the template library below identity fields", async () => {
    const onBrowseTemplates = vi.fn();
    render(
      <AgentProfileModal
        creationWizard
        agentInitializing
        t={t}
        agentModalMode="create"
        agentCreateBotKind="worker"
        agentDraft={agentToDraft({ ...worker, runtime_kind: "codex" })}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        onAgentCreateBotKindChange={vi.fn()}
        onBrowseTemplates={onBrowseTemplates}
        onClose={vi.fn()}
        onSave={vi.fn()}
        locale="en"
      />,
      { wrapper: createQueryWrapper().wrapper },
    );
    expect(screen.getByRole("navigation", { name: "createAgentWizardTitle" })).toBeInTheDocument();
    expect(screen.queryByLabelText("MCP Servers")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("agentWizardLoadingConfiguration");
    expect(screen.getByRole("textbox", { name: "Name" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "agentWizardNext" })).toBeDisabled();
    expect(screen.queryByRole("tablist", { name: "createAgentModeTabAriaLabel" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Template" })).not.toBeInTheDocument();
    const identity = screen.getByRole("dialog").querySelector(".agent-identity-layout");
    const browse = screen.getByRole("button", { name: "Browse agent templates" });
    expect(identity?.compareDocumentPosition(browse)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    await userEvent.setup().click(browse);
    expect(onBrowseTemplates).toHaveBeenCalledOnce();
  });

  it("shows the selected template separately with editable identity and a change-template exit", async () => {
    const onBrowseTemplates = vi.fn();
    const onAgentDraftChange = vi.fn();
    render(
      <AgentProfileModal
        creationWizard
        t={t}
        agentModalMode="create"
        agentCreateBotKind="worker"
        agentCreateMode="template"
        agentDraft={agentToDraft({
          ...worker,
          runtime_kind: "codex",
          from_template: "local.reviewer",
          template_name: "Reviewer",
        })}
        hubTemplates={[
          { id: "local.reviewer", name: "Reviewer", description: "Reviews project changes.", runtime_kind: "codex" },
          { id: "local.writer", name: "Writer", runtime_kind: "codex" },
        ]}
        onAgentDraftChange={onAgentDraftChange}
        onAgentModelsReset={vi.fn()}
        onAgentCreateBotKindChange={vi.fn()}
        onBrowseTemplates={onBrowseTemplates}
        onClose={vi.fn()}
        onSave={vi.fn()}
        locale="en"
      />,
    );
    expect(screen.queryByRole("navigation", { name: "createAgentWizardTitle" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Template" })).not.toBeInTheDocument();
    expect(screen.getByText("Reviewer")).toBeInTheDocument();
    expect(screen.getByText("Reviews project changes.")).toBeInTheDocument();
    expect(screen.queryByText("Writer")).not.toBeInTheDocument();
    fireEvent.input(screen.getByRole("textbox", { name: "Name" }), { target: { value: "My reviewer" } });
    expect(onAgentDraftChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ name: "My reviewer", from_template: "local.reviewer" }),
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "Change template" }));
    expect(onBrowseTemplates).toHaveBeenCalledOnce();
  });

  it("warns when the selected sandbox runtime is unavailable", () => {
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          from_template: "builtin.picoclaw-worker",
          template_name: "PicoClaw Worker",
          runtime_kind: "picoclaw_sandbox",
          runtime_name: "picoclaw",
          sandbox_enabled: true,
          model_provider_id: "provider-1",
          model_id: "gpt-test",
        }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[
          {
            id: "builtin.picoclaw-worker",
            name: "PicoClaw Worker",
            role: "worker",
            runtime_kind: "picoclaw_sandbox",
            description: "Sandbox template.",
          },
        ]}
        bootstrapConfig={{
          worker_runtime_choices: [
            { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
            {
              name: "picoclaw",
              sandbox_enabled: true,
              installed: false,
              label: "PicoClaw",
              message: "boxlite missing",
            },
          ],
        }}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="template"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByText("Current sandbox is unavailable: boxlite missing")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
  });

  it("hides env inputs in template mode when the template has no image env", () => {
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          from_template: "builtin.picoclaw-worker",
          template_name: "Worker",
          name: "Worker",
        }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[
          {
            id: "builtin.picoclaw-worker",
            name: "Worker",
            role: "worker",
            runtime_kind: "picoclaw_sandbox",
            description: "Handles coding tasks.",
          },
        ]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="template"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.queryByText("Environment")).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("profileEnvKey")).not.toBeInTheDocument();
  });

  it("requires required template env values before creating", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();

    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        from_template: "custom/gitlab",
        template_name: "GitLab Assistant",
        name: "gitlab-assistant",
        model_provider_id: "codex",
        model_id: "gpt-5.5",
        envRows: [{ key: "GITLAB_TOKEN", required: true, value: "" }],
      });
      return (
        <AgentProfileModal
          t={t}
          agentModalMode="create"
          editingAgent={null}
          agentDraft={draft}
          onAgentDraftChange={(update) =>
            setDraft((current) => {
              const next = typeof update === "function" ? update(current) : update;
              return next ?? current;
            })
          }
          onAgentModelsReset={vi.fn()}
          hubTemplates={[
            {
              id: "custom/gitlab",
              name: "GitLab Assistant",
              role: "worker",
              runtime_kind: "codex",
              image_env: [{ name: "GITLAB_TOKEN", required: true, secret: true }],
            },
          ]}
          bootstrapConfig={{}}
          managerAgent={null}
          agentModels={["gpt-5.5"]}
          agentModelBusy={false}
          locale="en"
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          agentCreateMode="template"
          onAgentCreateBotKindChange={vi.fn()}
          notifierWebhookPublicOrigin="http://127.0.0.1:18080"
          onProviderLogin={vi.fn()}
          agentError=""
          agentProgress={null}
          agentBusy={false}
          onClose={vi.fn()}
          onSave={onSave}
        />
      );
    }

    const { container } = render(<TestModal />);
    const createButton = screen.getByRole("button", { name: "Create" });

    expect(container.querySelector(".env-required-star")).toHaveTextContent("*");
    expect(createButton).toBeDisabled();

    await user.type(screen.getByPlaceholderText("profileEnvValue"), "token");

    expect(createButton).toBeEnabled();
  });

  it("allows toggling sandbox in custom mode and exposes only OpenClaw sandbox runtime", async () => {
    const user = userEvent.setup();

    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        sandbox_enabled: false,
        runtime_name: "codex",
        runtime_kind: "codex",
      });
      return (
        <AgentProfileModal
          t={t}
          agentModalMode="create"
          editingAgent={null}
          agentDraft={draft}
          onAgentDraftChange={(update) =>
            setDraft((current) => {
              const next = typeof update === "function" ? update(current) : update;
              return next ?? current;
            })
          }
          onAgentModelsReset={vi.fn()}
          hubTemplates={[]}
          bootstrapConfig={{
            worker_runtime_choices: [
              { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
              { name: "openclaw", sandbox_enabled: true, installed: true, label: "OpenClaw" },
              { name: "picoclaw", sandbox_enabled: true, installed: true, label: "PicoClaw" },
            ],
          }}
          managerAgent={null}
          agentModels={[]}
          agentModelBusy={false}
          locale="en"
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          agentCreateMode="custom"
          onAgentCreateBotKindChange={vi.fn()}
          notifierWebhookPublicOrigin="http://127.0.0.1:18080"
          onProviderLogin={vi.fn()}
          agentError=""
          agentProgress={null}
          agentBusy={false}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />
      );
    }

    render(<TestModal />);

    const toggle = screen.getByRole("checkbox", { name: "Sandbox" });
    expect(toggle).not.toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Runtime" })).toHaveTextContent("Codex CLI");

    await user.click(toggle);

    expect(screen.getByText("Enabled")).toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "Runtime" }));
    expect(screen.getByRole("option", { name: "OpenClaw" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "PicoClaw" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Codex CLI" })).not.toBeInTheDocument();
  });

  it("hides sandbox controls and exposes host runtimes for CSGHub custom creation", async () => {
    const user = userEvent.setup();
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          sandbox_enabled: true,
          runtime_name: "openclaw",
          runtime_kind: "openclaw_sandbox",
        }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{
          sandbox_provider: "csghub",
          worker_runtime_choices: [
            { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
            { name: "dsh", sandbox_enabled: false, installed: true, label: "DeepSeek Harness" },
          ],
        }}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.queryByRole("checkbox", { name: "Sandbox" })).not.toBeInTheDocument();
    const runtime = screen.getByRole("combobox", { name: "Runtime" });
    expect(runtime).toHaveTextContent("Codex CLI");
    await user.click(runtime);
    expect(screen.getByRole("option", { name: "DeepSeek Harness" })).toBeInTheDocument();
  });

  it("keeps the DSH option compact and shows installation below the runtime field", async () => {
    const user = userEvent.setup();
    const onInstallRuntime = vi.fn();
    const { container } = render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          sandbox_enabled: false,
          runtime_name: "dsh",
          runtime_kind: "dsh",
        }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{
          worker_runtime_choices: [
            { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
            {
              name: "dsh",
              sandbox_enabled: false,
              installed: false,
              installable: true,
              label: "DeepSeek Harness",
              message: "DSH CLI is unavailable; run a very long npm command",
              message_code: "dsh_not_installed",
            },
          ],
        }}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onInstallRuntime={onInstallRuntime}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Runtime" }));
    expect(screen.getByRole("option", { name: "DeepSeek Harness" })).toBeInTheDocument();
    expect(screen.queryByText(/very long npm command/)).not.toBeInTheDocument();
    const notice = container.querySelector(".agent-runtime-install-notice");
    expect(notice).not.toBeNull();
    expect(notice?.parentElement).toHaveClass("agent-runtime-image-row");
    expect(screen.getByText("DeepSeek Harness is required")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Install" }));
    expect(onInstallRuntime).toHaveBeenCalledWith("dsh");
  });

  it("uses the matching worker template image when switching blank drafts to OpenClaw", async () => {
    const user = userEvent.setup();
    const onAgentDraftChange = vi.fn();
    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        from_template: "",
        image: "picoclaw:current",
      });
      return (
        <AgentProfileModal
          t={t}
          agentModalMode="create"
          editingAgent={null}
          agentDraft={draft}
          onAgentDraftChange={(update) => {
            onAgentDraftChange(update);
            setDraft((current) => {
              const next = typeof update === "function" ? update(current) : update;
              return next ?? current;
            });
          }}
          onAgentModelsReset={vi.fn()}
          hubTemplates={[
            {
              id: "builtin.picoclaw-worker",
              name: "PicoClaw Worker",
              role: "worker",
              runtime_kind: "picoclaw_sandbox",
              image: "picoclaw:worker",
            },
            {
              id: "builtin.openclaw-worker",
              name: "OpenClaw Worker",
              role: "worker",
              runtime_kind: "openclaw_sandbox",
              image: "openclaw:worker",
            },
          ]}
          bootstrapConfig={{
            default_worker_template: "builtin.picoclaw-worker",
            runtime_default_images: {
              picoclaw_sandbox: "picoclaw:worker",
            },
            runtime_kind: "picoclaw_sandbox",
            worker_runtime_choices: [
              { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
              { name: "openclaw", sandbox_enabled: true, installed: true, label: "OpenClaw" },
              { name: "picoclaw", sandbox_enabled: true, installed: true, label: "PicoClaw" },
            ],
          }}
          managerAgent={{ ...worker, image: "picoclaw:manager" }}
          agentModels={[]}
          agentModelBusy={false}
          locale="en"
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          agentCreateMode="custom"
          onAgentCreateBotKindChange={vi.fn()}
          notifierWebhookPublicOrigin="http://127.0.0.1:18080"
          onProviderLogin={vi.fn()}
          agentError=""
          agentProgress={null}
          agentBusy={false}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />
      );
    }
    render(<TestModal />);

    await user.click(screen.getByRole("combobox", { name: "Runtime" }));
    await user.click(screen.getByRole("option", { name: "OpenClaw" }));

    expect(onAgentDraftChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        from_template: "",
        image: "openclaw:worker",
        runtime_kind: "openclaw_sandbox",
        runtime_name: "openclaw",
        sandbox_enabled: true,
      }),
    );
  });

  it("edits MCP servers for supported agent drafts", async () => {
    const user = userEvent.setup();
    const onAgentDraftChange = vi.fn();
    const openclawDraft = {
      ...agentToDraft({ ...worker, runtime_kind: "openclaw_sandbox" }),
      model_provider_id: "api",
      runtime_kind: "openclaw_sandbox",
      runtime_options: {
        local_workspace_dir: "/tmp/project",
      },
      mcpServers: {
        existing: {
          command: "node",
        },
      },
    };

    const { rerender } = render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={openclawDraft}
        locale="en"
        onAgentDraftChange={onAgentDraftChange}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    const editor = screen.getByLabelText("MCP Servers");
    expect((editor as HTMLTextAreaElement).value).toContain('"existing"');

    fireEvent.input(editor, {
      target: {
        value: '{"context7":{"command":"npx"}}',
      },
    });

    expect(onAgentDraftChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        runtime_options: {
          local_workspace_dir: "/tmp/project",
        },
        mcpServers: {
          context7: {
            command: "npx",
          },
        },
      }),
    );

    await user.click(screen.getByRole("button", { name: "Clear servers" }));

    expect(onAgentDraftChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        runtime_options: {
          local_workspace_dir: "/tmp/project",
        },
        mcpServers: null,
      }),
    );

    rerender(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...openclawDraft,
          mcpServers: {},
          runtime_kind: "picoclaw_sandbox",
        }}
        locale="en"
        onAgentDraftChange={onAgentDraftChange}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("MCP Servers")).toBeInTheDocument();
    expect(screen.getByLabelText("MCP Servers")).toHaveValue("{}");
  });

  it("preserves MCP servers when switching from OpenClaw to Codex", async () => {
    const user = userEvent.setup();
    const onAgentDraftChange = vi.fn();
    const openclawDraft: AgentDraft = {
      ...agentToDraft({ ...worker, runtime_kind: "openclaw_sandbox" }),
      from_template: "",
      model_provider_id: "api",
      runtime_name: "openclaw",
      runtime_kind: "openclaw_sandbox",
      sandbox_enabled: true,
      runtime_options: {
        local_workspace_dir: "/tmp/project",
      },
      mcpServers: {
        context7: {
          command: "npx",
        },
      },
    };

    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>(openclawDraft);
      return (
        <AgentProfileModal
          t={t}
          agentModalMode="create"
          editingAgent={null}
          agentDraft={draft}
          locale="en"
          onAgentDraftChange={(update) => {
            onAgentDraftChange(update);
            setDraft((current) => {
              const next = typeof update === "function" ? update(current) : update;
              return next ?? current;
            });
          }}
          onAgentModelsReset={vi.fn()}
          hubTemplates={[]}
          bootstrapConfig={{
            worker_runtime_choices: [
              { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
              { name: "openclaw", sandbox_enabled: true, installed: true, label: "OpenClaw" },
              { name: "picoclaw", sandbox_enabled: true, installed: true, label: "PicoClaw" },
            ],
          }}
          managerAgent={null}
          agentModels={[]}
          agentModelBusy={false}
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          agentCreateMode="custom"
          onAgentCreateBotKindChange={vi.fn()}
          notifierWebhookPublicOrigin="http://127.0.0.1:18080"
          onProviderLogin={vi.fn()}
          agentError=""
          agentProgress={null}
          agentBusy={false}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />
      );
    }

    render(<TestModal />);

    expect(screen.getByLabelText("MCP Servers")).toBeInTheDocument();

    await user.click(screen.getByRole("checkbox", { name: "Sandbox" }));

    const nextDraft = onAgentDraftChange.mock.calls.at(-1)?.[0] as AgentDraft;
    expect(nextDraft.sandbox_enabled).toBe(false);
    expect(nextDraft.runtime_name).toBe("codex");
    expect(nextDraft.runtime_kind).toBe("codex");
    expect(nextDraft.runtime_options).toEqual({ local_workspace_dir: "/tmp/project" });
    expect(nextDraft.mcpServers).toEqual({
      context7: {
        command: "npx",
      },
    });
    expect(screen.getByLabelText("MCP Servers")).toBeInTheDocument();
  });

  it("shows provider details once in the form and includes them in searchable model choices", async () => {
    const user = userEvent.setup();
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="edit"
        editingAgent={worker}
        agentDraft={{
          ...agentToDraft(worker),
          model_provider_id: "opencsg-aigateway",
          model_id: "MiniMax-M2.5",
        }}
        modelProviders={{
          providers: [
            {
              id: "opencsg-aigateway",
              kind: "openai_compatible",
              preset: "custom",
              display_name: "OpenCSG AIGateway",
              builtin: false,
              base_url: "https://aigateway.opencsg.com/v1",
              api_key_set: true,
              models: ["MiniMax-M2.5"],
              status: "connected",
            },
          ],
          builtinProviders: [],
          customProviders: [
            {
              id: "opencsg-aigateway",
              kind: "openai_compatible",
              preset: "custom",
              display_name: "OpenCSG AIGateway",
              builtin: false,
              base_url: "https://aigateway.opencsg.com/v1",
              api_key_set: true,
              models: ["MiniMax-M2.5"],
              status: "connected",
            },
          ],
        }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    const provider = screen.getByRole("combobox", { name: "profileModelProvider" });
    const model = screen.getByRole("combobox", { name: "Model" });
    expect(provider).toHaveTextContent("OpenCSG AIGateway");
    expect(model).toHaveTextContent("MiniMax-M2.5");
    expect(model).not.toHaveTextContent("OpenCSG AIGateway");
    await user.click(model);
    expect(screen.getByRole("option", { name: /MiniMax-M2.5.*OpenCSG AIGateway/ })).toBeInTheDocument();
  });

  it("filters model choices inside the agent model selector", async () => {
    const user = userEvent.setup();

    render(
      <AgentProfileModal
        t={t}
        agentModalMode="edit"
        editingAgent={worker}
        agentDraft={{
          ...agentToDraft(worker),
          model_provider_id: "opencsg-aigateway",
          model_id: "MiniMax-M2.5",
        }}
        modelProviders={{
          providers: [
            {
              id: "opencsg-aigateway",
              kind: "openai_compatible",
              preset: "custom",
              display_name: "OpenCSG AIGateway",
              builtin: false,
              base_url: "https://aigateway.opencsg.com/v1",
              api_key_set: true,
              models: ["MiniMax-M2.5", "Qwen3-235B", "gpt-5-high"],
              status: "connected",
            },
          ],
          builtinProviders: [],
          customProviders: [
            {
              id: "opencsg-aigateway",
              kind: "openai_compatible",
              preset: "custom",
              display_name: "OpenCSG AIGateway",
              builtin: false,
              base_url: "https://aigateway.opencsg.com/v1",
              api_key_set: true,
              models: ["MiniMax-M2.5", "Qwen3-235B", "gpt-5-high"],
              status: "connected",
            },
          ],
        }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Model" }));
    const searchInput = screen.getByRole("combobox", { name: "Search models" });
    await user.type(searchInput, "qwen");

    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue("qwen");
    expect(screen.getByRole("option", { name: /Qwen3-235B/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /MiniMax-M2.5/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /gpt-5-high/ })).not.toBeInTheDocument();

    await user.keyboard("{Backspace}{Backspace}{Backspace}{Backspace}");

    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue("");
    expect(screen.getByRole("option", { name: /MiniMax-M2.5/ })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /Qwen3-235B/ })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /gpt-5-high/ })).toBeInTheDocument();
  });

  it("allows choosing a directory runtime option path", async () => {
    const user = userEvent.setup();
    const onAgentDraftChange = vi.fn();
    vi.spyOn(directoryPicker, "pickLocalDirectoryPath").mockResolvedValue("/tmp/selected");

    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          runtime_kind: "codex",
          runtime_options: { local_workspace_dir: "" },
        }}
        locale="zh"
        onAgentDraftChange={onAgentDraftChange}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{
          runtime_option_schemas: {
            codex: [
              {
                key: "local_workspace_dir",
                path: "local_workspace_dir",
                label: "Local Workspace Dir",
                label_zh: "本地工作目录",
                label_en: "Local Workspace Dir",
                description: "Leave empty to use the default agent workspace.",
                description_zh: "留空时使用默认 Agent 工作目录。",
                description_en: "Leave empty to use the default agent workspace.",
                type: "directory",
              },
            ],
          },
        }}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "选择目录" }));

    expect(onAgentDraftChange).toHaveBeenCalledWith(
      expect.objectContaining({
        runtime_options: expect.objectContaining({
          local_workspace_dir: "/tmp/selected",
        }),
      }),
    );
  });

  it("allows clearing a directory runtime option path", async () => {
    const user = userEvent.setup();
    const onAgentDraftChange = vi.fn();

    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{
          ...agentToDraft(worker),
          runtime_kind: "codex",
          runtime_options: { local_workspace_dir: "/tmp/project" },
        }}
        locale="zh"
        onAgentDraftChange={onAgentDraftChange}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{
          runtime_option_schemas: {
            codex: [
              {
                key: "local_workspace_dir",
                path: "local_workspace_dir",
                label: "Local Workspace Dir",
                label_zh: "本地工作目录",
                label_en: "Local Workspace Dir",
                description: "Leave empty to use the default agent workspace.",
                description_zh: "留空时使用默认 Agent 工作目录。",
                description_en: "Leave empty to use the default agent workspace.",
                type: "directory",
              },
            ],
          },
        }}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "清空" }));

    expect(onAgentDraftChange).toHaveBeenCalledWith(
      expect.objectContaining({
        runtime_options: expect.objectContaining({
          local_workspace_dir: "",
        }),
      }),
    );
  });

  it("keeps avatar, name, and description together without create-mode tabs", () => {
    const { container } = render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{ ...agentToDraft(worker), avatar: "", description: "Research and report findings." }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="worker"
        agentCreateMode="custom"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    const identityLayout = container.querySelector(".agent-identity-layout");
    expect(identityLayout).toBeInTheDocument();
    expect(identityLayout?.querySelector(".agent-avatar-field .agent-avatar-picker")).toBeInTheDocument();
    expect(identityLayout?.querySelector(".agent-name-field")).toBeInTheDocument();
    expect(identityLayout?.querySelector(".agent-description-field")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Worker")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Research and report findings.")).toBeInTheDocument();
    expect(container.querySelector(".agent-create-kind-tabbar")).not.toBeInTheDocument();
  });

  it("flattens notification bot fields without basics or notifications section titles", () => {
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={{ ...agentToDraft(worker), bot_type: "notification", notifier_delivery_mode: "webhook" }}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[]}
        bootstrapConfig={{}}
        managerAgent={null}
        agentModels={[]}
        agentModelBusy={false}
        locale="en"
        authStatuses={{}}
        authBusyProvider=""
        agentCreateBotKind="notification"
        onAgentCreateBotKindChange={vi.fn()}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onProviderLogin={vi.fn()}
        agentError=""
        agentProgress={null}
        agentBusy={false}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.queryByText("Basics")).not.toBeInTheDocument();
    expect(screen.queryByText("profileNotifierSection")).not.toBeInTheDocument();
    expect(screen.getByText("notifierDeliveryMode")).toBeInTheDocument();
  });
});

it.each(["template", "custom"] as const)(
  "requires an explicit model choice for %s creation without defaults",
  async (mode) => {
    const user = userEvent.setup();
    const save = vi.fn();
    function Harness() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft({ name: "New worker", runtime_kind: "codex" }),
        from_template: mode === "template" ? "builtin.codex-worker" : "",
      });
      return (
        <AgentProfileModal
          t={t}
          agentModalMode="create"
          agentCreateMode={mode}
          agentCreateBotKind="worker"
          onAgentCreateBotKindChange={vi.fn()}
          editingAgent={null}
          agentDraft={draft}
          onAgentDraftChange={(update) =>
            setDraft((current) => (typeof update === "function" ? update(current) : update) ?? current)
          }
          onAgentModelsReset={vi.fn()}
          hubTemplates={[{ id: "builtin.codex-worker", name: "New worker", role: "worker", runtime_kind: "codex" }]}
          modelProviders={normalizeModelProviderCatalog({
            providers: [{ id: "fixture", display_name: "Fixture", status: "connected", models: ["first", "chosen"] }],
          })}
          bootstrapConfig={{ worker_runtime_choices: [{ name: "codex", sandbox_enabled: false, installed: true }] }}
          locale="en"
          onClose={vi.fn()}
          onSave={() => save(draft)}
        />
      );
    }
    render(<Harness />);
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
    const provider = screen.getByRole("combobox", { name: "profileModelProvider" });
    expect(provider).toHaveTextContent("profileProviderSelect");
    expect(screen.getByRole("combobox", { name: "Model" })).toHaveTextContent("profileProviderSelectFirst");
    await user.click(provider);
    await user.click(screen.getByRole("option", { name: /Fixture/ }));
    await user.click(screen.getByRole("combobox", { name: "Model" }));
    await user.click(screen.getByRole("option", { name: /chosen/ }));
    expect(screen.getByRole("button", { name: "Create" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        model_provider_id: "fixture",
        model_id: "chosen",
        image_generation: null,
        video_generation: null,
      }),
    );
  },
);
