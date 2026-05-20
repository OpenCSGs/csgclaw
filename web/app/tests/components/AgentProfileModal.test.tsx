import { render, screen } from "@testing-library/react";
import { AgentProfileModal } from "@/pages/WorkspacePage/components";
import { agentToDraft } from "@/models/agents";

const labels: Record<string, string> = {
  agentDescription: "Description",
  agentImage: "Image",
  agentName: "Name",
  agentNamePlaceholder: "For example: dev",
  close: "Close",
  editAgentSubtitle: "Change runtime settings.",
  editAgentTitle: "Edit Agent",
  profileBasics: "Basics",
  profileModelSection: "Model",
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
});
