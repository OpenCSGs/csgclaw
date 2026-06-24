import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentProfileModal } from "@/pages/WorkspacePage/components";
import { agentToDraft } from "@/models/agents";
import * as directoryPicker from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";

const labels: Record<string, string> = {
  agentDescription: "Description",
  agentInstructions: "Instructions",
  agentImage: "Image",
  agentName: "Name",
  agentNamePlaceholder: "For example: dev",
  close: "Close",
  editAgentSubtitle: "Change runtime settings.",
  editAgentTitle: "Edit Agent",
  profileBasics: "Basics",
  profileModelSection: "Model",
  profileModel: "Model",
  modelProviderModelSearch: "Search models",
  modelProviderNoModels: "No models",
  profileRuntimeOptions: "Runtime Options",
  profileProvider: "Provider",
  profileRuntimeKind: "Runtime",
};

function t(key: string): string {
  return labels[key] ?? key;
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

  it("keeps the agent name read-only in edit mode", () => {
    render(
      <AgentProfileModal
        t={t}
        agentModalMode="edit"
        editingAgent={worker}
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

    const nameInput = screen.getByDisplayValue("Worker");
    expect(nameInput).toBeDisabled();
    expect(nameInput).toHaveAttribute("readonly");
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
    const tabbar = container.querySelector(".agent-create-kind-tabbar");
    expect(identityLayout?.compareDocumentPosition(tabbar as Node)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
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
