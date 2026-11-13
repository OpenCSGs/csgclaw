import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import type { SkillUpdateInfo } from "@/models/skillUpdates";
import {
  applySkillUpdates,
  fetchRemoteSkillsPage,
  fetchSkillFile,
  fetchSkills,
  fetchSkillTree,
  installRemoteSkillRequest,
} from "@/api/skills";

function mockFetch(handler: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) {
  const fetchMock = vi.fn<typeof fetch>(handler);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock as Mock<typeof fetch>;
}

describe("skills API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("loads local skills from the CSGClaw API", async () => {
    const fetchMock = mockFetch(async () => new Response(JSON.stringify([]), { status: 200 }));

    await fetchSkills();

    expect(fetchMock).toHaveBeenCalledWith("api/v1/skills", expect.any(Object));
  });

  it("keeps local and remote file requests separate", async () => {
    const fetchMock = mockFetch(async () => new Response("{}", { status: 200 }));
    await fetchSkillTree("demo");
    await fetchSkillFile("demo/SKILL.md");
    const remote = { remotePath: "team/demo", remoteRef: "feature/preview" };
    await fetchSkillTree("", remote);
    await fetchSkillFile("scripts/run.py", remote);
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      "api/v1/skills/tree?path=demo",
      "api/v1/skills/file?path=demo%2FSKILL.md",
      "api/v1/skills/tree?remote_path=team%2Fdemo&ref=feature%2Fpreview",
      "api/v1/skills/file?remote_path=team%2Fdemo&ref=feature%2Fpreview&path=scripts%2Frun.py",
    ]);
  });

  it("loads and normalizes remote skills through the CSGClaw API", async () => {
    const fetchMock = mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            items: [
              {
                description: "Build agents from natural language.",
                name: "agent-builder",
                recommended: true,
                readonly: true,
                remote_path: "AIWizards/agent-builder",
                remote_ref: "dev",
                remote_url: "https://opencsg.com/skills/AIWizards/agent-builder",
                source: "official",
              },
            ],
            next_page: 2,
            page: 1,
            per: 16,
            total: 78,
          }),
          { status: 200 },
        ),
    );

    await expect(fetchRemoteSkillsPage()).resolves.toEqual({
      hasMore: true,
      items: [
        {
          description: "Build agents from natural language.",
          name: "agent-builder",
          recommended: true,
          readonly: true,
          remotePath: "AIWizards/agent-builder",
          remoteRef: "dev",
          remoteURL: "https://opencsg.com/skills/AIWizards/agent-builder",
          source: "official",
        },
      ],
      nextPage: 2,
      page: 1,
      per: 16,
      total: 78,
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("api/v1/skills/remote?page=1&per=16&search=", expect.any(Object));
  });

  it("passes pagination and search to the CSGClaw remote skills endpoint", async () => {
    const fetchMock = mockFetch(
      async () => new Response(JSON.stringify({ items: [], page: 2, per: 16, total: 16 }), { status: 200 }),
    );

    await expect(fetchRemoteSkillsPage(2, "sa")).resolves.toMatchObject({
      hasMore: false,
      items: [],
      nextPage: null,
      page: 2,
      per: 16,
      total: 16,
    });
    expect(fetchMock).toHaveBeenCalledWith("api/v1/skills/remote?page=2&per=16&search=sa", expect.any(Object));
  });

  it("installs a remote skill through the CSGClaw API", async () => {
    const fetchMock = mockFetch(async () => new Response(JSON.stringify({ name: "agent-builder" }), { status: 201 }));

    await expect(installRemoteSkillRequest("AIWizards/agent-builder", "dev")).resolves.toEqual({
      name: "agent-builder",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/skills:install",
      expect.objectContaining({
        body: JSON.stringify({ remote_path: "AIWizards/agent-builder", ref: "dev" }),
        method: "POST",
      }),
    );
  });

  it("sends replace when installing over an existing remote skill", async () => {
    const fetchMock = mockFetch(async () => new Response(JSON.stringify({ name: "agent-builder" }), { status: 201 }));

    await expect(installRemoteSkillRequest("AIWizards/agent-builder", "dev", true)).resolves.toEqual({
      name: "agent-builder",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/skills:install",
      expect.objectContaining({
        body: JSON.stringify({ remote_path: "AIWizards/agent-builder", ref: "dev", replace: true }),
        method: "POST",
      }),
    );
  });
});

describe("skill update batches", () => {
  afterEach(() => vi.unstubAllGlobals());

  function plans(count: number): SkillUpdateInfo[] {
    return Array.from({ length: count }, (_, i) => ({
      name: `skill-${i}`,
      available: true,
      localVersion: "1.0.0",
      latestVersion: "2.0.0",
      commit: "release-commit",
      state: `state-${i}`,
      publishedAt: "2026-11-13T00:00:00Z",
      checkedAt: "2026-11-13T01:00:00Z",
      sourceUrl: `https://example.com/skills/team/skill-${i}`,
      remotePath: `team/skill-${i}`,
      targets: [],
    }));
  }

  it.each([100, 101, 201])("submits all %i updates within the API limit", async (count) => {
    const sizes: number[] = [];
    mockFetch(async (_url, init) => {
      const { items } = JSON.parse(String(init?.body));
      sizes.push(items.length);
      if (items.length > 100) return Response.json({}, { status: 400 });
      return Response.json({
        results: items.map((item: { name: string }) => ({
          name: item.name,
          targetId: "library",
          targetName: "",
          status: "updated",
        })),
      });
    });
    const response = await applySkillUpdates(plans(count), "update");
    expect(response.results).toHaveLength(count);
    expect(response.results.every((result) => result.status === "updated")).toBe(true);
    expect(response.results.map((result) => result.name)).toEqual(plans(count).map((item) => item.name));
    expect(sizes).toEqual(count === 100 ? [100] : count === 101 ? [100, 1] : [100, 100, 1]);
  });

  it("retains successful results and reports a failed batch without skipping later skills", async () => {
    let calls = 0;
    mockFetch(async (_url, init) => {
      const { items } = JSON.parse(String(init?.body));
      calls++;
      if (calls === 2) return Response.json({ error: { code: "skill_release_changed" } }, { status: 409 });
      expect(items.every((item: { strategy: string }) => item.strategy === "keep-local")).toBe(true);
      return Response.json({
        results: items.map((item: { name: string }) => ({
          name: item.name,
          targetId: "library",
          targetName: "",
          status: "kept",
        })),
      });
    });
    const response = await applySkillUpdates(plans(201), "keep-local");
    expect(calls).toBe(3);
    expect(response.results).toHaveLength(201);
    expect(response.results.slice(0, 100).every((result) => result.status === "kept")).toBe(true);
    expect(
      response.results
        .slice(100, 200)
        .every((result) => result.status === "failed" && result.error === "skill_release_changed"),
    ).toBe(true);
    expect(response.results[200].status).toBe("kept");
  });
});
