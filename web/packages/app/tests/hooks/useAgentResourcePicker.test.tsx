import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAgentResourcePicker, type AgentResourcePickerKind } from "@/hooks/workspace/useAgentResourcePicker";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { createTranslator } from "@/shared/i18n";
import type { SkillSummary } from "@/models/skillhub";
import type { AgentSkillSummary } from "@/api/agents";
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
  installedSkills = [] as AgentSkillSummary[],
  installedServers = {} as Record<string, unknown>,
  createdMCPName = "",
  knowledgeName = "knowledge-engineering",
  installedName = "review",
} = {}) {
  const fetch = vi.fn(async (input: string, init?: RequestInit) => {
    const path = String(input);
    if (path === "api/v1/skills") return Response.json(skills);
    if (path.endsWith("/skill-summaries")) return Response.json(installedSkills);
    if (path === "api/v1/mcp-servers" && init?.method !== "POST") return Response.json({ mcpServers: servers });
    if (path.includes("/agents/") && path.endsWith("/mcp-servers")) return Response.json({ servers: installedServers });
    if (path === "api/v1/skills:install" || path === "api/v1/skills:upload") {
      skills = [...skills.filter((skill) => skill.name !== installedName), { name: installedName }];
      return Response.json({ name: installedName });
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
  const { client, wrapper } = createQueryWrapper();
  const result = renderHook(() => useAgentResourcePicker({ agentID: "agent-1", kind, authenticated, onAdd, t }), {
    wrapper,
  });
  return { ...result, onAdd, client };
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

  it.each(["mcp", "knowledge"] as const)("distinguishes %s membership from enablement", async (kind) => {
    mockAPI({
      installedServers: {
        disabled: { command: "node", enabled: false },
        enabled: { command: "node", enabled: true },
        legacy: { command: "node" },
      },
    });
    const { result } = renderPicker(kind);
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    expect([...result.current.installedNames]).toEqual(["disabled", "enabled", "legacy"]);
    expect([...result.current.enabledNames]).toEqual(["enabled", "legacy"]);
  });

  it("installs a market skill globally before enabling its returned name for the Agent", async () => {
    const fetch = mockAPI({ installedName: "custom-review" });
    const { result, onAdd } = renderPicker();
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    let completeInstall!: (response: Response) => void;
    fetch.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          completeInstall = resolve;
        }),
    );
    let operation!: Promise<boolean>;
    act(() => {
      operation = result.current.installSkill(remoteSkill);
    });
    expect(onAdd).not.toHaveBeenCalled();
    await act(async () => {
      completeInstall(Response.json({ name: "custom-review" }));
      expect(await operation).toBe(true);
    });
    expect(onAdd).toHaveBeenCalledExactlyOnceWith(["custom-review"], { replace: true });
    expect(fetch.mock.calls.find(([path]) => path === "api/v1/skills:install")?.[1]?.body).toBe(
      JSON.stringify({ remote_path: "team/review" }),
    );
  });

  it.each(["local", "market", "unknown"])(
    "replaces the existing global %s skill before updating the worker",
    async (source) => {
      const fetch = mockAPI({ skills: [{ name: "review", source }] });
      const { result, onAdd } = renderPicker();
      await waitFor(() => expect(result.current.libraryLoading).toBe(false));
      expect(result.current.skillAction(remoteSkill)).toBe("replace");
      await act(async () => {
        expect(await result.current.installSkill(remoteSkill)).toBe(true);
      });
      expect(fetch.mock.calls.find(([path]) => path === "api/v1/skills:install")?.[1]?.body).toBe(
        JSON.stringify({ remote_path: "team/review", replace: true }),
      );
      expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"], { replace: true });
    },
  );

  it.each([false, true])(
    "refreshes cached global skill files after replace=%s even when the worker update fails",
    async (replace) => {
      mockAPI({ skills: replace ? [{ name: "review" }] : [] });
      const { result, client } = renderPicker("skill", vi.fn().mockResolvedValue(false));
      await waitFor(() => expect(result.current.libraryLoading).toBe(false));
      const cached = [
        { key: workspaceQueryKeys.skillTree("review"), old: { entries: ["old.md"] }, fresh: { entries: ["new.md"] } },
        {
          key: workspaceQueryKeys.skillTree("review/guides"),
          old: { entries: ["old.md"] },
          fresh: { entries: ["new.md"] },
        },
        { key: workspaceQueryKeys.skillFile("review/SKILL.md"), old: { content: "Old" }, fresh: { content: "New" } },
        {
          key: workspaceQueryKeys.skillFile("review/guides/setup.md"),
          old: { content: "Old guide" },
          fresh: { content: "New guide" },
        },
      ];
      for (const item of cached) client.setQueryData(item.key, item.old);
      await act(async () => {
        expect(await result.current.installSkill(remoteSkill)).toBe(false);
      });
      for (const item of cached) {
        const queryFn = vi.fn().mockResolvedValue(item.fresh);
        await expect(client.fetchQuery({ queryKey: item.key, queryFn, staleTime: Infinity })).resolves.toEqual(
          item.fresh,
        );
        expect(queryFn).toHaveBeenCalledOnce();
      }
    },
  );

  it.each([undefined, true, false])(
    "replaces a worker skill with enabled=%s even when the global copy is absent",
    async (enabled) => {
      const fetch = mockAPI({ installedSkills: [{ name: "review", enabled }] });
      const { result, onAdd } = renderPicker();
      await waitFor(() => expect(result.current.libraryLoading).toBe(false));
      expect(result.current.skillAction(remoteSkill)).toBe("replace");
      expect(result.current.enabledNames.has("review")).toBe(enabled !== false);
      await act(async () => {
        expect(await result.current.installSkill(remoteSkill)).toBe(true);
      });
      expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"], { replace: true });
      expect(fetch.mock.calls.find(([path]) => path === "api/v1/skills:install")?.[1]?.body).toBe(
        JSON.stringify({ remote_path: "team/review", replace: true }),
      );
    },
  );

  it("retries binding the acquired market skill without reinstalling it", async () => {
    const fetch = mockAPI({ installedName: "custom-review" });
    const onAdd = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    const { result } = renderPicker("skill", onAdd);
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    const skill = { ...remoteSkill, remoteRef: "main" };
    await act(async () => {
      expect(await result.current.installSkill(skill)).toBe(false);
    });
    expect(result.current.skillAction(skill)).toBe("replace");
    await act(async () => {
      expect(await result.current.installSkill(skill)).toBe(true);
    });
    expect(fetch.mock.calls.filter(([path]) => path === "api/v1/skills:install")).toHaveLength(1);
    expect(onAdd.mock.calls).toEqual([
      [["custom-review"], { replace: true }],
      [["custom-review"], { replace: true }],
    ]);
    await act(async () => {
      expect(await result.current.installSkill(skill)).toBe(true);
    });
    const installs = fetch.mock.calls.filter(([path]) => path === "api/v1/skills:install");
    expect(installs).toHaveLength(2);
    expect(JSON.parse(String(installs[1][1]?.body))).toEqual({
      remote_path: "team/review",
      ref: "main",
      replace: true,
    });
  });

  it.each([
    { ...remoteSkill, remotePath: "different-publisher/review", remoteRef: "main" },
    { ...remoteSkill, remoteRef: "release" },
  ])("clears an earlier retry after $remotePath at $remoteRef replaces the same name", async (skill) => {
    const fetch = mockAPI();
    const onAdd = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true);
    const { result } = renderPicker("skill", onAdd);
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    await act(async () => {
      expect(await result.current.installSkill({ ...remoteSkill, remoteRef: "main" })).toBe(false);
    });
    await act(async () => {
      expect(await result.current.installSkill(skill)).toBe(true);
    });
    await act(async () => {
      expect(await result.current.installSkill({ ...remoteSkill, remoteRef: "main" })).toBe(true);
    });
    expect(onAdd.mock.calls).toEqual([
      [["review"], { replace: true }],
      [["review"], { replace: true }],
      [["review"], { replace: true }],
    ]);
    const installs = fetch.mock.calls.filter(([path]) => path === "api/v1/skills:install");
    expect(installs.map(([, init]) => JSON.parse(String(init?.body)))).toEqual([
      { remote_path: "team/review", ref: "main" },
      { remote_path: skill.remotePath, ref: skill.remoteRef, replace: true },
      { remote_path: "team/review", ref: "main", replace: true },
    ]);
  });

  it.each([false, true])(
    "retries a failed global installation with replace=%s before updating the worker",
    async (replace) => {
      const fetch = mockAPI({ skills: replace ? [{ name: "review" }] : [] });
      const { result, onAdd } = renderPicker();
      await waitFor(() => expect(result.current.libraryLoading).toBe(false));
      fetch.mockResolvedValueOnce(
        Response.json({ error: { code: "skill_install_failed", message: "Install failed" } }, { status: 503 }),
      );
      await act(async () => {
        expect(await result.current.installSkill(remoteSkill)).toBe(false);
      });
      expect(result.current.error).toBe("Install failed");
      expect(result.current.skillAction(remoteSkill)).toBe(replace ? "replace" : "install");
      expect(onAdd).not.toHaveBeenCalled();
      await act(async () => {
        expect(await result.current.installSkill(remoteSkill)).toBe(true);
      });
      expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"], { replace: true });
      const installs = fetch.mock.calls.filter(([path]) => path === "api/v1/skills:install");
      expect(installs).toHaveLength(2);
      for (const [, init] of installs) {
        expect(JSON.parse(String(init?.body))).toEqual({
          remote_path: "team/review",
          ...(replace ? { replace: true } : {}),
        });
      }
    },
  );

  it("replaces the installed global skill after the picker is reopened", async () => {
    const fetch = mockAPI();
    const first = renderPicker("skill", vi.fn().mockResolvedValue(false));
    await waitFor(() => expect(first.result.current.libraryLoading).toBe(false));
    await act(async () => {
      expect(await first.result.current.installSkill(remoteSkill)).toBe(false);
    });
    first.unmount();
    const second = renderPicker();
    await waitFor(() => expect(second.result.current.libraryLoading).toBe(false));
    expect(second.result.current.skillAction(remoteSkill)).toBe("replace");
    await act(async () => {
      expect(await second.result.current.installSkill(remoteSkill)).toBe(true);
    });
    expect(second.onAdd).toHaveBeenCalledExactlyOnceWith(["review"], { replace: true });
    expect(fetch.mock.calls.filter(([path]) => path === "api/v1/skills:install")).toHaveLength(2);
  });

  it("enables library skills without replacing global files", async () => {
    const fetch = mockAPI({ skills: [{ name: "review" }] });
    const { result, onAdd } = renderPicker();
    await waitFor(() => expect(result.current.libraryLoading).toBe(false));
    await act(async () => {
      expect(await result.current.add(["review"])).toBe(true);
    });
    expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"]);
    expect(fetch.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
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
    expect(onAdd.mock.calls).toEqual([[["review"]], [["review"]]]);
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
