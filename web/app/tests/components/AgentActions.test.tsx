import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { AgentDetailPane, AgentRow, AgentView, NotificationParticipantDetailPane } from "@/pages/AgentPage/components";
import { agentToDraft, type AgentDraft } from "@/models/agents";
import { AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY } from "@/shared/storage/keys";

const labels: Record<string, string> = {
  agentActivityTab: "Activity",
  agentDelete: "Delete",
  agentDeleteBoundChannels: "This agent is bound to {channels}.",
  agentDeleteCascadeNote: "Deleting the agent will also disconnect it from those channels.",
  agentDeleteConfirmMessage: 'Delete agent "{name}"?',
  agentInstructions: "Instructions",
  agentModel: "Model",
  cancel: "Cancel",
  editDescription: "Edit description",
  editAgentName: "Edit name",
  agentRecreate: "Recreate",
  agentStart: "Start",
  agentStop: "Stop",
  agentUpgrade: "Upgrade",
  agentMoreActions: "More",
  agentProfileSectionNavLabel: "Profile sections",
  agentProfileTab: "Profile",
  agentProfileSkillsTab: "Skills",
  agentProfileMCPTab: "MCP",
  agentSaved: "Saved",
  agentSaveChanges: "Save changes",
  agentUpdateSave: "Save",
  agentChannelsTitle: "Channels",
  agentChannelsDescription: "Manage external channels.",
  agentSkillAdd: "Add skill",
  agentSkillAddSubtitle: "Candidates come from global skills.",
  agentSkillAddEmpty: "No skills are available to add.",
  agentDeleteSkill: "Delete",
  agentDeleteSkillConfirmMessage: 'Delete skill "alpha" from this agent?',
  agentMCPAdd: "Add MCP",
  agentMCPAddSubtitle: "Candidates come from Hub.",
  agentMCPAddEmpty: "No MCP servers are available to add.",
  agentMCPEmpty: "No MCP servers installed yet.",
  agentDeleteMCP: "Delete",
  agentDeleteMCPConfirmMessage: 'Delete MCP server "{name}" from this agent?',
  feishuChannelName: "Feishu",
  feishuConnect: "Connect Feishu",
  feishuReconnect: "Reconnect Feishu",
  feishuDisconnect: "Disconnect Feishu",
  feishuCompleteConnection: "Complete connection",
  feishuPending: "Waiting",
  feishuPendingDetail: "Waiting for Feishu authorization. CSGClaw will finish automatically.",
  feishuConnected: "Connected",
  feishuDisconnected: "Disconnected",
  feishuOpenConnection: "Open Feishu",
  openDM: "DM",
  profileCompleteBadge: "Complete",
  profileAdvanced: "Advanced",
  profileFastMode: "Fast mode",
  profileModel: "Model",
  profileModelProvider: "Model provider",
  profileModelSection: "Model",
  profileProvider: "Provider",
  profileReasoning: "Reasoning",
  profileRequestOptions: "Request options",
  profileRestartRequired: "Recreate required",
  profileNotifierSection: "Notifications",
  profileUpgradeRequired: "Upgrade required",
  profileRuntimeKind: "Runtime",
  profileRuntimeSection: "Runtime environment",
  close: "Close",
  profileMCPServers: "MCP Servers",
  profileMCPServersHint: 'Enter MCP servers as {"server-name": {...}}.',
  profileMCPServersHubHint: "Install MCP servers from Hub.",
  profileMCPServersPlaceholder: '{\n  "context7": {}\n}',
  profileMCPServersUseExample: "Use example",
  profileMCPServersClear: "Clear servers",
  profileMCPServersInvalidJSON: "Enter a valid JSON object.",
  profileMCPServersObjectRequired: "MCP servers must be a JSON object.",
  agentName: "Name",
  agentDescription: "Description",
  agentImage: "Image",
};

function t(key: string, params: Record<string, string | number> = {}): string {
  return (labels[key] ?? key).replace(/\{(\w+)\}/g, (_, name: string) => `${params[name] ?? ""}`);
}

const worker = {
  id: "worker-1",
  name: "Worker",
  description: "Agent description",
  role: "worker",
  runtime_kind: "picoclaw_sandbox",
  status: "running",
  provider: "api",
  model_id: "gpt-test",
};

describe("agent action visibility", () => {
  beforeEach(() => {
    window.localStorage.removeItem(AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY);
  });

  it("shows a recreate warning when backend marks an agent restart required", () => {
    const onUpgrade = vi.fn();
    render(
      <AgentRow
        item={{ ...worker, env_restart_required: true }}
        t={t}
        activeRoom={null}
        busyKey=""
        onEdit={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={onUpgrade}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
      />,
    );

    expect(screen.getByText("Recreate required")).toBeInTheDocument();
    expect(screen.getByText("Complete")).toHaveClass("ready");
    expect(screen.queryByRole("button", { name: "Upgrade", hidden: true })).not.toBeInTheDocument();
  });

  it("shows an upgrade warning when only the agent image is outdated", () => {
    render(
      <AgentRow
        item={{ ...worker, image_upgrade_required: true }}
        t={t}
        activeRoom={null}
        busyKey=""
        onEdit={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
      />,
    );

    expect(screen.getByText("Upgrade required")).toBeInTheDocument();
    expect(screen.queryByText("Recreate required")).not.toBeInTheDocument();
  });

  it("shows upgrade and recreate for outdated worker rows even when lifecycle actions are hidden", async () => {
    const user = userEvent.setup();
    const onUpgrade = vi.fn();
    render(
      <AgentRow
        item={{ ...worker, image_upgrade_required: true }}
        t={t}
        activeRoom={null}
        busyKey=""
        onEdit={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={onUpgrade}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Upgrade", hidden: true }));
    expect(onUpgrade).toHaveBeenCalledWith(expect.objectContaining({ id: worker.id }));
    expect(screen.getByRole("button", { name: "Recreate", hidden: true })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("shows upgrade and recreate for outdated worker detail panes even when lifecycle actions are hidden", async () => {
    const user = userEvent.setup();
    const onUpgrade = vi.fn();
    render(
      <AgentDetailPane
        item={{ ...worker, image_upgrade_required: true }}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={null}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={onUpgrade}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Upgrade" }));
    expect(onUpgrade).toHaveBeenCalledWith(expect.objectContaining({ id: worker.id }));
    await user.click(screen.getByRole("button", { name: "More" }));
    expect(screen.getByRole("menuitem", { name: "Recreate" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Upgrade" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("keeps the agent name compact until entering edit mode", async () => {
    const user = userEvent.setup();
    const savedDraft = agentToDraft(worker);
    const onDraftChange = vi.fn();

    function TestPane() {
      const [draft, setDraft] = useState<AgentDraft>(savedDraft);
      return (
        <AgentDetailPane
          item={worker}
          t={t}
          activeRoom={null}
          busyKey=""
          error=""
          draft={draft}
          savedDraft={savedDraft}
          models={[]}
          modelBusy={false}
          saving={false}
          publishBusy={false}
          saveError=""
          authStatuses={{}}
          authBusyProvider=""
          notifierWebhookPublicOrigin="http://127.0.0.1:18080"
          onDraftChange={(nextDraft) => {
            onDraftChange(nextDraft);
            setDraft(nextDraft);
          }}
          onSave={vi.fn()}
          onPublish={vi.fn()}
          onProviderLogin={vi.fn()}
          onStart={vi.fn()}
          onStop={vi.fn()}
          onRecreate={vi.fn()}
          onUpgrade={vi.fn()}
          onDelete={vi.fn()}
          onInvite={vi.fn()}
          onOpenDM={vi.fn()}
        />
      );
    }

    render(<TestPane />);

    expect(screen.queryByRole("textbox", { name: "Name" })).not.toBeInTheDocument();
    const nameTrigger = screen.getByRole("button", { name: "Edit name" });
    expect(nameTrigger).toHaveTextContent("Worker");

    await user.click(nameTrigger);

    const nameInput = screen.getByRole("textbox", { name: "Name" });
    expect(nameInput).toHaveFocus();
    expect(nameInput).toHaveValue("Worker");

    await user.clear(nameInput);
    await user.type(nameInput, "测试工程师");

    expect(onDraftChange).toHaveBeenLastCalledWith(expect.objectContaining({ name: "测试工程师" }));
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();

    await user.tab();
    expect(screen.getByRole("button", { name: "Edit name" })).toHaveTextContent("测试工程师");
  });

  it("keeps the provider dropdown below its trigger", async () => {
    const user = userEvent.setup();
    const draft = {
      ...agentToDraft(worker),
      model_provider_id: "codex",
      model_id: "gpt-test",
    };

    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        modelOptions={[
          {
            value: "codex.gpt-test",
            label: "Codex / gpt-test",
            providerID: "codex",
            providerDisplayName: "Codex",
            providerAvatar: "model-providers/codex.svg",
            modelID: "gpt-test",
          },
        ]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Model provider" }));

    expect(screen.getByRole("listbox")).toHaveAttribute("data-side", "bottom");
  });

  it("renders the builtin manager name as fixed text in edit mode", () => {
    const manager = {
      ...worker,
      id: "agent-manager",
      name: "manager",
      role: "manager",
      runtime_kind: "codex",
      runtime: {
        option_schemas: [
          {
            key: "local_workspace_dir",
            path: "local_workspace_dir",
            label: "Local Workspace Dir",
            type: "directory",
          },
        ],
      },
    };
    const draft = agentToDraft(manager);

    render(
      <AgentDetailPane
        item={manager}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "manager" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit name" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Name" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit description" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Local Workspace Dir")).not.toBeInTheDocument();
  });

  it("shows Feishu connect and reconnect controls in the agent detail channels section", async () => {
    const user = userEvent.setup();
    const onStartFeishuConnect = vi.fn();
    const onDisconnectFeishu = vi.fn();
    const draft = agentToDraft(worker);
    const { rerender } = render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onStartFeishuConnect={onStartFeishuConnect}
        onFinalizeFeishuConnect={vi.fn()}
        onDisconnectFeishu={onDisconnectFeishu}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Channels" }));
    expect(screen.getByRole("region", { name: "Channels" })).toBeInTheDocument();
    expect(screen.getByText("Manage external channels.")).toBeInTheDocument();
    expect(document.querySelector(".agent-channel-icon img")).toHaveAttribute("src", "icons/feishu.png");
    expect(screen.getByText("Disconnected")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disconnect Feishu" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Connect Feishu" }));
    expect(onStartFeishuConnect).toHaveBeenCalledWith(expect.objectContaining({ id: worker.id }));

    rerender(
      <AgentDetailPane
        item={{
          ...worker,
          participants: [
            {
              agent_id: worker.id,
              channel: "feishu",
              channel_user_kind: "app_id",
              id: "worker-1",
              type: "agent",
            },
          ],
        }}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onStartFeishuConnect={onStartFeishuConnect}
        onFinalizeFeishuConnect={vi.fn()}
        onDisconnectFeishu={onDisconnectFeishu}
        notice="Feishu connection configured."
        noticeTone="success"
      />,
    );

    expect(screen.getByText("Connected")).toHaveClass("connected");
    expect(screen.getByText("Feishu connection configured.")).toHaveClass("success");
    expect(screen.getByRole("button", { name: "Reconnect Feishu" })).toBeInTheDocument();
    const channelActionButtons = Array.from(document.querySelectorAll(".agent-channel-actions .btn"));
    expect(channelActionButtons.map((button) => button.textContent?.trim())).toEqual([
      "Reconnect Feishu",
      "Disconnect Feishu",
    ]);
    const disconnectButton = screen.getByRole("button", { name: "Disconnect Feishu" });
    expect(disconnectButton).toHaveClass("btn-outline-danger");
    await user.click(disconnectButton);
    expect(onDisconnectFeishu).toHaveBeenCalledWith(expect.objectContaining({ id: worker.id }));
  });

  it("shows pending Feishu authorization without requiring a manual completion action", async () => {
    const user = userEvent.setup();
    const draft = agentToDraft(worker);
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        feishuPendingRegistration={{
          connect_url: "https://feishu.example/connect",
          registration_id: "reg-worker",
          status: "pending",
        }}
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onStartFeishuConnect={vi.fn()}
        onFinalizeFeishuConnect={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Channels" }));
    expect(screen.getByText("Waiting")).toBeInTheDocument();
    expect(
      screen.getByText("Waiting for Feishu authorization. CSGClaw will finish automatically."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open Feishu" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Complete connection" })).not.toBeInTheDocument();
  });

  it("shows connected Feishu status while a reconnect authorization is pending", async () => {
    const user = userEvent.setup();
    const connectedWorker = {
      ...worker,
      participants: [
        {
          agent_id: worker.id,
          channel: "feishu",
          channel_user_kind: "app_id",
          id: "worker-feishu",
          type: "agent",
        },
      ],
    };
    const draft = agentToDraft(connectedWorker);
    render(
      <AgentView
        item={connectedWorker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        feishuPendingRegistration={{
          connect_url: "https://feishu.example/connect",
          registration_id: "reg-worker",
          status: "pending",
        }}
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onStartFeishuConnect={vi.fn()}
        onFinalizeFeishuConnect={vi.fn()}
        onDisconnectFeishu={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Channels" }));
    expect(screen.getByText("Connected")).toBeInTheDocument();
    expect(
      screen.getByText("Waiting for Feishu authorization. CSGClaw will finish automatically."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open Feishu" })).toBeInTheDocument();
  });

  it("hides upgrade action in worker detail panes when only recreate is required", () => {
    render(
      <AgentDetailPane
        item={{ ...worker, env_restart_required: true }}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={null}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Upgrade" })).not.toBeInTheDocument();
    expect(screen.getByText("Recreate required")).toBeInTheDocument();
  });

  it("prints a temporary manager setup notice in detail panes", () => {
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        notice="Create manager first"
        draft={null}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Create manager first");
  });

  it("shows upgrade required in worker detail panes when only the agent image is outdated", () => {
    render(
      <AgentDetailPane
        item={{ ...worker, image_upgrade_required: true }}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={null}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByText("Upgrade required")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Upgrade" })).toBeInTheDocument();
    expect(screen.queryByText("Recreate required")).not.toBeInTheDocument();
  });

  it("hides upgrade controls for codex agents without images", () => {
    render(
      <AgentDetailPane
        item={{ ...worker, runtime_kind: "codex", image_upgrade_required: true }}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={null}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.queryByText("Upgrade required")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Upgrade" })).not.toBeInTheDocument();
  });

  it("trusts complete notifier profile state when gating recreate in detail panes", async () => {
    const user = userEvent.setup();
    const notifier = {
      ...worker,
      id: "notifier-1",
      name: "Notifier",
      runtime_kind: "notifier",
      profile_complete: true,
      runtime_options: {},
    };
    render(
      <AgentDetailPane
        item={notifier}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={agentToDraft(notifier)}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "More" }));
    expect(screen.getByRole("menuitem", { name: "Recreate" })).not.toHaveAttribute("data-disabled");
  });

  it("keeps header description compact until entering edit mode and removes duplicate basics fields", async () => {
    const user = userEvent.setup();
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={agentToDraft(worker)}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Edit name" })).toHaveTextContent("Worker");
    expect(screen.queryByDisplayValue("Agent description")).not.toBeInTheDocument();
    const descriptionTrigger = screen.getByRole("button", { name: "Edit description" });
    expect(descriptionTrigger).toHaveTextContent("Agent description");
    expect(screen.queryByText("Description")).not.toBeInTheDocument();

    await user.click(descriptionTrigger);
    const descriptionInput = screen.getByDisplayValue("Agent description");
    expect(descriptionInput).toBeInTheDocument();
    expect(descriptionInput.tagName).toBe("TEXTAREA");
  });

  it("shows model settings in Profile and instructions in their own tab", async () => {
    const user = userEvent.setup();
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={{ ...agentToDraft(worker), instructions: "reply in Chinese" }}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    const navigation = screen.getByRole("navigation", { name: "Profile sections" });
    expect(within(navigation).getByRole("button", { name: "Profile" })).toHaveAttribute("aria-current", "location");
    expect(screen.getByText("Model provider")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("reply in Chinese")).not.toBeInTheDocument();

    await user.click(within(navigation).getByRole("button", { name: "Instructions" }));

    const instructionsEditor = screen.getByDisplayValue("reply in Chinese");
    expect(instructionsEditor).toHaveClass("agent-instructions-editor");
    expect(instructionsEditor.closest(".agent-page-editor")).toHaveClass("agent-page-editor-instructions");
    expect(instructionsEditor.closest(".agent-profile-scroll-region")).toHaveClass(
      "agent-profile-scroll-region-instructions",
    );
    expect(screen.queryByText("Provider")).not.toBeInTheDocument();
  });

  it("does not show the image field in the merged agent profile runtime section", () => {
    const image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.8";
    const { container } = render(
      <AgentDetailPane
        item={{
          ...worker,
          image,
        }}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={agentToDraft({
          ...worker,
          image,
        })}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(container.querySelector("#agent-profile-runtime")).toBeInTheDocument();
    expect(screen.queryByLabelText("Image")).not.toBeInTheDocument();
  });

  it("installs MCP servers from hub candidates in the detail MCP tab", async () => {
    const user = userEvent.setup();
    const onInstallMCPServers = vi.fn(() => true);
    const onDeleteMCPServer = vi.fn(() => true);
    const item = {
      ...worker,
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
    const existingMCP = {
      name: "existing",
      config: {
        command: "node",
      },
      description: "node",
    };
    const context7MCP = {
      name: "context7",
      config: {
        command: "npx",
      },
      description: "npx",
    };

    render(
      <AgentDetailPane
        item={item}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={agentToDraft(item)}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        workspaceSupported
        mcpServers={[existingMCP]}
        mcpCandidates={[context7MCP]}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onInstallMCPServers={onInstallMCPServers}
        onDeleteMCPServer={onDeleteMCPServer}
      />,
    );

    expect(screen.queryByLabelText("MCP Servers")).not.toBeInTheDocument();
    const navigation = screen.getByRole("navigation", { name: "Profile sections" });
    expect(
      within(navigation)
        .getAllByRole("button")
        .map((button) => button.textContent),
    ).toEqual(["Profile", "Activity", "Instructions", "Skills0", "MCP", "Channels"]);

    await user.click(within(navigation).getByRole("button", { name: "MCP" }));

    expect(screen.getByText("existing")).toBeInTheDocument();
    expect(screen.getByText("node")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Add MCP" }));
    await user.click(screen.getByLabelText(/context7/));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Add MCP" }));

    expect(onInstallMCPServers).toHaveBeenCalledWith(["context7"]);

    const existingCard = screen.getByText("existing").closest("article");
    expect(existingCard).not.toBeNull();
    await user.click(within(existingCard as HTMLElement).getByRole("button", { name: "Delete" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }));

    expect(onDeleteMCPServer).toHaveBeenCalledWith(existingMCP);
  });

  it("keeps agent MCP server management separate from the profile editor", async () => {
    const user = userEvent.setup();
    const item = {
      ...worker,
      runtime_kind: "openclaw_sandbox",
      runtime_options: {},
      mcpServers: {
        existing: {
          command: "node",
        },
      },
    };
    const draft = agentToDraft(item);

    render(
      <AgentDetailPane
        item={item}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        workspaceSupported
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    const navigation = screen.getByRole("navigation", { name: "Profile sections" });
    await user.click(within(navigation).getByRole("button", { name: "MCP" }));

    expect(screen.queryByLabelText("MCP Servers")).not.toBeInTheDocument();
    expect(screen.getByText("No MCP servers installed yet.")).toBeInTheDocument();
    expect(screen.queryByText("Enter a valid JSON object.")).not.toBeInTheDocument();

    await user.click(within(navigation).getByRole("button", { name: "Profile" }));
    expect(screen.getByText("Saved")).toBeInTheDocument();

    await user.click(within(navigation).getByRole("button", { name: "MCP" }));
    expect(screen.queryByLabelText("MCP Servers")).not.toBeInTheDocument();
  });

  it("shows a saved status instead of a save button when the draft is unchanged", () => {
    const draft = agentToDraft(worker);
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Saved");
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("opens an add-skill dialog with only skills that are not already installed", async () => {
    const user = userEvent.setup();
    const onAddSkills = vi.fn().mockResolvedValue(true);
    const draft = agentToDraft(worker);
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        workspaceSupported
        skills={[{ name: "alpha", description: "Alpha installed" }]}
        skillCandidates={[
          { name: "beta", description: "Beta candidate" },
          { name: "gamma", description: "Gamma candidate" },
        ]}
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onAddSkills={onAddSkills}
      />,
    );

    await user.click(screen.getByRole("button", { name: /^skills/i }));
    await user.click(screen.getByRole("button", { name: "Add skill" }));
    expect(screen.getByText("Candidates come from global skills.")).toBeInTheDocument();
    expect(screen.getByText("Beta candidate")).toBeInTheDocument();
    expect(screen.getByText("Gamma candidate")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: /alpha/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole("checkbox", { name: /beta/i }));
    const addButtons = screen.getAllByRole("button", { name: "Add skill" });
    await user.click(addButtons[addButtons.length - 1]);

    expect(onAddSkills).toHaveBeenCalledWith(["beta"]);
    expect(screen.queryByText("Candidates come from global skills.")).not.toBeInTheDocument();
  });

  it("shows a confirmation dialog before deleting an installed skill", async () => {
    const user = userEvent.setup();
    const onDeleteSkill = vi.fn().mockResolvedValue(true);
    const draft = agentToDraft(worker);
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={draft}
        savedDraft={draft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        workspaceSupported
        skills={[{ name: "alpha", description: "Alpha installed" }]}
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onDeleteSkill={onDeleteSkill}
      />,
    );

    await user.click(screen.getByRole("button", { name: /^skills/i }));
    await user.click(screen.getAllByRole("button", { name: "Delete" })[0]);
    expect(screen.getByText('Delete skill "alpha" from this agent?')).toBeInTheDocument();

    const deleteButtons = screen.getAllByRole("button", { name: "Delete" });
    await user.click(deleteButtons[deleteButtons.length - 1]);
    expect(onDeleteSkill).toHaveBeenCalledWith({ name: "alpha", description: "Alpha installed" });
  });

  it("matches the notification bot profile header interaction and keeps actions on the right", async () => {
    const user = userEvent.setup();
    const notifier = {
      ...worker,
      id: "notifier-1",
      name: "Notifier",
      description: "Notifier description",
      type: "notification",
      bot_type: "notification",
      runtime_kind: "notifier",
    };
    const { container } = render(
      <NotificationParticipantDetailPane
        item={notifier}
        t={t}
        busyKey=""
        error=""
        saveError=""
        draft={{ ...agentToDraft(notifier), notifier_delivery_mode: "webhook" }}
        saving={false}
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onOpenDM={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.queryByText("Basics")).not.toBeInTheDocument();
    expect(screen.queryByText("profileNotifierSection")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Notifier" })).toBeInTheDocument();
    const descriptionTrigger = screen.getByRole("button", { name: "Edit description" });
    expect(descriptionTrigger).toHaveTextContent("Notifier description");
    expect(descriptionTrigger.closest(".entity-heading")).toBeInTheDocument();
    const toolbar = container.querySelector(".entity-header .entity-toolbar");
    expect(toolbar).toBeInTheDocument();
    expect(toolbar?.textContent).toContain("Save");
    expect(toolbar?.textContent).toContain("DM");
    expect(toolbar?.textContent).toContain("Delete");

    await user.click(descriptionTrigger);
    expect(screen.getByDisplayValue("Notifier description")).toBeInTheDocument();
    expect(screen.getByText("notifierDeliveryMode")).toBeInTheDocument();
  });

  it("shows save changes when the draft differs from the saved draft", () => {
    const savedDraft = agentToDraft(worker);
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={{ ...savedDraft, description: "Changed" }}
        savedDraft={savedDraft}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("opens a styled delete confirmation dialog for a Feishu-connected agent", async () => {
    const user = userEvent.setup();
    const onDelete = vi.fn();
    const connectedWorker = {
      ...worker,
      name: "Worker with Feishu",
      bot_type: "notification",
      type: "notification",
      participants: [
        {
          channel: "csgclaw",
          id: "worker-1",
          type: "agent",
        },
        {
          channel: "feishu",
          channel_user_kind: "app_id",
          id: "worker-1",
          type: "agent",
        },
      ],
    };

    render(
      <AgentView
        item={connectedWorker}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={null}
        models={[]}
        modelBusy={false}
        saving={false}
        publishBusy={false}
        saveError=""
        authStatuses={{}}
        authBusyProvider=""
        notifierWebhookPublicOrigin="http://127.0.0.1:18080"
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onPublish={vi.fn()}
        onProviderLogin={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onUpgrade={vi.fn()}
        onDelete={onDelete}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent('Delete agent "Worker with Feishu"?');
    expect(dialog).toHaveTextContent("This agent is bound to Feishu.");
    expect(dialog).toHaveTextContent("Deleting the agent will also disconnect it from those channels.");

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(onDelete).toHaveBeenCalledWith(expect.objectContaining({ id: connectedWorker.id }));
  });

  it("shows merged profile sections in the requested order", () => {
    const draft = agentToDraft(worker);
    const { container } = render(
      <AgentDetailPane
        item={worker}
        t={t}
        draft={draft}
        savedDraft={draft}
        models={[]}
        authStatuses={{}}
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Profile" })).toHaveAttribute("aria-current", "location");
    const runtimeSection = container.querySelector("#agent-profile-runtime");
    const modelSection = container.querySelector("#agent-profile-model");
    const advancedSection = container.querySelector("#agent-profile-advanced");
    const instructionsSection = container.querySelector("#agent-profile-instructions");

    expect(runtimeSection).toBeInTheDocument();
    expect(modelSection).toBeInTheDocument();
    expect(advancedSection).toBeInTheDocument();
    expect(instructionsSection).not.toBeInTheDocument();
    expect(runtimeSection?.compareDocumentPosition(modelSection as HTMLElement)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(modelSection?.compareDocumentPosition(advancedSection as HTMLElement)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(within(advancedSection as HTMLElement).getByText("Advanced")).toBeInTheDocument();
    expect(within(advancedSection as HTMLElement).getByText("Request options")).toBeInTheDocument();
  });

  it("navigates to profile sections from the horizontal tabs", async () => {
    const user = userEvent.setup();
    const draft = agentToDraft(worker);
    render(
      <AgentDetailPane
        item={worker}
        t={t}
        draft={draft}
        savedDraft={draft}
        models={[]}
        authStatuses={{}}
        workspaceSupported
        onDraftChange={vi.fn()}
        onSave={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onRecreate={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    const navigation = screen.getByRole("navigation", { name: "Profile sections" });
    const tabs = within(navigation).getAllByRole("button");
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      "Profile",
      "Activity",
      "Instructions",
      "Skills0",
      "MCP",
      "Channels",
    ]);
    expect(tabs[0]).toHaveAttribute("aria-current", "location");
    expect(screen.getByText("Request options")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Channels" })).not.toBeInTheDocument();

    await user.click(within(navigation).getByRole("button", { name: "Channels" }));

    expect(within(navigation).getByRole("button", { name: "Channels" })).toHaveAttribute("aria-current", "location");
    expect(screen.getByRole("region", { name: "Channels" })).toBeInTheDocument();
    expect(screen.queryByText("Request options")).not.toBeInTheDocument();
  });

  it("restores the last selected profile tab after remounting", async () => {
    const user = userEvent.setup();
    const draft = agentToDraft(worker);
    const props = {
      item: worker,
      t,
      draft,
      savedDraft: draft,
      models: [],
      authStatuses: {},
      workspaceSupported: true,
      onDraftChange: vi.fn(),
      onSave: vi.fn(),
      onStart: vi.fn(),
      onStop: vi.fn(),
      onRecreate: vi.fn(),
      onDelete: vi.fn(),
      onInvite: vi.fn(),
      onOpenDM: vi.fn(),
    };
    const { unmount } = render(<AgentDetailPane {...props} />);

    await user.click(screen.getByRole("button", { name: "Channels" }));
    expect(window.localStorage.getItem(AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY)).toBe("channels");
    unmount();

    render(<AgentDetailPane {...props} />);

    expect(screen.getByRole("button", { name: "Channels" })).toHaveAttribute("aria-current", "location");
    expect(screen.getByRole("region", { name: "Channels" })).toBeInTheDocument();
  });
});
