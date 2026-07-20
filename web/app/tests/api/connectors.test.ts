import { afterEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
import {
  disconnectGitHubConnectorRequest,
  disconnectGitLabConnectorRequest,
  fetchConnectors,
  fetchGitHubConnectorStatus,
  gitHubConnectorOAuthStartURL,
  saveGitHubConnectorConfigRequest,
  saveGitLabConnectorConfigRequest,
  startGitHubConnectorAppInstallRequest,
  startGitHubConnectorOAuthRequest,
} from "@/api/connectors";

function mockFetch(): Mock<typeof fetch> {
  const fetchMock = vi.fn<typeof fetch>(async (_input, _init) => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

async function requestJSON(init: RequestInit | undefined): Promise<unknown> {
  return JSON.parse(String(init?.body ?? "{}"));
}

describe("connector API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads connector collection and GitHub status", async () => {
    const fetchMock = mockFetch();

    await fetchConnectors();
    await fetchGitHubConnectorStatus();

    expect(fetchMock).toHaveBeenNthCalledWith(1, "api/v1/connectors", expect.any(Object));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "api/v1/connectors/github", expect.any(Object));
  });

  it("saves GitHub config without inventing an empty secret update", async () => {
    const fetchMock = mockFetch();

    await saveGitHubConnectorConfigRequest({
      client_id: "client-id",
      client_secret: "",
      scopes: ["repo", "read:user", "user:email"],
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/connectors/github/config",
      expect.objectContaining({ method: "PUT" }),
    );
    await expect(requestJSON(fetchMock.mock.calls[0]?.[1])).resolves.toEqual({
      client_id: "client-id",
      scopes: ["repo", "read:user", "user:email"],
    });
  });

  it("starts GitHub OAuth with the current return URL", async () => {
    const fetchMock = mockFetch();

    await startGitHubConnectorOAuthRequest("http://127.0.0.1:8080/workspace");

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/connectors/github/oauth/start",
      expect.objectContaining({ method: "POST" }),
    );
    await expect(requestJSON(fetchMock.mock.calls[0]?.[1])).resolves.toEqual({
      return_url: "http://127.0.0.1:8080/workspace",
    });
  });

  it("starts GitHub App install management", async () => {
    const fetchMock = mockFetch();

    await startGitHubConnectorAppInstallRequest();

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/connectors/github/app/install/start",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("builds the browser GitHub OAuth start URL", () => {
    expect(gitHubConnectorOAuthStartURL("http://127.0.0.1:8080/#/rooms/general")).toBe(
      "api/v1/connectors/github/oauth/start?return_url=http%3A%2F%2F127.0.0.1%3A8080%2F%23%2Frooms%2Fgeneral",
    );
  });

  it("disconnects GitHub", async () => {
    const fetchMock = mockFetch();

    await disconnectGitHubConnectorRequest();

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/connectors/github/disconnect",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("saves and disconnects GitLab without sending an empty token", async () => {
    const fetchMock = mockFetch();
    await saveGitLabConnectorConfigRequest({ base_url: "https://gitlab.example.com/", access_token: "" });
    await disconnectGitLabConnectorRequest();

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "api/v1/connectors/gitlab/config",
      expect.objectContaining({ method: "PUT" }),
    );
    await expect(requestJSON(fetchMock.mock.calls[0]?.[1])).resolves.toEqual({
      base_url: "https://gitlab.example.com",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "api/v1/connectors/gitlab/disconnect",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
