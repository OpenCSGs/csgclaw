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
  profileRestartRequired: "Restart required",
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
};

describe("agent action visibility", () => {
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
        notifierWebhookOrigin=""
        setNotifierWebhookOrigin={vi.fn()}
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
        notifierWebhookOrigin=""
        setNotifierWebhookOrigin={vi.fn()}
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
});
