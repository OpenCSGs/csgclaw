import {
  authEnvironmentDisplayLabel,
  authEnvironmentDraftFromPreset,
  authEnvironmentDraftFromStatus,
  authEnvironmentLoginPayload,
  normalizeAuthEnvironmentDraft,
} from "@/models/authEnvironment";

describe("auth environment model", () => {
  it("builds the stage login payload", () => {
    expect(authEnvironmentLoginPayload(authEnvironmentDraftFromPreset("stage"))).toEqual({
      opencsg_base_url: "https://opencsg-stg.com",
      csghub_base_url: "https://opencsg-stg.com",
      ai_gateway_base_url: "https://aigateway.opencsg-stg.com/v1",
    });
  });

  it("derives custom service URLs from the OpenCSG site URL", () => {
    const draft = normalizeAuthEnvironmentDraft({
      preset: "custom",
      opencsgBaseURL: " https://openeast.opencsg.com/ ",
      csgHubBaseURL: "",
      aiGatewayBaseURL: "",
    });

    expect(authEnvironmentLoginPayload(draft)).toEqual({
      opencsg_base_url: "https://openeast.opencsg.com",
      csghub_base_url: "https://openeast.opencsg.com",
      ai_gateway_base_url: "https://openeast.opencsg.com/aigateway/v1",
    });
  });

  it("ignores custom service URL overrides and derives from the site URL", () => {
    const draft = normalizeAuthEnvironmentDraft({
      preset: "custom",
      opencsgBaseURL: " https://openeast.opencsg.com/ ",
      csgHubBaseURL: " https://hub.example.com/ ",
      aiGatewayBaseURL: " https://east.opencsg.com/aigateway/ ",
    });

    expect(authEnvironmentLoginPayload(draft)).toEqual({
      opencsg_base_url: "https://openeast.opencsg.com",
      csghub_base_url: "https://openeast.opencsg.com",
      ai_gateway_base_url: "https://openeast.opencsg.com/aigateway/v1",
    });
  });

  it("hydrates a draft from authenticated status", () => {
    const draft = authEnvironmentDraftFromStatus({
      authenticated: true,
      user_id: "alice",
      user_uuid: "user-1",
      avatar: "",
      opencsg_base_url: "https://opencsg-stg.com",
      base_url: "https://opencsg-stg.com",
      ai_gateway_base_url: "https://aigateway.opencsg-stg.com/v1",
      portal_url: "",
      logged_in_at: "",
    });

    expect(draft.preset).toBe("stage");
  });

  it("hydrates custom status without falling back to production service URLs", () => {
    const draft = authEnvironmentDraftFromStatus({
      authenticated: true,
      user_id: "alice",
      user_uuid: "user-1",
      avatar: "",
      opencsg_base_url: "https://openeast.opencsg.com",
      base_url: "",
      ai_gateway_base_url: "",
      portal_url: "",
      logged_in_at: "",
    });

    expect(draft).toMatchObject({
      preset: "custom",
      opencsgBaseURL: "https://openeast.opencsg.com",
      csgHubBaseURL: "https://openeast.opencsg.com",
      aiGatewayBaseURL: "https://openeast.opencsg.com/aigateway/v1",
    });
  });

  it("shows the selected environment by domain", () => {
    expect(authEnvironmentDisplayLabel(authEnvironmentDraftFromPreset("prod"))).toBe("opencsg.com");
    expect(
      authEnvironmentDisplayLabel(
        normalizeAuthEnvironmentDraft({
          preset: "custom",
          opencsgBaseURL: "https://east.opencsg.com",
        }),
      ),
    ).toBe("east.opencsg.com");
    expect(authEnvironmentDisplayLabel(normalizeAuthEnvironmentDraft({ preset: "custom" }), "Custom environment")).toBe(
      "Custom environment",
    );
  });
});
