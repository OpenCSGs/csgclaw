import {
  DEFAULT_GITHUB_CONNECTOR_SCOPES,
  githubConnectorDraftFromStatus,
  gitLabConnectorDraftFromStatus,
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
      base_url: "",
      access_token_set: false,
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
      base_url: "",
      access_token_set: false,
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

  it("normalizes connector lists and keeps GitHub and GitLab", () => {
    expect(
      normalizeConnectorList({
        connectors: [{ provider: "github", configured: true }, null, "bad", { provider: "gitlab", configured: true }],
      }),
    ).toEqual([
      expect.objectContaining({
        provider: "github",
        configured: true,
      }),
      expect.objectContaining({ provider: "gitlab", configured: true }),
    ]);
  });

  it("normalizes GitLab status and creates a secret-free edit draft", () => {
    const status = normalizeConnectorStatus({
      provider: "gitlab",
      name: "GitLab",
      base_url: " https://gitlab.example.com ",
      access_token_set: true,
      access_token: "must-not-survive",
      configured: true,
      connected: true,
      account: { login: "root" },
    });
    expect(status).toEqual(
      expect.objectContaining({
        provider: "gitlab",
        base_url: "https://gitlab.example.com",
        access_token_set: true,
        connected: true,
      }),
    );
    expect(gitLabConnectorDraftFromStatus(status)).toEqual({
      base_url: "https://gitlab.example.com",
      access_token: "",
    });
    expect(Object.prototype.hasOwnProperty.call(status, "access_token")).toBe(false);
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
