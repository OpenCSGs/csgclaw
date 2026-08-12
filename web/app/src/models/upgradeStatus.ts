// API returns Version from git describe (e.g. "v0.2.1-5-gabc-dirty") or "dev"; keep the UI label plain.
import type { TranslateFn } from "@/models/conversations";

export type UpgradeChannel = "release" | "beta";

export type UpgradeStatus = {
  auto_upgrade_supported: boolean;
  auto_upgrade_unsupported_reason: string;
  channel: UpgradeChannel;
  checking: boolean;
  current_version: string;
  downloaded: boolean;
  last_checked_at: unknown;
  last_error: string;
  last_error_kind: string;
  last_error_log_path: string;
  latest_version: string;
  manual_restart_required: boolean;
  update_available: boolean;
  upgrading: boolean;
};

export type UpgradePhase = "idle" | "starting" | "restarting" | "manual_restart" | "done" | "error";

export function formatSidebarVersionLabel(version: unknown): string {
  const raw = typeof version === "string" ? version.trim() : "";
  if (!raw) {
    return "dev";
  }
  return raw.startsWith("v") ? raw : `v${raw}`;
}

export function isLocalBuildVersion(version: unknown): boolean {
  const raw = typeof version === "string" ? version.trim() : "";
  return (
    raw.length > 0 &&
    (raw === "dev" || raw.endsWith("+local") || raw.endsWith("-dirty") || /-\d+-g[0-9a-f]+/i.test(raw))
  );
}

export function isLocalBuildUpgradeStatus(status: UpgradeStatus | null | undefined, version: unknown): boolean {
  return (
    isLocalBuildVersion(version) ||
    Boolean(
      status && status.auto_upgrade_supported === false && status.auto_upgrade_unsupported_reason === "local_build",
    )
  );
}

export function normalizeUpgradeStatus(status: unknown): UpgradeStatus | null {
  if (!status || typeof status !== "object") {
    return null;
  }
  const source = status as Partial<Record<keyof UpgradeStatus, unknown>>;
  return {
    auto_upgrade_supported: source.auto_upgrade_supported !== false,
    auto_upgrade_unsupported_reason:
      typeof source.auto_upgrade_unsupported_reason === "string" ? source.auto_upgrade_unsupported_reason : "",
    channel: source.channel === "beta" ? "beta" : "release",
    current_version: typeof source.current_version === "string" ? source.current_version : "",
    downloaded: Boolean(source.downloaded),
    latest_version: typeof source.latest_version === "string" ? source.latest_version : "",
    update_available: Boolean(source.update_available),
    checking: Boolean(source.checking),
    upgrading: Boolean(source.upgrading),
    last_checked_at: source.last_checked_at || "",
    last_error: typeof source.last_error === "string" ? source.last_error : "",
    last_error_kind: typeof source.last_error_kind === "string" ? source.last_error_kind : "",
    last_error_log_path: typeof source.last_error_log_path === "string" ? source.last_error_log_path : "",
    manual_restart_required: Boolean(source.manual_restart_required),
  };
}

export function upgradeErrorMessage(status: UpgradeStatus | null | undefined, t: TranslateFn): string {
  if (!status?.last_error && !status?.last_error_kind && !status?.last_error_log_path) {
    return "";
  }
  const detail = status.last_error.trim();
  const logPath = status.last_error_log_path.trim();
  const kind = status.last_error_kind.trim();
  if (!kind) {
    return [detail, logPath ? t("upgradeErrorLogPath", { path: logPath }) : ""].filter(Boolean).join("\n");
  }

  const parts = [upgradeErrorSummary(kind, t)];
  if (detail) {
    parts.push(t("upgradeErrorDetails", { detail }));
  }
  if (logPath) {
    parts.push(t("upgradeErrorLogPath", { path: logPath }));
  }
  return parts.filter(Boolean).join("\n");
}

function upgradeErrorSummary(kind: string, t: TranslateFn): string {
  switch (kind) {
    case "archive_invalid":
      return t("upgradeErrorArchiveInvalid");
    case "disk_space":
      return t("upgradeErrorDiskSpace");
    case "http_asset":
    case "http_metadata":
    case "network_download":
    case "network_check":
      return t("upgradeErrorNetworkOrService");
    case "missing_path":
      return t("upgradeErrorLocalInstall");
    case "permission":
      return t("upgradeErrorPermission");
    default:
      return t("upgradeErrorUnknown");
  }
}

export function upgradeStatusLabel(phase: UpgradePhase, t: TranslateFn): string {
  switch (phase) {
    case "starting":
      return t("upgradeStatusStarting");
    case "restarting":
      return t("upgradeStatusRestarting");
    case "manual_restart":
      return t("upgradeStatusManualRestart");
    case "done":
      return t("upgradeStatusDone");
    case "error":
      return t("upgradeStatusError");
    default:
      return t("upgradeStatusReady");
  }
}

export function hasUpgradeAttention(
  status: UpgradeStatus | null | undefined,
  phase: UpgradePhase,
  busy = false,
): boolean {
  return Boolean(
    status?.update_available ||
    status?.upgrading ||
    status?.manual_restart_required ||
    busy ||
    phase === "manual_restart" ||
    phase === "done" ||
    phase === "error",
  );
}
