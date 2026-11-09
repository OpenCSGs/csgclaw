import { describe, expect, it } from "vitest";
import type { AppInstallation } from "@/api/apps";
import { createTranslator } from "@/shared/i18n";
import { appStatus, appAuthorizationSummary, appConnectionError } from "./appStatus";

const resource: AppInstallation = {
  installation_id: "feishu-cli",
  agent_id: "",
  app_id: "feishu",
  name: "Feishu",
  kind: "cli",
  enabled: true,
  disconnected: false,
  status: "connected",
  authorized: true,
  config: {},
  credentials_set: {},
  tools: [],
  created_at: "",
  updated_at: "",
  auth_status: {
    appId: "cli_test",
    defaultAs: "auto",
    identities: {
      bot: { available: true, verified: true, status: "ready" },
      user: { available: false, status: "pending" },
    },
  },
};
const cancelled: AppInstallation = {
  ...resource,
  last_error: "CLI connector operation could not be completed; retry the operation.",
  operation: {
    operation_id: "connect",
    session_id: "session",
    resource_id: resource.installation_id,
    resource_revision: 1,
    action: "connect",
    stage: "user_authorization",
    status: "cancelled",
  },
};

it("shows disconnect and disable intent ahead of the previous connection status", () => {
  const existing = { ...resource, kind: "mcp" as const, disconnected: true };
  expect(appStatus(existing, (key) => key)).toBe("appStatusDisconnected");
  expect(appStatus({ ...existing, enabled: false }, (key) => key)).toBe("appStatusDisabled");
});

describe("connector errors", () => {
  it.each(["en", "zh"] as const)("keeps healthy and cancelled connectors free of errors in %s", (locale) => {
    const t = createTranslator(locale);
    expect(appConnectionError(resource, t)).toBe("");
    expect(appConnectionError(cancelled, t)).toBe("");
    expect(appConnectionError({ ...cancelled, status: "needs_configuration", authorized: false }, t)).toBe("");
  });

  it.each([
    "authorization_expired",
    "authorization_denied",
    "cleanup_pending",
    "migration_failed",
    "configuration_failed",
    "cli_operation_failed",
    "permission_denied",
  ])("uses the shared translation for %s even without a diagnostic message", (code) => {
    for (const locale of ["en", "zh"] as const) {
      const t = createTranslator(locale);
      const item = { ...resource, status: "error" as const, last_error_code: code };
      expect(t(`errors.${code}`)).not.toBe(`errors.${code}`);
      expect(appConnectionError(item, t)).toBe(t(`errors.${code}`));
    }
  });

  it("preserves errors when cancellation requires recovery", () => {
    const t = createTranslator("zh");
    expect(appConnectionError({ ...cancelled, status: "error", last_error_code: "cleanup_pending" }, t)).toBe(
      "清理尚未完成，请重试断开连接，完成后重新连接。",
    );
    expect(appConnectionError({ ...cancelled, status: "error" }, t)).toBe("连接器操作未完成，请重试。");
    expect(appConnectionError({ ...cancelled, status: "authorization_required" }, t)).toBe(
      "连接器操作未完成，请重试。",
    );
  });

  it.each(["connecting", "applying"] as const)("suppresses previous errors while %s", (status) => {
    expect(
      appConnectionError({ ...cancelled, status, last_error_code: "authorization_expired" }, createTranslator("zh")),
    ).toBe("");
  });

  it.each(["pending", "running"])("suppresses previous errors during a %s operation", (status) => {
    expect(
      appConnectionError(
        { ...cancelled, last_error_code: "authorization_expired", operation: { ...cancelled.operation!, status } },
        createTranslator("zh"),
      ),
    ).toBe("");
  });

  it("preserves MCP diagnostics and HTTP status", () => {
    const item: AppInstallation = {
      ...resource,
      kind: "mcp",
      status: "authorization_required",
      last_error_code: "app_platform_unauthorized",
      last_error_http_status: 401,
    };
    const t = createTranslator("en");
    expect(appConnectionError(item, t)).toBe(`${t("errors.app_platform_unauthorized")} (HTTP 401)`);
    expect(appConnectionError({ ...item, last_error_code: undefined, last_error: "Request timed out" }, t)).toBe(
      "Request timed out",
    );
  });
});

describe("Feishu authorization summaries", () => {
  it.each([
    ["zh", "机器人：已授权 · 个人账号：待授权", "机器人：已授权 · 个人账号：已授权"],
    ["en", "Bot: Authorized · Account: Authorization required", "Bot: Authorized · Account: Authorized"],
  ] as const)("describes both identities in %s", (locale, partial, complete) => {
    const t = createTranslator(locale);
    expect(appAuthorizationSummary(resource, t)).toBe(partial);
    expect(
      appAuthorizationSummary(
        {
          ...resource,
          auth_status: {
            ...resource.auth_status!,
            identities: {
              ...resource.auth_status!.identities,
              user: { available: true, verified: true, status: "ready" },
            },
          },
        },
        t,
      ),
    ).toBe(complete);
  });

  it("requires a verified identity before describing it as authorized", () => {
    expect(
      appAuthorizationSummary(
        {
          ...resource,
          auth_status: {
            ...resource.auth_status!,
            identities: {
              bot: { available: true, status: "unknown" },
              user: { available: true, verified: true, status: "ready" },
            },
          },
        },
        createTranslator("zh"),
      ),
    ).toBe("机器人：待授权 · 个人账号：已授权");
  });

  it.each([
    { kind: "mcp" as const },
    { app_id: "custom-cli" },
    { enabled: false },
    { resource_enabled: false },
    { disconnected: true },
    { authorized: false },
    { status: "connecting" as const },
    { status: "error" as const },
    { auth_status: undefined },
  ])("keeps the normal status for %j", (fields) => {
    expect(appAuthorizationSummary({ ...resource, ...fields }, createTranslator("en"))).toBe("");
  });
});
