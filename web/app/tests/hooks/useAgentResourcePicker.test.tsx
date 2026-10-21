import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAgentResourcePicker, type AgentResourcePickerKind } from "@/hooks/workspace/useAgentResourcePicker";
import { createTranslator } from "@/shared/i18n";
import type { SkillSummary } from "@/models/skillhub";
import { createQueryWrapper } from "../helpers/queryClient";

const t = createTranslator("en");
const remoteSkill = { name: "Review", remotePath: "team/review" };
const knowledge = { id: "kb-1", contentID: "wiki-1", name: "Engineering", availability: "available" as const };
const kbConfig = {
  url: "https://example.com/wiki/mcp",
  _meta: {
    "com.opencsg/mcp": {
      type: "llm_wiki",
      resource_id: "kb-1",
      content_id: "wiki-1",
      auth_type: "csghub_access_token",
    },
  },
};

function mockAPI({
  skills = [] as SkillSummary[],
  servers = {} as Record<string, unknown>,
  installedSkills = [] as { name: string }[],
  installedServers = {} as Record<string, unknown>,
  createdMCPName = "",
  knowledgeName = "knowledge-engineering",
} = {}) {
  const fetch = vi.fn(async (input: string, init?: RequestInit) => {
    const path = String(input);
    if (path === "api/v1/skills") return Response.json(skills);
    if (path.endsWith("/skill-summaries")) return Response.json(installedSkills);
    if (path === "api/v1/mcp-servers" && init?.method !== "POST") return Response.json({ mcpServers: servers });
    if (path.includes("/agents/") && path.endsWith("/mcp-servers")) return Response.json({ servers: installedServers });
    if (path === "api/v1/skills:install" || path === "api/v1/skills:upload") {
      skills = [...skills, { name: "review" }];
      return Response.json({ name: "review" });
    }
    if (path.endsWith("/mcp-config")) return Response.json({ name: knowledgeName, config: kbConfig });
    if (path === "api/v1/mcp-servers" && init?.method === "POST") {
      const body = JSON.parse(String(init.body));
      servers = { ...servers, [createdMCPName || body.name]: { ...body.config, display_name: body.name.trim() } };
      return Response.json({ mcpServers: servers });
    }
    if (path.endsWith("/remote/remote-1/install")) {
      servers = { ...servers, "mcp-installed": { command: "node" } };
      return Response.json({ name: "mcp-installed" });
    }
    if (path.includes("/remote?")) return Response.json({ items: [], total: 0 });
    throw new Error(`Unexpected request: ${path}`);
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

function renderPicker(
  kind: AgentResourcePickerKind = "skill",
  onAdd = vi.fn().mockResolvedValue(true),
  authenticated = true,
) {
  const result = renderHook(() => useAgentResourcePicker({ agentID: "agent-1", kind, authenticated, onAdd, t }), {
    wrapper: createQueryWrapper().wrapper,
  });
  return { ...result, onAdd };
}

afterEach(() => vi.unstubAllGlobals());

describe("Agent resource acquisition", () => {
  it("uses complete Agent membership and the shared global skill catalog", async () => {
    mockAPI({ skills: [{ name: "review" }, { name: "docs" }], installedSkills: [{ name: "review" }] });
    const { result } = renderPicker();
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    expect(result.current.skills).toHaveLength(2);
    expect([...result.current.installedNames]).toEqual(["review"]);
  });

  it("installs a market skill and binds the returned name to the Agent", async () => {
    const fetch = mockAPI();
    const { result, onAdd } = renderPicker();
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    await act(async () => {
      expect(await result.current.installSkill(remoteSkill)).toBe(true);
    });
    expect(onAdd).toHaveBeenCalledWith(["review"]);
    expect(fetch.mock.calls.find(([path]) => path === "api/v1/skills:install")?.[1]?.body).toBe(
      JSON.stringify({ remote_path: "team/review" }),
    );
  });

  it.each(["local", "market", "unknown"])(
    "requires explicit library selection when a same-named %s skill has no verified source identity",
    async (source) => {
      const fetch = mockAPI({ skills: [{ name: "review", source }], installedSkills: [{ name: "review" }] });
      const { result, onAdd } = renderPicker();
      await waitFor(() => expect(result.current.libraryLoading).toBe(false));
      expect(result.current.isSkillEnabled(remoteSkill)).toBe(false);
      await act(async () => {
        expect(await result.current.installSkill(remoteSkill)).toBe(false);
      });
      expect(onAdd).not.toHaveBeenCalled();
      expect(result.current.error).toBe(t("profileSkillNameConflict", { name: "review" }));
      expect(fetch.mock.calls.some(([path]) => path === "api/v1/skills:install")).toBe(false);
      await act(async () => {
        expect(await result.current.add(["review"])).toBe(true);
      });
      expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"]);
    },
  );

  it("retries binding the acquired market skill without reinstalling it", async () => {
    const fetch = mockAPI();
    const onAdd = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    const { result } = renderPicker("skill", onAdd);
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    const skill = { ...remoteSkill, remoteRef: "main" };
    await act(async () => {
      expect(await result.current.installSkill(skill)).toBe(false);
    });
    await act(async () => {
      expect(await result.current.installSkill(skill)).toBe(true);
    });
    expect(fetch.mock.calls.filter(([path]) => path === "api/v1/skills:install")).toHaveLength(1);
    expect(onAdd.mock.calls).toEqual([[["review"]], [["review"]]]);
  });

  it.each([
    { ...remoteSkill, remotePath: "different-publisher/review", remoteRef: "main" },
    { ...remoteSkill, remoteRef: "release" },
  ])("does not reuse an acquired skill for $remotePath at $remoteRef", async (skill) => {
    const fetch = mockAPI();
    const onAdd = vi.fn().mockResolvedValue(false);
    const { result } = renderPicker("skill", onAdd);
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    await act(async () => {
      expect(await result.current.installSkill({ ...remoteSkill, remoteRef: "main" })).toBe(false);
    });
    await act(async () => {
      expect(await result.current.installSkill(skill)).toBe(false);
    });
    expect(result.current.error).toBe(t("profileSkillNameConflict", { name: "review" }));
    expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"]);
    expect(fetch.mock.calls.filter(([path]) => path === "api/v1/skills:install")).toHaveLength(1);
  });

  it("retries Agent binding without uploading the same archive again", async () => {
    const fetch = mockAPI();
    const onAdd = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    const { result } = renderPicker("skill", onAdd);
    const file = new File(["archive"], "review.zip");
    await act(async () => {
      expect(await result.current.upload(file)).toBe(false);
    });
    await act(async () => {
      expect(await result.current.upload(file)).toBe(true);
    });
    expect(fetch.mock.calls.filter(([path]) => path === "api/v1/skills:upload")).toHaveLength(1);
    expect(onAdd).toHaveBeenCalledTimes(2);
  });

  it("does not bind to a different Agent after the picker is unmounted", async () => {
    const fetch = mockAPI();
    const { result, onAdd, unmount } = renderPicker();
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    let resolve!: (response: Response) => void;
    fetch.mockImplementationOnce(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    );
    let operation!: Promise<boolean>;
    act(() => {
      operation = result.current.installSkill(remoteSkill);
    });
    unmount();
    resolve(Response.json({ name: "review" }));
    await act(async () => {
      expect(await operation).toBe(false);
    });
    expect(onAdd).not.toHaveBeenCalled();
  });

  it.each(["local-tools", "Team tools", "团队工具"])(
    "binds the created MCP key for %s on every retry",
    async (name) => {
      const fetch = mockAPI({
        servers: { [name]: { command: "old", display_name: "Renamed server" } },
        createdMCPName: "mcp_created",
      });
      const onAdd = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
      const { result } = renderPicker("mcp", onAdd);
      const payload = { name, config: { command: "node", args: ["tools.js"] } };
      await waitFor(() => expect(result.current.libraryLoading).toBe(false));
      await act(async () => {
        expect(await result.current.createMCP(payload)).toBe(false);
      });
      await act(async () => {
        expect(await result.current.createMCP(payload)).toBe(true);
      });
      expect(
        fetch.mock.calls.filter(([path, init]) => path === "api/v1/mcp-servers" && init?.method === "POST"),
      ).toHaveLength(1);
      expect(onAdd.mock.calls).toEqual([[["mcp_created"]], [["mcp_created"]]]);
    },
  );

  it("binds the canonical name returned by the MCP market", async () => {
    mockAPI();
    const { result, onAdd } = renderPicker("mcp");
    await act(async () => {
      expect(await result.current.installMCP({ id: "remote-1", name: "Remote tools" })).toBe(true);
    });
    expect(onAdd).toHaveBeenCalledWith(["mcp-installed"]);
  });

  it("keeps the Agent unchanged when a creation response has no matching MCP", async () => {
    const fetch = mockAPI();
    const { result, onAdd } = renderPicker("mcp");
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    fetch.mockImplementationOnce(async () =>
      Response.json({ mcpServers: { tools: { command: "old", display_name: "Renamed server" } } }),
    );
    await act(async () => {
      expect(await result.current.createMCP({ name: "tools", config: { command: "node" } })).toBe(false);
    });
    expect(onAdd).not.toHaveBeenCalled();
    expect(result.current.error).toBe(t("profileMCPIdentityMissing"));
  });

  it("connects a knowledge base using its created MCP key and retains it for retries", async () => {
    const fetch = mockAPI({
      servers: { "knowledge-engineering": { command: "old", display_name: "Renamed server" } },
      createdMCPName: "mcp_knowledge",
      knowledgeName: "工程知识库",
    });
    const onAdd = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    const { result } = renderPicker("knowledge", onAdd);
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    await act(async () => {
      expect(await result.current.connectKnowledge(knowledge)).toBe(false);
    });
    await act(async () => {
      expect(await result.current.connectKnowledge(knowledge)).toBe(true);
    });
    const creates = fetch.mock.calls.filter(([path, init]) => path === "api/v1/mcp-servers" && init?.method === "POST");
    expect(creates).toHaveLength(1);
    expect(JSON.parse(String(creates[0][1]?.body))).toEqual({ name: "工程知识库", config: kbConfig });
    expect(onAdd.mock.calls).toEqual([[["mcp_knowledge"]], [["mcp_knowledge"]]]);
  });

  it("reuses connected knowledge configuration by source identity", async () => {
    const fetch = mockAPI({ servers: { renamed: kbConfig } });
    const { result, onAdd } = renderPicker("knowledge");
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    await act(async () => {
      await result.current.connectKnowledge(knowledge);
    });
    expect(onAdd).toHaveBeenCalledWith(["renamed"]);
    expect(fetch.mock.calls.some(([path]) => path.endsWith("/mcp-config"))).toBe(false);
  });

  it("keeps remote catalogs inactive until authentication is available", async () => {
    const fetch = mockAPI();
    const { result } = renderPicker("skill", undefined, false);
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    act(() => result.current.setMarket(true));
    expect(fetch.mock.calls.some(([path]) => path.includes("/remote?"))).toBe(false);
  });
});
