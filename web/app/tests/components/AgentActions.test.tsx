import { render, screen } from "@testing-library/react";
import { AgentDetailPane, AgentRow } from "@/pages/AgentPage/components";
import { agentToDraft } from "@/models/agents";

const labels: Record<string, string> = {
  agentDelete: "Delete",
  agentModel: "Model",
  agentRecreate: "Recreate",
  agentStart: "Start",
  agentStop: "Stop",
  agentUpdateSave: "Save",
  openDM: "DM",
  profileCompleteBadge: "Complete",
  profileFastMode: "Fast mode",
  profileModel: "Model",
  profileProvider: "Provider",
  profileReasoning: "Reasoning",
  profileRestartRequired: "Recreate required",
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
        onDelete={vi.fn()}
        onInvite={vi.fn()}
      />,
    );

    expect(screen.getByText("Recreate required")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Recreate" })).toBeInTheDocument();
  });

  it("shows recreate for worker rows even when lifecycle actions are hidden", () => {
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
        onDelete={vi.fn()}
        onInvite={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Recreate" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("shows recreate for worker detail panes even when lifecycle actions are hidden", () => {
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
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Recreate" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("trusts complete notifier profile state when gating recreate in detail panes", () => {
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

    expect(screen.getByRole("button", { name: "Recreate" })).not.toBeDisabled();
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
});
