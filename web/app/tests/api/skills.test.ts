import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { fetchRemoteSkillsPage, fetchSkills, installRemoteSkillRequest } from "@/api/skills";

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

  it("loads and normalizes remote skills through the CSGClaw API", async () => {
    const fetchMock = mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            items: [
              {
                description: "Build agents from natural language.",
                name: "agent-builder",
                readonly: true,
                remote_path: "AIWizards/agent-builder",
                remote_ref: "dev",
                remote_url: "https://hub.example.test/skills/AIWizards/agent-builder",
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
          readonly: true,
          remotePath: "AIWizards/agent-builder",
          remoteRef: "dev",
          remoteURL: "https://hub.example.test/skills/AIWizards/agent-builder",
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
