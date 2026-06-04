import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentDetailPane, AgentRow } from "@/pages/AgentPage/components";
import { agentToDraft } from "@/models/agents";

const labels: Record<string, string> = {
  agentDelete: "Delete",
  agentModel: "Model",
  agentRecreate: "Recreate",
  agentStart: "Start",
  agentStop: "Stop",
  agentUpgrade: "Upgrade",
  agentMoreActions: "More",
  agentSaved: "Saved",
  agentSaveChanges: "Save changes",
  agentUpdateSave: "Save",
  openDM: "DM",
  profileCompleteBadge: "Complete",
  profileFastMode: "Fast mode",
  profileModel: "Model",
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
    screen.getByRole("button", { name: "Upgrade" }).click();
    expect(onUpgrade).toHaveBeenCalledWith(expect.objectContaining({ id: "worker-1" }));
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

  it("shows recreate for worker rows even when lifecycle actions are hidden", () => {
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

    expect(screen.getByRole("button", { name: "Recreate", hidden: true })).toBeInTheDocument();
    screen.getByRole("button", { name: "Upgrade", hidden: true }).click();
    expect(onUpgrade).toHaveBeenCalledWith(expect.objectContaining({ id: "worker-1" }));
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("shows recreate for worker detail panes even when lifecycle actions are hidden", async () => {
    const onUpgrade = vi.fn();
    const user = userEvent.setup();
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

    await user.click(screen.getByRole("button", { name: "More" }));
    expect(screen.getByRole("menuitem", { name: "Recreate" })).toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Upgrade" }));
    expect(onUpgrade).toHaveBeenCalledWith(expect.objectContaining({ id: "worker-1" }));
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("shows upgrade in worker detail panes when backend marks an agent restart required", async () => {
    const onUpgrade = vi.fn();
    const user = userEvent.setup();
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
        onUpgrade={onUpgrade}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "More" }));
    await user.click(screen.getByRole("menuitem", { name: "Upgrade" }));
    expect(onUpgrade).toHaveBeenCalledWith(expect.objectContaining({ id: "worker-1" }));
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
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "More" }));
    expect(screen.getByRole("menuitem", { name: "Recreate" })).not.toHaveAttribute("data-disabled");
  });

  it("keeps the agent detail name read-only while editing", () => {
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
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    const nameInput = screen.getByDisplayValue("Worker");
    expect(nameInput).toBeDisabled();
    expect(nameInput).toHaveAttribute("readonly");
  });

  it("shows long agent image values with full hover text and full-row alignment", () => {
    const image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.27";
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
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    const imageInput = screen.getByLabelText("Image");
    expect(imageInput).toHaveValue(image);
    expect(imageInput).toHaveAttribute("title", image);
    expect(imageInput).toHaveClass("long-image-input");
    expect(imageInput.closest("label")).toHaveClass("span-2", "agent-image-field");
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
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Saved");
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
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
        onUpgrade={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
