import { applyAgentCreationResources, reconcileCreatedAgentRuntime } from "./agentCreation";
import {
  batchAddAgentMCPServersRequest,
  fetchAgent,
  fetchAgentMCPServers,
  runAgentActionRequest,
  updateAgentRequest,
} from "./agents";
import { bindAgentApp } from "./apps";
import { fetchRemoteKnowledgeBaseMCPConfig } from "./knowledgeBases";
vi.mock("./agents", () => ({
  batchAddAgentMCPServersRequest: vi.fn(),
  fetchAgent: vi.fn(),
  fetchAgentMCPServers: vi.fn(),
  runAgentActionRequest: vi.fn(),
  updateAgentRequest: vi.fn(),
}));
vi.mock("./apps", () => ({ bindAgentApp: vi.fn() }));
vi.mock("./knowledgeBases", () => ({ fetchRemoteKnowledgeBaseMCPConfig: vi.fn() }));

beforeEach(() => {
  vi.clearAllMocks();
});

it("applies selected resources while preserving existing connectors and reports partial failures", async () => {
  vi.mocked(batchAddAgentMCPServersRequest).mockRejectedValue(new Error("unavailable"));
  vi.mocked(bindAgentApp).mockResolvedValue({
    installation_id: "bound",
    agent_id: "created",
    app_id: "gitlab",
    name: "GitLab",
    enabled: true,
    disconnected: false,
    status: "connected",
    config: {},
    credentials_set: {},
    tools: [],
    created_at: "",
    updated_at: "",
  });
  vi.mocked(fetchRemoteKnowledgeBaseMCPConfig).mockResolvedValue({
    name: "knowledge",
    config: { url: "https://example.test/knowledge" },
  });
  vi.mocked(fetchAgentMCPServers).mockResolvedValue({
    servers: { existing: { command: "existing" }, tool: { command: "tool" } },
  });
  vi.mocked(updateAgentRequest).mockResolvedValue({ id: "created" });
  const failed = await applyAgentCreationResources("created", {
    connectors: ["tool"],
    apps: ["app1"],
    knowledgeBases: [
      { id: "kb1", name: "KB" },
      { id: "kb2", name: "Connected KB", configuredMCPName: "saved-kb" },
    ],
  });
  expect(failed).toEqual(["tool, saved-kb"]);
  expect(batchAddAgentMCPServersRequest).toHaveBeenCalledWith("created", ["tool", "saved-kb"]);
  expect(bindAgentApp).toHaveBeenCalledWith("created", "app1");
  expect(updateAgentRequest).toHaveBeenCalledWith("created", {
    mcpServers: {
      existing: { command: "existing" },
      tool: { command: "tool" },
      knowledge: { url: "https://example.test/knowledge" },
    },
  });
});

it.each(["created", "stopped", "exited", "failed"])(
  "restarts a newly-created agent left in the recoverable %s state",
  async (status) => {
    const stopped = { id: "created", name: "Agent", role: "worker", status };
    const running = { ...stopped, status: "running" };
    vi.mocked(fetchAgent).mockResolvedValue(stopped);
    vi.mocked(runAgentActionRequest).mockResolvedValue(running);

    await expect(reconcileCreatedAgentRuntime("created")).resolves.toEqual(running);

    expect(fetchAgent).toHaveBeenCalledWith("created", { cacheBust: true });
    expect(runAgentActionRequest).toHaveBeenCalledWith("created", "start");
  },
);

it("recreates a newly-created agent whose runtime state is unknown", async () => {
  const unknown = { id: "created", name: "Agent", role: "worker", status: "unknown" };
  const running = { ...unknown, status: "running" };
  vi.mocked(fetchAgent).mockResolvedValue(unknown);
  vi.mocked(runAgentActionRequest).mockResolvedValue(running);

  await expect(reconcileCreatedAgentRuntime("created")).resolves.toEqual(running);

  expect(runAgentActionRequest).toHaveBeenCalledWith("created", "recreate");
});

it.each(["running", "starting", "stopping", "runtime_unavailable"])(
  "does not override the non-recoverable created-agent state %s",
  async (status) => {
    const agent = { id: "created", name: "Agent", role: "worker", status };
    vi.mocked(fetchAgent).mockResolvedValue(agent);

    await expect(reconcileCreatedAgentRuntime("created")).resolves.toEqual(agent);

    expect(runAgentActionRequest).not.toHaveBeenCalled();
  },
);
