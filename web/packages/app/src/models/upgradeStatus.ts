// API returns Version from git describe (e.g. "v0.2.1-5-gabc-dirty") or "dev"; keep the UI label plain.
import type { TranslateFn } from "@/models/conversations";

export type UpgradeChannel = "release" | "beta";

const STABLE_RELEASE_VERSION_PATTERN = /^(?:v)?(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/;

export function inferUpgradeChannelFromVersion(version: unknown): UpgradeChannel {
  const raw = typeof version === "string" ? version.trim() : "";
  return raw.length > 0 && STABLE_RELEASE_VERSION_PATTERN.test(raw) ? "release" : "beta";
}

export type UpgradeStatus = {
  auto_upgrade_supported: boolean;
  auto_upgrade_unsupported_reason: string;
  /** Installed channel derived from current_version, never a pending switch target. */
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

export type UpgradePhase = "idle" | "starting" | "restarting" | "manual_restart" | "done" | "error" | "unconfirmed";

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
  if (/not an official csgclaw bundle|local[_ ]build|本地开发构建/i.test(text)) {
    return "local_build";
  }
  return "desktop_update";
}

export function hasUpgradeStatusError(status: UpgradeStatus | null | undefined): boolean {
  return Boolean(status?.last_error.trim() || status?.last_error_kind.trim() || status?.last_error_log_path.trim());
}

export function upgradeErrorMessage(status: UpgradeStatus | null | undefined, t: TranslateFn): string {
  if (!status || !hasUpgradeStatusError(status)) {
    return "";
  }
  const reportedKind = status.last_error_kind.trim();
  const kind =
    !reportedKind || reportedKind === "desktop_update"
      ? classifyDesktopUpdateErrorKind(status.last_error)
      : reportedKind;
  const summary = t(upgradeErrorKey(kind));
  const logPath = status.last_error_log_path.trim();
  return logPath ? `${summary}\n${t("upgradeErrorLogPath", { path: logPath })}` : summary;
}

export function upgradeErrorKey(kind: string, fallback = "upgradeErrorUnknown"): string {
  switch (kind) {
    case "upgrade_preview_only":
      return "upgradePreviewOnly";
    case "archive_invalid":
      return "upgradeErrorArchiveInvalid";
    case "disk_space":
      return "upgradeErrorDiskSpace";
    case "http_asset":
    case "http_metadata":
    case "network_download":
    case "network_check":
      return "upgradeErrorNetworkOrService";
    case "missing_path":
      return "upgradeErrorLocalInstall";
    case "missing_update_package":
      return "upgradeErrorMissingUpdatePackage";
    case "permission":
      return "upgradeErrorPermission";
    case "signature":
      return "upgradeErrorSignature";
    case "local_build":
      return "upgradeLocalBuildUnsupported";
    default:
      return fallback;
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
    case "unconfirmed":
      return t("upgradeStatusUnconfirmed");
    default:
      return t("upgradeStatusReady");
  }
}

export function latestSidebarUpdateVersion({
  appVersion,
  showUpgradeControls,
  upgradePhase,
  upgradeStatus,
}: {
  appVersion: string;
  showUpgradeControls: boolean;
  upgradePhase: UpgradePhase;
  upgradeStatus: UpgradeStatus | null | undefined;
}): string {
  const latestVersion = upgradeStatus?.latest_version?.trim();
  if (
    !showUpgradeControls ||
    upgradePhase === "done" ||
    !upgradeStatus?.update_available ||
    upgradeStatus.auto_upgrade_supported === false ||
    !latestVersion ||
    isLocalBuildUpgradeStatus(upgradeStatus, upgradeStatus.current_version || appVersion)
  ) {
    return "";
  }
  return formatSidebarVersionLabel(latestVersion);
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
    phase === "done",
  );
}
