import { describe, expect, it } from "vitest";
import type { UpgradeStatus } from "@/models/upgradeStatus";
import { shouldShowUpgradeAlertDot } from "./SidebarUserButton";

function upgradeStatus(overrides: Partial<UpgradeStatus> = {}): UpgradeStatus {
  return {
    auto_upgrade_supported: true,
    auto_upgrade_unsupported_reason: "",
    checking: false,
    current_version: "v0.3.18",
    latest_version: "v0.3.19",
    last_checked_at: "",
    last_error: "",
    last_error_kind: "",
    last_error_log_path: "",
    manual_restart_required: false,
    update_available: true,
    upgrading: false,
    ...overrides,
  };
}

describe("shouldShowUpgradeAlertDot", () => {
  it("shows the settings red dot while an update is available", () => {
    expect(
      shouldShowUpgradeAlertDot({
        controlsAvailable: true,
        phase: "idle",
        status: upgradeStatus({ update_available: true }),
      }),
    ).toBe(true);
  });

  it("hides the settings red dot after the upgrade is done", () => {
    expect(
      shouldShowUpgradeAlertDot({
        controlsAvailable: true,
        phase: "done",
        status: upgradeStatus({ current_version: "v0.3.19", latest_version: "v0.3.19", update_available: false }),
      }),
    ).toBe(false);
  });

  it("hides the settings red dot when already up to date", () => {
    expect(
      shouldShowUpgradeAlertDot({
        controlsAvailable: true,
        phase: "idle",
        status: upgradeStatus({ current_version: "v0.3.19", latest_version: "v0.3.19", update_available: false }),
      }),
    ).toBe(false);
  });
});
