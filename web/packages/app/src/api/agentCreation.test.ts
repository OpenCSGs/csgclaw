import { applyAgentCreationResources } from "./agentCreation";
import { batchAddAgentMCPServersRequest, fetchAgentMCPServers, updateAgentRequest } from "./agents";
import { bindAgentApp } from "./apps";
import { fetchRemoteKnowledgeBaseMCPConfig } from "./knowledgeBases";
vi.mock("./agents", () => ({
  batchAddAgentMCPServersRequest: vi.fn(),
  fetchAgentMCPServers: vi.fn(),
  updateAgentRequest: vi.fn(),
}));
vi.mock("./apps", () => ({ bindAgentApp: vi.fn() }));
vi.mock("./knowledgeBases", () => ({ fetchRemoteKnowledgeBaseMCPConfig: vi.fn() }));

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
