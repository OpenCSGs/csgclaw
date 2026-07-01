import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { fetchAgenticHubOfficialSkillsPage, fetchSkills, installRemoteSkillRequest } from "@/api/skills";

const AGENTICHUB_SKILLS_URL =
  "https://opencsg-stg.example.test/api/v1/skills?page=1&per=16&search=&sort=trending&source=";
const SERVER_CONFIG_RESPONSE = JSON.stringify({
  hub_official_url: "https://opencsg-stg.example.test/",
});

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

  it("normalizes AgenticHub skills as readonly official skills", async () => {
    const fetchMock = mockFetch(async (input) => {
      if (String(input) === "api/v1/server/config") {
        return new Response(SERVER_CONFIG_RESPONSE, { status: 200 });
      }
      return new Response(
        JSON.stringify({
          msg: "OK",
          data: [
            {
              default_branch: "dev",
              description: "Build agents from natural language.",
              name: "agent-builder",
              path: "AIWizards/agent-builder",
              source: "local",
            },
            {
              description: "Missing remote path should not be installable.",
              name: "broken-skill",
            },
          ],
          total: 2,
        }),
        { status: 200 },
      );
    });

    const page = await fetchAgenticHubOfficialSkillsPage();

    expect(page.items).toEqual([
      {
        description: "Build agents from natural language.",
        name: "agent-builder",
        readonly: true,
        remoteRef: "dev",
        remotePath: "AIWizards/agent-builder",
        source: "official",
      },
    ]);
    expect(fetchMock).toHaveBeenCalledWith("api/v1/server/config", expect.any(Object));
    expect(fetchMock).toHaveBeenCalledWith(AGENTICHUB_SKILLS_URL, expect.objectContaining({ credentials: "omit" }));
  });

  it("uses the configured official Hub URL", async () => {
    const fetchMock = mockFetch(async (input) => {
      if (String(input) === "api/v1/server/config") {
        return new Response(JSON.stringify({ hub_official_url: "https://opencsg-stg.com/" }), { status: 200 });
      }
      return new Response(JSON.stringify({ data: [] }), { status: 200 });
    });

    await fetchAgenticHubOfficialSkillsPage();

    expect(fetchMock).toHaveBeenCalledWith(
      "https://opencsg-stg.com/api/v1/skills?page=1&per=16&search=&sort=trending&source=",
      expect.objectContaining({ credentials: "omit" }),
    );
  });

  it("loads an AgenticHub official skills page with pagination metadata", async () => {
    const fetchMock = mockFetch(async (input) => {
      if (String(input) === "api/v1/server/config") {
        return new Response(SERVER_CONFIG_RESPONSE, { status: 200 });
      }
      return new Response(
        JSON.stringify({
          data: [{ name: "page-two-skill", path: "AIWizards/page-two-skill" }],
          total: 78,
        }),
        { status: 200 },
      );
    });

    await expect(fetchAgenticHubOfficialSkillsPage(2)).resolves.toMatchObject({
      hasMore: true,
      items: [
        {
          name: "page-two-skill",
          remotePath: "AIWizards/page-two-skill",
          source: "official",
        },
      ],
      nextPage: 3,
      page: 2,
      per: 16,
      total: 78,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://opencsg-stg.example.test/api/v1/skills?page=2&per=16&search=&sort=trending&source=",
      expect.objectContaining({ credentials: "omit" }),
    );
  });

  it("passes the AgenticHub official skills search parameter", async () => {
    const fetchMock = mockFetch(async (input) => {
      if (String(input) === "api/v1/server/config") {
        return new Response(SERVER_CONFIG_RESPONSE, { status: 200 });
      }
      return new Response(JSON.stringify({ data: [], total: 0 }), { status: 200 });
    });

    await expect(fetchAgenticHubOfficialSkillsPage(1, "sa")).resolves.toMatchObject({
      items: [],
      page: 1,
      total: 0,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://opencsg-stg.example.test/api/v1/skills?page=1&per=16&search=sa&sort=trending&source=",
      expect.objectContaining({ credentials: "omit" }),
    );
  });

  it("does not fall back to a default Hub URL when server config fails", async () => {
    const fetchMock = mockFetch(async () => new Response("unavailable", { status: 500 }));

    await expect(fetchAgenticHubOfficialSkillsPage()).rejects.toMatchObject({ status: 500 });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("api/v1/server/config", expect.any(Object));
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
});
