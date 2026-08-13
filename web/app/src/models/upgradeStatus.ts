// API returns Version from git describe (e.g. "v0.2.1-5-gabc-dirty") or "dev"; keep the UI label plain.
import type { TranslateFn } from "@/models/conversations";

export type UpgradeChannel = "release" | "beta";

const STABLE_RELEASE_VERSION_PATTERN = /^(?:v)?\d+\.\d+\.\d+$/i;

export function inferUpgradeChannelFromVersion(version: unknown): UpgradeChannel {
  const raw = typeof version === "string" ? version.trim() : "";
  return raw.length > 0 && STABLE_RELEASE_VERSION_PATTERN.test(raw) ? "release" : "beta";
}

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

export function classifyDesktopUpdateErrorKind(message: string): string {
  const text = message.trim();
  if (!text) {
    return "";
  }
  if (
    /does not provide a signed|HTTP 404|NoSuchKey|specified key does not exist|update feed did not offer|did not offer version/i.test(
      text,
    )
  ) {
    return "missing_update_package";
  }
  if (
    /code sign|codesign|code signature|not signed|unsigned|improperly signed|Developer ID|notar|Gatekeeper|spctl/i.test(
      text,
    )
  ) {
    return "signature";
  }
  if (
    /网络连接已中断|net::ERR_|ERR_CONNECTION|ENOTFOUND|ECONNRESET|ETIMEDOUT|timed out|offline|interrupted|Failed to download/i.test(
      text,
    )
  ) {
    return "network_download";
  }
  return "desktop_update";
}

export function upgradeErrorMessage(status: UpgradeStatus | null | undefined, t: TranslateFn): string {
  if (!status?.last_error && !status?.last_error_kind && !status?.last_error_log_path) {
    return "";
  }
  const detail = status.last_error.trim();
  const logPath = status.last_error_log_path.trim();
  let kind = status.last_error_kind.trim();
  if (kind === "desktop_update") {
    kind = classifyDesktopUpdateErrorKind(detail) || kind;
  } else if (!kind) {
    const classified = classifyDesktopUpdateErrorKind(detail);
    if (classified && classified !== "desktop_update") {
      kind = classified;
    }
  }
  if (!kind) {
    return [detail, logPath ? t("upgradeErrorLogPath", { path: logPath }) : ""].filter(Boolean).join("\n");
  }

  const parts = [upgradeErrorSummary(kind, t)];
  if (detail && kind !== "signature" && kind !== "missing_update_package") {
    parts.push(t("upgradeErrorDetails", { detail }));
  }
  if (logPath) {
    parts.push(t("upgradeErrorLogPath", { path: logPath }));
  }
  return parts.filter(Boolean).join("\n");
}

export function formatClassifiedUpgradeError(detail: string, t: TranslateFn): string {
  return upgradeErrorMessage(
    {
      auto_upgrade_supported: true,
      auto_upgrade_unsupported_reason: "",
      channel: "release",
      checking: false,
      current_version: "",
      downloaded: false,
      last_checked_at: "",
      last_error: detail.trim(),
      last_error_kind: classifyDesktopUpdateErrorKind(detail) || "desktop_update",
      last_error_log_path: "",
      latest_version: "",
      manual_restart_required: false,
      update_available: false,
      upgrading: false,
    },
    t,
  );
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
    case "missing_update_package":
      return t("upgradeErrorMissingUpdatePackage");
    case "permission":
      return t("upgradeErrorPermission");
    case "signature":
      return t("upgradeErrorSignature");
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
