import type { AppInstallation, CLIIdentity } from "@/api/apps";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";

export function appStatus(app: AppInstallation, t: TranslateFn): string {
  if (!app.enabled || app.resource_enabled === false) return t("appStatusDisabled");
  if (app.disconnected) return t(app.kind === "cli" ? "appStatusNeedsConfiguration" : "appStatusDisconnected");
  const keys = {
    not_connected: "appStatusNotConnected",
    applying: "feishuCLIApplying",
    configured: "appStatusConfigured",
    needs_configuration: "appStatusNeedsConfiguration",
    connecting: "appStatusConnecting",
    connected: "appStatusConnected",
    error: "appStatusError",
    authorization_required: "appStatusAuthorizationRequired",
    disconnected: "appStatusDisconnected",
    disabled: "appStatusDisabled",
  };
  return t(keys[app.status] ?? "appStatusNeedsConfiguration");
}

export function appConnectionPending(app: AppInstallation): boolean {
  return (
    app.status === "connecting" ||
    app.status === "applying" ||
    app.operation?.status === "running" ||
    app.operation?.status === "pending"
  );
}

export function appConnectionError(app: AppInstallation, t: TranslateFn): string {
  if (appConnectionPending(app) || (!app.last_error_code && !app.last_error)) return "";
  if (
    !app.last_error_code &&
    app.kind === "cli" &&
    app.operation?.status === "cancelled" &&
    !app.operation.error_code &&
    app.status !== "error" &&
    app.status !== "authorization_required"
  ) {
    return "";
  }
  const message = localizeAPIError(
    { code: app.last_error_code, message: app.last_error },
    t,
    app.kind === "cli" ? t("errors.cli_operation_failed") : "",
  );
  return app.last_error_code && app.last_error_http_status
    ? `${message} (HTTP ${app.last_error_http_status})`
    : message;
}

export function cliIdentityAuthorized(identity: CLIIdentity | undefined): boolean {
  return Boolean(identity?.available && identity.verified === true);
}

export function appAuthorizationSummary(app: AppInstallation, t: TranslateFn): string {
  if (
    app.kind !== "cli" ||
    app.app_id !== "feishu" ||
    !app.enabled ||
    app.resource_enabled === false ||
    app.disconnected ||
    !app.authorized ||
    app.status !== "connected" ||
    !app.auth_status?.identities
  ) {
    return "";
  }
  const { bot, user } = app.auth_status.identities;
  return t("feishuCLIAuthorizationSummary", {
    bot: t(cliIdentityAuthorized(bot) ? "feishuCLIAvailable" : "feishuCLIUnauthorized"),
    user: t(cliIdentityAuthorized(user) ? "feishuCLIAvailable" : "feishuCLIUnauthorized"),
  });
}
