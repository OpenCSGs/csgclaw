import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import {
  batchAddAgentMCPServersRequest,
  batchDeleteAgentMCPServersRequest,
  batchAddAgentSkillsRequest,
  deleteAgentSkillRequest,
  fetchAgentMCPServers,
  startFeishuRegistrationRequest,
} from "@/api/agents";

function mockFetch(): Mock<typeof fetch> {
  const fetchMock = vi.fn<typeof fetch>(async () => new Response("", { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("agents API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts batch add requests to the agent-scoped skills endpoint", async () => {
    const fetchMock = mockFetch();

    await batchAddAgentSkillsRequest("u-manager", ["alpha", "beta"]);

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/agents/u-manager/skills:batchAdd",
      expect.objectContaining({
        body: JSON.stringify({ names: ["alpha", "beta"] }),
        method: "POST",
      }),
    );
  });

  it("uses the agent MCP servers paths", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(JSON.stringify({ servers: {} }), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await fetchAgentMCPServers("u-manager");
    await batchAddAgentMCPServersRequest("u-manager", ["context7"]);
    await batchDeleteAgentMCPServersRequest("u-manager", ["context7"]);

    expect(fetchMock).toHaveBeenNthCalledWith(1, "api/v1/agents/u-manager/mcp-servers", expect.anything());
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "api/v1/agents/u-manager/mcp-servers:batchAdd",
      expect.objectContaining({
        body: JSON.stringify({ names: ["context7"] }),
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "api/v1/agents/u-manager/mcp-servers:batchDelete",
      expect.objectContaining({
        body: JSON.stringify({ names: ["context7"] }),
        method: "POST",
      }),
    );
  });

  it("deletes skills from the agent-scoped skills endpoint", async () => {
    const fetchMock = mockFetch();

    await deleteAgentSkillRequest("u-manager", "alpha");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/agents/u-manager/skills/alpha",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("returns the active pending Feishu registration when create reports a conflict", async () => {
    const pendingRegistration = {
      agent_id: "u-dev",
      connect_url: "https://open.feishu.cn/page/launcher?user_code=ABCD-EFGH",
      expires_at: "2026-06-22T03:48:22.7024502Z",
      next_poll_seconds: 5,
      participant_id: "dev",
      registration_id: "reg-dev",
      status: "pending",
      user_code: "ABCD-EFGH",
    };
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response(JSON.stringify(pendingRegistration), { status: 409 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(startFeishuRegistrationRequest("u-dev")).resolves.toEqual(pendingRegistration);
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/channels/feishu/registrations",
      expect.objectContaining({
        body: JSON.stringify({ agent_id: "u-dev" }),
        method: "POST",
      }),
    );
  });
});
