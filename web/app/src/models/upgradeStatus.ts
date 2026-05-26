// API returns Version from git describe (e.g. "v0.2.1-5-gabc-dirty") or "dev"; keep the UI label plain.
import type { TranslateFn } from "@/models/conversations";

export type UpgradeStatus = {
  checking: boolean;
  current_version: string;
  last_checked_at: unknown;
  last_error: string;
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

export function normalizeUpgradeStatus(status: unknown): UpgradeStatus | null {
  if (!status || typeof status !== "object") {
    return null;
  }
  const source = status as Partial<Record<keyof UpgradeStatus, unknown>>;
  return {
    current_version: typeof source.current_version === "string" ? source.current_version : "",
    latest_version: typeof source.latest_version === "string" ? source.latest_version : "",
    update_available: Boolean(source.update_available),
    checking: Boolean(source.checking),
    upgrading: Boolean(source.upgrading),
    last_checked_at: source.last_checked_at || "",
    last_error: typeof source.last_error === "string" ? source.last_error : "",
    manual_restart_required: Boolean(source.manual_restart_required),
  };
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
