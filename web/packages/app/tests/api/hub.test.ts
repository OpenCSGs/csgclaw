import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import {
  deleteHubTemplateRequest,
  fetchHubTemplate,
  fetchHubWorkspace,
  fetchHubWorkspaceFile,
  publishAgentTemplateRequest,
  publishHubTemplateToCommunityRequest,
} from "@/api/hub";

function mockFetch(): Mock<typeof fetch> {
  const fetchMock = vi.fn<typeof fetch>(async (_input, _init) => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("hub API", () => {
  it("sends Chinese display names separately from the validated name", async () => {
    const fetchMock = mockFetch();
    await publishAgentTemplateRequest("agent-alice", "local", "ReviewBot_2", "", false, "代码审查助手");
    const payload = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body));
    expect(payload).toMatchObject({ name: "ReviewBot_2", display_name: "代码审查助手" });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses single-id paths for namespaced template detail requests", async () => {
    const fetchMock = mockFetch();

    await fetchHubTemplate("builtin.manager-codex");

    expect(fetchMock).toHaveBeenCalledWith("api/v1/hub/templates/builtin.manager-codex", expect.any(Object));
  });

  it("keeps namespace/name template IDs inside one route segment", async () => {
    const fetchMock = mockFetch();

    await fetchHubTemplate("official.alice/review-bot");

    expect(fetchMock).toHaveBeenCalledWith("api/v1/hub/templates/official.alice~s~review-bot", expect.any(Object));
  });

  it("uses the URL-safe namespace separator for namespaced workspace file requests", async () => {
    const fetchMock = mockFetch();

    await fetchHubWorkspaceFile("Agentic/resume-scorer", "instructions/AGENTS.md");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates/Agentic~s~resume-scorer/workspace/file?path=instructions%2FAGENTS.md",
      expect.any(Object),
    );
  });

  it("escapes literal tildes without confusing them with the namespace separator", async () => {
    const fetchMock = mockFetch();

    await fetchHubTemplate("team~one/resume~scorer");

    expect(fetchMock).toHaveBeenCalledWith("api/v1/hub/templates/team~~one~s~resume~~scorer", expect.any(Object));
  });

  it.each([
    ["local", "local"],
    ["official", "official"],
  ] as const)("publishes agents to the selected %s registry", async (_label, registry) => {
    const fetchMock = mockFetch();

    await publishAgentTemplateRequest("agent-alice", registry, "ReviewBot_2", "Reviews changes");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates",
      expect.objectContaining({
        body: JSON.stringify({
          agent_id: "agent-alice",
          registry,
          name: "ReviewBot_2",
          display_name: "ReviewBot_2",
          description: "Reviews changes",
        }),
        method: "POST",
      }),
    );
  });

  it("includes the avatar only when saving locally", async () => {
    const fetchMock = mockFetch();
    await publishAgentTemplateRequest(
      "agent-alice",
      "local",
      "ReviewBot",
      "Review",
      false,
      "助手",
      "data:image/png;base64,aGVsbG8=",
    );
    expect(fetchMock).toHaveBeenLastCalledWith(
      "api/v1/hub/templates",
      expect.objectContaining({ body: expect.stringContaining('"avatar":"data:image/png;base64,aGVsbG8="') }),
    );
    await publishAgentTemplateRequest(
      "agent-alice",
      "official",
      "ReviewBot",
      "Review",
      false,
      "助手",
      "data:image/png;base64,aGVsbG8=",
    );
    expect(JSON.parse(fetchMock.mock.calls.at(-1)![1]!.body as string)).not.toHaveProperty("avatar");
  });

  it("includes agent memory only after explicit opt-in", async () => {
    const fetchMock = mockFetch();

    await publishAgentTemplateRequest("agent-alice", "official", "ReviewBot_2", "Reviews changes", true);

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates",
      expect.objectContaining({
        body: expect.stringContaining('"include_memory":true'),
        method: "POST",
      }),
    );
  });

  it("publishes a local template to the official registry", async () => {
    const fetchMock = mockFetch();

    await publishHubTemplateToCommunityRequest("local.review-bot");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates",
      expect.objectContaining({
        body: JSON.stringify({ template_id: "local.review-bot", registry: "official" }),
        method: "POST",
      }),
    );
  });

  it("sends the revision shown in the publication confirmation", async () => {
    const fetchMock = mockFetch();
    await publishHubTemplateToCommunityRequest("local.review-bot", false, 3);
    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates",
      expect.objectContaining({
        body: JSON.stringify({ template_id: "local.review-bot", expected_revision: 3, registry: "official" }),
        method: "POST",
      }),
    );
  });

  it("includes local template memory only after explicit opt-in", async () => {
    const fetchMock = mockFetch();

    await publishHubTemplateToCommunityRequest("local.review-bot", true);

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates",
      expect.objectContaining({
        body: JSON.stringify({
          template_id: "local.review-bot",
          registry: "official",
          include_memory: true,
        }),
        method: "POST",
      }),
    );
  });

  it("uses single-id paths for template delete requests", async () => {
    const fetchMock = mockFetch();

    await deleteHubTemplateRequest("local.gitlab-assistant");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates/local.gitlab-assistant",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("uses single-id paths for namespaced workspace file requests", async () => {
    const fetchMock = mockFetch();

    await fetchHubWorkspaceFile("builtin.manager-codex", "skills/custom/SKILL.md");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates/builtin.manager-codex/workspace/file?path=skills%2Fcustom%2FSKILL.md",
      expect.any(Object),
    );
  });

  it("loads workspace directories by path", async () => {
    const fetchMock = mockFetch();

    await fetchHubWorkspace("official.gitlab-assistant", "skills/custom");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates/official.gitlab-assistant/workspace?path=skills%2Fcustom",
      expect.any(Object),
    );
  });
});
