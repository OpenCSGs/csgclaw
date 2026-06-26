import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import { deleteHubTemplateRequest, fetchHubTemplate, fetchHubWorkspace, fetchHubWorkspaceFile } from "@/api/hub";

function mockFetch(): Mock<typeof fetch> {
  const fetchMock = vi.fn<typeof fetch>(async (_input, _init) => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("hub API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses single-id paths for namespaced template detail requests", async () => {
    const fetchMock = mockFetch();

    await fetchHubTemplate("builtin.openclaw-manager");

    expect(fetchMock).toHaveBeenCalledWith("api/v1/hub/templates/builtin.openclaw-manager", expect.any(Object));
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

    await fetchHubWorkspaceFile("builtin.openclaw-manager", "skills/custom/SKILL.md");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/hub/templates/builtin.openclaw-manager/workspace/file?path=skills%2Fcustom%2FSKILL.md",
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
