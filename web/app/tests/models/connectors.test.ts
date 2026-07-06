import {
  DEFAULT_GITHUB_CONNECTOR_SCOPES,
  githubConnectorDraftFromStatus,
  normalizeConnectorList,
  normalizeConnectorStatus,
  normalizeOAuthStartResponse,
} from "@/models/connectors";

describe("connector model", () => {
  it("normalizes GitHub status without preserving secrets", () => {
    const got = normalizeConnectorStatus({
      provider: " github ",
      name: " GitHub ",
      configured: true,
      connected: true,
      app_manageable: true,
      oauth_pending: true,
      client_id: " client-id ",
      client_secret_set: true,
      client_secret: "secret",
      access_token: "token",
      scopes: [" repo ", "", "read:user", "repo"],
      callback_url: " http://127.0.0.1:8080/api/v1/connectors/github/oauth/callback ",
      connected_at: "2026-07-01T01:02:03Z",
      updated_at: "2026-07-01T01:01:00Z",
      account: {
        login: " octocat ",
        id: 583231,
        avatar_url: " https://github.com/images/error/octocat_happy.gif ",
        html_url: " https://github.com/octocat ",
        name: " The Octocat ",
        email: " octocat@github.com ",
      },
    });

    expect(got).toEqual({
      provider: "github",
      name: "GitHub",
      configured: true,
      connected: true,
      app_manageable: true,
      oauth_pending: true,
      client_id: "client-id",
      client_secret_set: true,
      scopes: ["repo", "read:user"],
      callback_url: "http://127.0.0.1:8080/api/v1/connectors/github/oauth/callback",
      connected_at: "2026-07-01T01:02:03Z",
      updated_at: "2026-07-01T01:01:00Z",
      account: {
        login: "octocat",
        id: 583231,
        avatar_url: "https://github.com/images/error/octocat_happy.gif",
        html_url: "https://github.com/octocat",
        name: "The Octocat",
        email: "octocat@github.com",
      },
    });
    expect(Object.prototype.hasOwnProperty.call(got, "client_secret")).toBe(false);
    expect(Object.prototype.hasOwnProperty.call(got, "access_token")).toBe(false);
  });

  it("clears account fields when disconnected", () => {
    const got = normalizeConnectorStatus({
      provider: "github",
      configured: true,
      connected: false,
      account: { login: "octocat" },
    });

    expect(got.connected).toBe(false);
    expect(got.account).toBeNull();
  });

  it("normalizes connector lists and keeps only known connector objects", () => {
    expect(
      normalizeConnectorList({
        connectors: [{ provider: "github", configured: true }, null, "bad", { provider: "gitlab", configured: true }],
      }),
    ).toEqual([
      expect.objectContaining({
        provider: "github",
        configured: true,
      }),
    ]);
  });

  it("normalizes OAuth start responses", () => {
    expect(
      normalizeOAuthStartResponse({
        provider: " github ",
        authorization_url: " https://github.com/login/oauth/authorize?client_id=id ",
      }),
    ).toEqual({
      provider: "github",
      authorization_url: "https://github.com/login/oauth/authorize?client_id=id",
    });
    expect(normalizeOAuthStartResponse(null)).toEqual({ provider: "", authorization_url: "" });
  });

  it("builds an editable GitHub draft from status defaults", () => {
    expect(githubConnectorDraftFromStatus(normalizeConnectorStatus({ provider: "github" }))).toEqual({
      client_id: "",
      client_secret: "",
      scopes: DEFAULT_GITHUB_CONNECTOR_SCOPES,
    });
    expect(
      githubConnectorDraftFromStatus(
        normalizeConnectorStatus({
          provider: "github",
          client_id: "id",
          scopes: ["repo"],
        }),
      ),
    ).toEqual({
      client_id: "id",
      client_secret: "",
      scopes: ["repo"],
    });
  });
});
