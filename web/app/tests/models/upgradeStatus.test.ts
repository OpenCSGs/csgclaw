import {
  formatSidebarVersionLabel,
  hasUpgradeAttention,
  isLocalBuildUpgradeStatus,
  isLocalBuildVersion,
  normalizeUpgradeStatus,
  upgradeStatusLabel,
} from "@/models/upgradeStatus";
import type { UpgradeStatus } from "@/models/upgradeStatus";

describe("upgrade status helpers", () => {
  it("formats sidebar versions as plain semver labels", () => {
    expect(formatSidebarVersionLabel("")).toBe("dev");
    expect(formatSidebarVersionLabel("v0.2.1")).toBe("v0.2.1");
    expect(formatSidebarVersionLabel("0.2.1")).toBe("v0.2.1");
  });

  it("detects local build versions and unsupported local installs", () => {
    expect(isLocalBuildVersion("v0.3.10-21-g4dd4395-dirty")).toBe(true);
    expect(isLocalBuildVersion("v0.3.10-21-g4dd4395")).toBe(true);
    expect(isLocalBuildVersion("v0.3.10+local")).toBe(true);
    expect(isLocalBuildVersion("dev")).toBe(true);
    expect(isLocalBuildVersion("v0.3.11")).toBe(false);
    expect(isLocalBuildUpgradeStatus({ ...baseUpgradeStatus, auto_upgrade_supported: false }, "v0.3.11")).toBe(false);
    expect(
      isLocalBuildUpgradeStatus(
        { ...baseUpgradeStatus, auto_upgrade_supported: false, auto_upgrade_unsupported_reason: "not_official_bundle" },
        "v0.3.11",
      ),
    ).toBe(false);
    expect(
      isLocalBuildUpgradeStatus(
        { ...baseUpgradeStatus, auto_upgrade_supported: false, auto_upgrade_unsupported_reason: "local_build" },
        "v0.3.11",
      ),
    ).toBe(true);
  });

  it("normalizes loose upgrade status payloads", () => {
    expect(normalizeUpgradeStatus(null)).toBeNull();
    expect(
      normalizeUpgradeStatus({
        checking: 1,
        current_version: "v0.2.0",
        last_checked_at: 123,
        last_error: 404,
        latest_version: "v0.2.1",
        manual_restart_required: "yes",
        auto_upgrade_supported: false,
        auto_upgrade_unsupported_reason: "not_official_bundle",
        update_available: true,
        upgrading: "",
      }),
    ).toEqual({
      auto_upgrade_supported: false,
      auto_upgrade_unsupported_reason: "not_official_bundle",
      checking: true,
      current_version: "v0.2.0",
      last_checked_at: 123,
      last_error: "",
      latest_version: "v0.2.1",
      manual_restart_required: true,
      update_available: true,
      upgrading: false,
    });
  });

  it("defaults missing auto-upgrade support to true for older servers", () => {
    expect(normalizeUpgradeStatus({ current_version: "v0.2.0" })?.auto_upgrade_supported).toBe(true);
  });

  it("maps upgrade phases through translated labels", () => {
    const t = (key: string) => `label:${key}`;

    expect(upgradeStatusLabel("starting", t)).toBe("label:upgradeStatusStarting");
    expect(upgradeStatusLabel("restarting", t)).toBe("label:upgradeStatusRestarting");
    expect(upgradeStatusLabel("manual_restart", t)).toBe("label:upgradeStatusManualRestart");
    expect(upgradeStatusLabel("done", t)).toBe("label:upgradeStatusDone");
    expect(upgradeStatusLabel("error", t)).toBe("label:upgradeStatusError");
    expect(upgradeStatusLabel("idle", t)).toBe("label:upgradeStatusReady");
  });

  it("detects upgrade states that need settings attention", () => {
    expect(hasUpgradeAttention(null, "idle")).toBe(false);
    expect(hasUpgradeAttention({ ...baseUpgradeStatus, update_available: true }, "idle")).toBe(true);
    expect(hasUpgradeAttention({ ...baseUpgradeStatus, upgrading: true }, "idle")).toBe(true);
    expect(hasUpgradeAttention({ ...baseUpgradeStatus, manual_restart_required: true }, "idle")).toBe(true);
    expect(hasUpgradeAttention({ ...baseUpgradeStatus, last_error: "boom" }, "idle")).toBe(false);
    expect(hasUpgradeAttention(baseUpgradeStatus, "manual_restart")).toBe(true);
    expect(hasUpgradeAttention(baseUpgradeStatus, "done")).toBe(true);
    expect(hasUpgradeAttention(baseUpgradeStatus, "error")).toBe(true);
    expect(hasUpgradeAttention(baseUpgradeStatus, "idle", true)).toBe(true);
  });
});

const baseUpgradeStatus: UpgradeStatus = {
  auto_upgrade_supported: true,
  auto_upgrade_unsupported_reason: "",
  checking: false,
  current_version: "v0.2.0",
  last_checked_at: "",
  last_error: "",
  latest_version: "v0.2.0",
  manual_restart_required: false,
  update_available: false,
  upgrading: false,
};
