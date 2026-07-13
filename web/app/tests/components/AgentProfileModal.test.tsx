import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { AgentProfileModal } from "@/pages/WorkspacePage/components";
import { agentToDraft, type AgentDraft } from "@/models/agents";
import * as directoryPicker from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";

const labels: Record<string, string> = {
  agentDescription: "Description",
  agentInstructions: "Instructions",
  agentImage: "Image",
  agentName: "Name",
  agentNamePlaceholder: "For example: dev",
  agentCreateSave: "Create",
  close: "Close",
  createAgentModeCustom: "Custom",
  createAgentModeCustomDescription: "Manually configure runtime, model, and instructions.",
  createAgentModeTemplate: "From template",
  createAgentModeTemplateDescription: "Pick a template to inherit runtime and defaults.",
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

  it("switches between template and custom create modes", async () => {
    const user = userEvent.setup();
    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        from_template: "builtin.openclaw-worker",
        template_name: "OpenClaw Worker",
        name: "OpenClaw Worker",
        runtime_name: "openclaw",
        sandbox_enabled: true,
        runtime_kind: "openclaw_sandbox",
      });
      const [mode, setMode] = useState<"template" | "custom">("template");
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
              id: "builtin.openclaw-worker",
              name: "OpenClaw Worker",
              role: "worker",
              runtime_kind: "openclaw_sandbox",
              description: "Handles coding tasks.",
              image_env: [{ name: "OPENAI_API_KEY", secret: true }],
            },
          ]}
          bootstrapConfig={{
            worker_runtime_choices: [
              { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
              { name: "openclaw", sandbox_enabled: true, installed: true, label: "OpenClaw" },
            ],
          }}
          managerAgent={null}
          agentModels={[]}
          agentModelBusy={false}
          locale="en"
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          agentCreateMode={mode}
          onAgentCreateModeChange={setMode}
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

    expect(screen.getByText("Template setup")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Template" })).toBeInTheDocument();
    expect(screen.getAllByText("Environment").length).toBeGreaterThan(0);
    expect(screen.getByText("Basics")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Name" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Provider" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /Custom/i }));

    expect(screen.getByRole("textbox", { name: "Name" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Runtime" })).toHaveTextContent("Codex CLI");
    expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("");
  });

  it("clears template identity fields when switching to custom mode", async () => {
    const user = userEvent.setup();
    const templateAvatar = "avatar/3D-5.png";

    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        avatar: templateAvatar,
        from_template: "builtin.openclaw-worker",
        template_name: "OpenClaw Worker",
        name: "generic-assistant-openclaw",
        description: "通用型助手（OpenClaw 版）",
        runtime_name: "openclaw",
        sandbox_enabled: true,
        runtime_kind: "openclaw_sandbox",
      });
      const [mode, setMode] = useState<"template" | "custom">("template");
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
              id: "builtin.openclaw-worker",
              name: "OpenClaw Worker",
              role: "worker",
              runtime_kind: "openclaw_sandbox",
              description: "通用型助手（OpenClaw 版）",
            },
          ]}
          bootstrapConfig={{
            worker_runtime_choices: [
              { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
              { name: "openclaw", sandbox_enabled: true, installed: true, label: "OpenClaw" },
            ],
          }}
          managerAgent={null}
          agentModels={[]}
          agentModelBusy={false}
          locale="en"
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          agentCreateMode={mode}
          onAgentCreateModeChange={setMode}
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

    const { container } = render(<TestModal />);

    await user.click(screen.getByRole("tab", { name: /Custom/i }));

    expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("");
    expect(screen.getByRole("textbox", { name: "Description" })).toHaveValue("");
    expect(screen.queryByDisplayValue("https://git-devops.opencsg.com")).not.toBeInTheDocument();
    expect(container.querySelector(".agent-avatar-picker.has-avatar .agent-avatar-trigger-image")).toHaveAttribute(
      "src",
      templateAvatar,
    );
  });

  it("clears template env rows when switching to custom mode", async () => {
    const user = userEvent.setup();

    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        from_template: "builtin.openclaw-worker",
        template_name: "OpenClaw Worker",
        name: "generic-assistant-openclaw",
        description: "Template worker",
        runtime_name: "openclaw",
        sandbox_enabled: true,
        runtime_kind: "openclaw_sandbox",
        envRows: [
          { key: "GITLAB_TOKEN", value: "" },
          { key: "GITLAB_BASE_URL", value: "https://git-devops.opencsg.com" },
        ],
      });
      const [mode, setMode] = useState<"template" | "custom">("template");
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
              id: "builtin.openclaw-worker",
              name: "OpenClaw Worker",
              role: "worker",
              runtime_kind: "openclaw_sandbox",
              description: "Template worker",
              image_env: [
                { name: "GITLAB_TOKEN", secret: true },
                { name: "GITLAB_BASE_URL", secret: false, default: "https://git-devops.opencsg.com" },
              ],
            },
          ]}
          bootstrapConfig={{
            worker_runtime_choices: [
              { name: "codex", sandbox_enabled: false, installed: true, label: "Codex CLI" },
              { name: "openclaw", sandbox_enabled: true, installed: true, label: "OpenClaw" },
            ],
          }}
          managerAgent={null}
          agentModels={[]}
          agentModelBusy={false}
          locale="en"
          authStatuses={{}}
          authBusyProvider=""
          agentCreateBotKind="worker"
          agentCreateMode={mode}
          onAgentCreateModeChange={setMode}
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

    expect(screen.getByDisplayValue("https://git-devops.opencsg.com")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /Custom/i }));

    expect(screen.queryByDisplayValue("https://git-devops.opencsg.com")).not.toBeInTheDocument();
  });

  it("restores the last template selection when switching back from custom mode", async () => {
    const user = userEvent.setup();

    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        from_template: "builtin.openclaw-worker",
        template_name: "OpenClaw Worker",
        name: "OpenClaw Worker",
        description: "OpenClaw template",
        runtime_name: "openclaw",
        sandbox_enabled: true,
        runtime_kind: "openclaw_sandbox",
      });
      const [mode, setMode] = useState<"template" | "custom">("template");
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
              id: "builtin.picoclaw-worker",
              name: "PicoClaw Worker",
              role: "worker",
              runtime_kind: "picoclaw_sandbox",
              description: "PicoClaw template",
            },
            {
              id: "builtin.openclaw-worker",
              name: "OpenClaw Worker",
              role: "worker",
              runtime_kind: "openclaw_sandbox",
              description: "OpenClaw template",
            },
          ]}
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
          agentCreateMode={mode}
          onAgentCreateModeChange={setMode}
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

    expect(screen.getByRole("combobox", { name: "Template" })).toHaveTextContent("OpenClaw Worker");

    await user.click(screen.getByRole("tab", { name: /Custom/i }));
    await user.click(screen.getByRole("tab", { name: /From template/i }));

    expect(screen.getByRole("combobox", { name: "Template" })).toHaveTextContent("OpenClaw Worker");
  });

  it("hides manager templates from the worker template dropdown in create mode", async () => {
    const user = userEvent.setup();

    render(
      <AgentProfileModal
        t={t}
        agentModalMode="create"
        editingAgent={null}
        agentDraft={agentToDraft(worker)}
        onAgentDraftChange={vi.fn()}
        onAgentModelsReset={vi.fn()}
        hubTemplates={[
          {
            id: "builtin.manager-codex",
            name: "Manager",
            role: "manager",
            runtime_kind: "codex",
            description: "Coordinates workers.",
          },
          {
            id: "builtin.picoclaw-worker",
            name: "PicoClaw Worker",
            role: "worker",
            runtime_kind: "picoclaw_sandbox",
            description: "Legacy sandbox template.",
          },
          {
            id: "builtin.openclaw-worker",
            name: "OpenClaw Worker",
            role: "worker",
            runtime_kind: "openclaw_sandbox",
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

    await user.click(screen.getByRole("combobox", { name: "Template" }));
    expect(screen.getByRole("option", { name: "OpenClaw Worker" })).toBeInTheDocument();
    expect(screen.getByText("Handles coding tasks.")).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "PicoClaw Worker" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Manager" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "No template" })).not.toBeInTheDocument();
    expect(screen.queryByText("Legacy sandbox template.")).not.toBeInTheDocument();
    expect(screen.queryByText("Coordinates workers.")).not.toBeInTheDocument();
  });

  it("defaults template create to OpenClaw and excludes PicoClaw templates", async () => {
    function TestModal() {
      const [draft, setDraft] = useState<AgentDraft>({
        ...agentToDraft(worker),
        from_template: "",
        template_name: "",
        runtime_kind: "codex",
        runtime_name: "codex",
        sandbox_enabled: false,
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
              id: "builtin.codex-worker",
              name: "Codex Worker",
              role: "worker",
              runtime_kind: "codex",
              description: "Host runtime template.",
            },
            {
              id: "builtin.picoclaw-worker",
              name: "PicoClaw Worker",
              role: "worker",
              runtime_kind: "picoclaw_sandbox",
              description: "Sandbox template.",
            },
            {
              id: "builtin.openclaw-worker",
              name: "OpenClaw Worker",
              role: "worker",
              runtime_kind: "openclaw_sandbox",
              description: "OpenClaw sandbox template.",
            },
          ]}
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
          agentCreateMode="template"
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

    expect(await screen.findByRole("combobox", { name: "Template" })).toHaveTextContent("OpenClaw Worker");
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Template" }));
    expect(screen.queryByRole("option", { name: "PicoClaw Worker" })).not.toBeInTheDocument();
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
              runtime_kind: "openclaw_sandbox",
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

  it("shows provider logos only in the provider field, not in the model field", () => {
    const { container } = render(
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

    const fields = container.querySelectorAll(".agent-model-config-grid .field");
    const providerField = fields[0];
    const modelField = fields[1];
    expect(providerField?.querySelector(".model-option-avatar")).not.toBeNull();
    expect(modelField?.querySelector(".model-option-avatar")).toBeNull();
    expect(modelField?.querySelector(".model-option-provider")).toBeNull();
    expect(modelField?.textContent).toContain("MiniMax-M2.5");
    expect(modelField?.textContent).not.toContain("OpenCSG AIGateway");
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
    const searchInput = screen.getByRole("searchbox", { name: "Search models" });
    await user.type(searchInput, "qwen");

    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue("qwen");
    expect(screen.getByRole("option", { name: "Qwen3-235B" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "MiniMax-M2.5" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "gpt-5-high" })).not.toBeInTheDocument();

    await user.keyboard("{Backspace}{Backspace}{Backspace}{Backspace}");

    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue("");
    expect(screen.getByRole("option", { name: "MiniMax-M2.5" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Qwen3-235B" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "gpt-5-high" })).toBeInTheDocument();
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

  it("places avatar and name above a full-width description before the create-kind tabs", () => {
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
    expect(container.querySelector(".agent-create-kind-tabbar")).toBeInTheDocument();
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
