import { describe, expect, it } from "vitest";
import type { AppDefinition, AppInstallation } from "@/api/apps";
import { appFormPayload, appStatus, initialAppForm } from "./appForm";

const definition: AppDefinition = {
  app_id: "gitlab",
  name: "GitLab",
  description: "",
  version: "1",
  interface: {},
  config_schema: {},
  auth_methods: ["bearer"],
  oauth_supported: false,
};

const installation: AppInstallation = {
  installation_id: "app-1",
  agent_id: "agent-1",
  app_id: "gitlab",
  name: "Work",
  config: { transport: "http", url: "http://localhost/mcp" },
  enabled: true,
  disconnected: false,
  status: "connected",
  credentials_set: { token: true },
  tools: [],
  created_at: "",
  updated_at: "",
};

describe("App settings payload", () => {
  it("keeps secret values out of the displayable configuration and preserves argument boundaries", () => {
    const form = initialAppForm(definition, null, false);
    form.config = { ...form.config, transport: "stdio", url: "https://previous.example/mcp", command: "npx" };
    form.args = "--endpoint\nhttps://service.example/a b\n--label=team one\n";
    form.env = [{ key: "API_KEY", value: "private-key" }];
    form.credentials = { token: "" };
    const payload = appFormPayload(form);
    expect(payload.config.url).toBeUndefined();
    expect(payload.config.args).toEqual(["--endpoint", "https://service.example/a b", "--label=team one"]);
    expect(payload.config.env).toBeUndefined();
    expect(payload.credentials.env).toEqual({ API_KEY: "private-key" });
    expect(payload.credentials).not.toHaveProperty("token");
  });

  it("references the existing Feishu channel without copying credentials", () => {
    const form = initialAppForm({ ...definition, app_id: "feishu" }, null, true);
    expect(form.config.credential_source).toBe("feishu_channel");
    expect(form.config.auth_mode).toBe("feishu");
    expect(form.credentials).toEqual({});
    expect(initialAppForm({ ...definition, app_id: "feishu" }, null, false).config.credential_source).toBe("manual");
  });

  it("does not restore persisted secret values into an editing form", () => {
    const existing = installation;
    expect(initialAppForm(definition, existing, false).credentials).toEqual({});
  });

  it("shows a persisted disconnect or disable intent ahead of old connection status", () => {
    const existing = { ...installation, disconnected: true };
    expect(appStatus(existing, (key) => key)).toBe("appStatusDisconnected");
    expect(appStatus({ ...existing, enabled: false }, (key) => key)).toBe("appStatusDisabled");
  });
});

describe("Shared OpenCSG platform credentials", () => {
  it.each(["gitlab", "feishu", "llm-wiki"])("defaults %s to login for OpenCSG URLs", (appID) => {
    const form = initialAppForm({ ...definition, app_id: appID }, null, false);
    form.config.url = "https://demo.public.opencsg-stg.com/mcp";
    expect(appFormPayload(form).config.platform_credential_source).toBe("opencsg_login");
    form.config.platform_credential_source = "manual";
    form.credentials.token = "explicit-token";
    expect(appFormPayload(form).credentials.token).toBe("explicit-token");
    expect(appFormPayload(form).config.platform_credential_source).toBe("manual");
  });

  it.each([
    "http://localhost/mcp",
    "https://example.com/mcp",
    "https://demo.public.opencsg-stg.com.evil.example/mcp",
    "https://demo.public.opencsg-stg.com:8443/mcp",
  ])("does not send login to %s", (url) => {
    const form = initialAppForm(definition, null, false);
    form.config.url = url;
    expect(appFormPayload(form).config.platform_credential_source).toBe("manual");
  });

  it("preserves a business API key alongside platform login", () => {
    const form = initialAppForm(definition, null, false);
    form.config = {
      transport: "http",
      auth_mode: "header",
      token_header: "PRIVATE-TOKEN",
      url: "https://demo.public.opencsg-stg.com/mcp",
    };
    form.credentials.token = "business-fixture";
    const result = appFormPayload(form);
    expect(result.config.platform_credential_source).toBe("opencsg_login");
    expect(result.credentials.token).toBe("business-fixture");
  });
});

describe("Catalog connection defaults", () => {
  const configured: AppDefinition = {
    ...definition,
    config_schema: {
      properties: {
        url: { type: "string", default: "https://mcp.example.test/gitlab" },
        auth_mode: { type: "string", default: "header" },
        token_header: { type: "string", default: "PRIVATE-TOKEN" },
        token_prefix: { type: "string", default: "" },
        startup_timeout_sec: { type: "integer", default: 45 },
        token: { type: "string", writeOnly: true, default: "not-a-default-secret" },
        env: { default: { TOKEN: "not-a-default-secret" } },
      },
    },
  };
  it("fills non-secret connection defaults and submits edited values", () => {
    const form = initialAppForm(configured, null, false);
    expect(form.config).toMatchObject({
      url: "https://mcp.example.test/gitlab",
      auth_mode: "header",
      token_header: "PRIVATE-TOKEN",
      token_prefix: "",
      startup_timeout_sec: 45,
      tool_timeout_sec: 60,
    });
    expect(form.credentials).toEqual({});
    expect(form.config).not.toHaveProperty("token");
    expect(form.config.env).toBeUndefined();
    form.config.url = "http://localhost:9999/mcp";
    expect(appFormPayload(form).config.url).toBe("http://localhost:9999/mcp");
  });
  it("does not replace saved configuration with newer catalog defaults", () => {
    expect(initialAppForm(configured, installation, false).config).toEqual(installation.config);
  });
});
