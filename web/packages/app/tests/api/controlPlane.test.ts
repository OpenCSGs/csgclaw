import { afterEach, describe, expect, it, vi } from "vitest";
import type { operations } from "@/api/generated/controlPlane";
import {
  archiveAgent,
  createBotRequest,
  fetchAgent,
  fetchAgents,
  fetchArchivedAgents,
  fetchAgentProfileModels,
  fetchAgentSkillSummaries,
  restoreAgent,
  runAgentActionRequest,
  updateAgentRequest,
} from "@/api/agents";
import { fetchAgentRuntimes } from "@/api/agentRuntimes";
import { fetchRuntimeImages } from "@/api/app";
import { fetchHubTemplates, fetchTemplateVersions } from "@/api/hub";
import { checkModelProvider, fetchModelProviders, fetchModelProviderPresets } from "@/api/modelProviders";
import { fetchSkills, rescanDiscoveredSkills } from "@/api/skills";

const agent = {
  id: "agent-1",
  name: "Reviewer",
  role: "worker",
  created_at: "2026-11-03T00:00:00Z",
  archived_by: "operator-1",
  runtime: {
    name: "codex",
    sandbox_enabled: false,
    image: "example/runtime:1",
    options: { execution_mode: "read_only" },
    state: "running",
  },
  model_config: { model_id: "chat", video_generation: { provider_id: "video", model_id: "generate" } },
  mcp_servers: { calendar: { url: "https://example.test/mcp" } },
};

afterEach(() => vi.unstubAllGlobals());

describe("control plane transport", () => {
  it.each([{}, { base_url: "https://models.example/v1", api_key: "test-key", connect: true }])(
    "sends the generated model provider check body %j",
    async (body) => {
      const request: operations["postModelProvidersIdCheck"]["requestBody"]["content"]["application/json"] = body;
      const fetchMock = vi.fn().mockResolvedValue(Response.json({ id: "custom", status: "connected", models: [] }));
      vi.stubGlobal("fetch", fetchMock);
      await checkModelProvider("custom", request);
      expect(fetchMock.mock.calls[0][0]).toBe("api/v1/model-providers/custom/check");
      expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual(body);
    },
  );

  it.each([
    ["detail", () => fetchAgent("agent-1")],
    ["archive", () => archiveAgent("agent-1")],
    ["restore", () => restoreAgent("agent-1")],
    ["start", () => runAgentActionRequest("agent-1", "start")],
  ] as const)("preserves runtime, model and MCP values after %s", async (_name, read) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(agent)));
    await expect(read()).resolves.toMatchObject({
      id: "agent-1",
      archived_by: "operator-1",
      runtime: agent.runtime,
      image: agent.runtime.image,
      model_config: agent.model_config,
      mcpServers: agent.mcp_servers,
    });
  });

  it("combines canonical Agent items with notification participants", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(Response.json({ items: [agent] }))
      .mockResolvedValueOnce(Response.json([{ id: "notification-1", type: "notification", name: "News" }]))
      .mockResolvedValueOnce(Response.json({ items: [agent] }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(fetchAgents()).resolves.toMatchObject([
      { id: "agent-1", image: agent.runtime.image, mcpServers: agent.mcp_servers },
      { id: "notification-1", type: "notification" },
    ]);
    await expect(fetchArchivedAgents()).resolves.toMatchObject([
      { id: "agent-1", archived_by: "operator-1", mcpServers: agent.mcp_servers },
    ]);
  });

  it("updates only supplied fields and preserves explicit empty values", async () => {
    const fetchMock = vi.fn().mockResolvedValue(Response.json(agent));
    vi.stubGlobal("fetch", fetchMock);
    await updateAgentRequest("agent-1", {
      description: "",
      runtime_options: {},
      image: "",
      profile: "custom.chat",
      mcpServers: null,
    });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      description: "",
      runtime: { image: "", options: {} },
      model_config: { selector: "custom.chat" },
      mcp_servers: null,
    });
  });

  it.each([false, true])("creates a bound Agent with sandbox_enabled=%s", async (sandboxEnabled) => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(Response.json({ id: "participant-1", agent_id: "agent-1" }))
      .mockResolvedValueOnce(Response.json(agent));
    vi.stubGlobal("fetch", fetchMock);
    await createBotRequest({
      name: "Reviewer",
      role: "worker",
      runtime_name: "codex",
      sandbox_enabled: sandboxEnabled,
      image: "example/runtime:1",
      runtime_options: {},
      skills: ["review"],
      from_template: "local.reviewer",
      profile: "custom.chat",
      agent_profile: { model_id: "chat", video_generation: { provider_id: "video", model_id: "generate" } },
      mcpServers: {},
    });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      name: "Reviewer",
      type: "agent",
      agent_binding: {
        mode: "create",
        agent: {
          name: "Reviewer",
          role: "worker",
          runtime: { name: "codex", sandbox_enabled: sandboxEnabled, image: "example/runtime:1", options: {} },
          skills: ["review"],
          from_template: "local.reviewer",
          model_config: {
            selector: "custom.chat",
            model_id: "chat",
            video_generation: { provider_id: "video", model_id: "generate" },
          },
          mcp_servers: {},
        },
      },
    });
  });

  it.each([
    ["runtime catalog", () => fetchAgentRuntimes(), [{ name: "codex", installed: true }]],
    ["image candidates", () => fetchRuntimeImages(), ["example/runtime:1"]],
    ["skills", () => fetchSkills(), [{ name: "review" }]],
    ["discovery", () => rescanDiscoveredSkills(), [{ name: "review" }]],
    ["Agent skills", () => fetchAgentSkillSummaries("agent-1"), [{ name: "review", enabled: false }]],
    ["templates", () => fetchHubTemplates(), [{ id: "local.review", latest_revision: 3 }]],
    ["template versions", () => fetchTemplateVersions("local.review"), [{ revision: 3, created_at: "2026-11-03" }]],
  ] as const)("reads %s from the items collection", async (_name, read, items) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ items })));
    await expect(read()).resolves.toEqual(items);
  });

  it("keeps model discovery metadata and provider capabilities", async () => {
    const provider = { id: "custom", status: "connected", models: ["chat"], video_models: ["video"] };
    const presets = [{ id: "custom", api_protocol: "openai" }];
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(Response.json({ items: ["chat"], provider: "api" }))
        .mockResolvedValueOnce(Response.json({ items: [provider] }))
        .mockResolvedValueOnce(Response.json({ items: presets })),
    );
    await expect(fetchAgentProfileModels({ provider: "api" })).resolves.toEqual({ models: ["chat"], provider: "api" });
    await expect(fetchModelProviders()).resolves.toMatchObject({
      providers: [{ id: "custom", videoModels: ["video"] }],
    });
    await expect(fetchModelProviderPresets()).resolves.toEqual({ presets });
  });
});
