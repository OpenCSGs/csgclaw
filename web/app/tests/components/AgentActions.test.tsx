import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentDetailPane, AgentRow, NotificationParticipantDetailPane } from "@/pages/AgentPage/components";
import { agentToDraft } from "@/models/agents";

const labels: Record<string, string> = {
  agentDelete: "Delete",
  agentInstructions: "Instructions",
  agentModel: "Model",
  editDescription: "Edit description",
  agentRecreate: "Recreate",
  agentStart: "Start",
  agentStop: "Stop",
  agentUpgrade: "Upgrade",
  agentMoreActions: "More",
  agentSaved: "Saved",
  agentSaveChanges: "Save changes",
  agentUpdateSave: "Save",
  agentChannelsTitle: "Channels",
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
  profileFastMode: "Fast mode",
  profileModel: "Model",
  profileModelSection: "Model",
  profileProvider: "Provider",
  profileReasoning: "Reasoning",
  profileRestartRequired: "Recreate required",
  profileUpgradeRequired: "Upgrade required",
  profileRuntimeKind: "Runtime",
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
    const { container } = render(
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

    expect(screen.getByRole("heading", { name: "Worker" })).toBeInTheDocument();
    expect(screen.queryByDisplayValue("Agent description")).not.toBeInTheDocument();
    const descriptionTrigger = screen.getByRole("button", { name: "Edit description" });
    expect(descriptionTrigger).toHaveTextContent("Agent description");
    expect(container.querySelector('input[value="Worker"]')).not.toBeInTheDocument();
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
});
