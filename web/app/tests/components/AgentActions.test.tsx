import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { AgentDetailPane, AgentRow, NotificationParticipantDetailPane } from "@/pages/AgentPage/components";
import { agentToDraft, type AgentDraft } from "@/models/agents";

const labels: Record<string, string> = {
  agentDelete: "Delete",
  agentInstructions: "Instructions",
  agentModel: "Model",
  editDescription: "Edit description",
  editAgentName: "Edit name",
  agentRecreate: "Recreate",
  agentStart: "Start",
  agentStop: "Stop",
  agentUpgrade: "Upgrade",
  agentMoreActions: "More",
  agentProfileSectionNavLabel: "Profile sections",
  agentProfileSkillsTab: "skills",
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
  profileModelSection: "Model",
  profileProvider: "Provider",
  profileReasoning: "Reasoning",
  profileRequestOptions: "Request options",
  profileRestartRequired: "Recreate required",
  profileNotifierSection: "Notifications",
  profileUpgradeRequired: "Upgrade required",
  profileRuntimeKind: "Runtime",
  profileRuntimeSection: "Runtime environment",
  agentName: "Name",
  agentDescription: "Description",
  agentImage: "Image",
};

function t(key: string): string {
  return labels[key] ?? key;
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
    expect(screen.getByRole("button", { name: "Upgrade", hidden: true })).toBeInTheDocument();
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

  it("shows upgrade and recreate for worker rows even when lifecycle actions are hidden", async () => {
    const user = userEvent.setup();
    const onUpgrade = vi.fn();
    render(
      <AgentRow
        item={worker}
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

  it("shows upgrade and recreate for worker detail panes even when lifecycle actions are hidden", async () => {
    const user = userEvent.setup();
    const onUpgrade = vi.fn();
    render(
      <AgentDetailPane
        item={worker}
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

  it("renders the builtin manager name as fixed text in edit mode", () => {
    const manager = {
      ...worker,
      id: "agent-manager",
      name: "manager",
      role: "manager",
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

    expect(screen.getByRole("heading", { name: "Channels" })).toBeInTheDocument();
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

  it("shows pending Feishu authorization without requiring a manual completion action", () => {
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

    expect(screen.getByText("Waiting")).toBeInTheDocument();
    expect(
      screen.getByText("Waiting for Feishu authorization. CSGClaw will finish automatically."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open Feishu" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Complete connection" })).not.toBeInTheDocument();
  });

  it("keeps upgrade action visible in worker detail panes when backend marks an agent restart required", () => {
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

    expect(screen.getByRole("button", { name: "Upgrade" })).toBeInTheDocument();
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
    expect(screen.queryByText("Recreate required")).not.toBeInTheDocument();
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

  it("places instructions below the model section in the profile editor", () => {
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

    const modelSectionTitle = screen.getAllByText("Model")[0];
    const instructionSectionTitle = screen.getAllByText("Instructions")[0];
    expect(screen.getByDisplayValue("reply in Chinese")).toBeInTheDocument();
    expect(modelSectionTitle.compareDocumentPosition(instructionSectionTitle)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });

  it("shows long agent image values with full hover text and full-row alignment", () => {
    const image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.8";
    render(
      <AgentDetailPane
        item={{ ...worker, image }}
        t={t}
        activeRoom={null}
        busyKey=""
        error=""
        draft={agentToDraft({ ...worker, image })}
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

    const imageInput = screen.getByLabelText("Image");
    expect(imageInput).toHaveValue(image);
    expect(imageInput).toHaveAttribute("title", image);
    expect(imageInput).toHaveClass("long-image-input");
    expect(imageInput.closest("label")).toHaveClass("agent-image-field");
    expect(imageInput.closest(".agent-runtime-image-row")).toBeInTheDocument();
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

  it("shows advanced profile options by default", () => {
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

    const advancedSection = container.querySelector("#agent-profile-advanced");
    expect(advancedSection).toBeInTheDocument();
    expect(within(advancedSection as HTMLElement).getByText("Advanced")).toBeInTheDocument();
    expect(within(advancedSection as HTMLElement).getByText("Request options")).toBeInTheDocument();
  });

  it("navigates to profile sections from the horizontal tabs", async () => {
    const user = userEvent.setup();
    const draft = agentToDraft(worker);
    const scrollTo = vi.fn();
    const originalScrollTo = HTMLElement.prototype.scrollTo;
    HTMLElement.prototype.scrollTo = scrollTo;

    try {
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
      const modelTab = within(navigation).getByRole("button", { name: "Model" });
      expect(within(navigation).getAllByRole("button")).toHaveLength(6);

      await user.click(modelTab);

      expect(modelTab).toHaveAttribute("aria-current", "location");
      await waitFor(() => expect(scrollTo).toHaveBeenCalledWith({ top: 0 }));
    } finally {
      HTMLElement.prototype.scrollTo = originalScrollTo;
    }
  });
});
