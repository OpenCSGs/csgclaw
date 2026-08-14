import {
  classifyDesktopUpdateErrorKind,
  formatClassifiedUpgradeError,
  formatSidebarVersionLabel,
  hasUpgradeAttention,
  inferUpgradeChannelFromVersion,
  isLocalBuildUpgradeStatus,
  isLocalBuildVersion,
  normalizeUpgradeStatus,
  upgradeErrorMessage,
  upgradeStatusLabel,
} from "@/models/upgradeStatus";
import type { UpgradeStatus } from "@/models/upgradeStatus";

describe("upgrade status helpers", () => {
  it("formats sidebar versions as plain semver labels", () => {
    expect(formatSidebarVersionLabel("")).toBe("dev");
    expect(formatSidebarVersionLabel("v0.2.1")).toBe("v0.2.1");
    expect(formatSidebarVersionLabel("0.2.1")).toBe("v0.2.1");
  });

  it("infers the update channel from the running version string", () => {
    expect(inferUpgradeChannelFromVersion("0.0.1")).toBe("release");
    expect(inferUpgradeChannelFromVersion("v0.2.0")).toBe("release");
    expect(inferUpgradeChannelFromVersion("V0.2.0")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("v0.2.0-beta.1")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("v0.2.0-alf")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("0.5.0-beta.2")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("v0.2.0.1")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("v0.2")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("v0.4.6-dev2g6c80d157dirty")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("dev")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("")).toBe("beta");
    expect(inferUpgradeChannelFromVersion("v01.0.0")).toBe("beta");
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
        downloaded: 1,
        last_checked_at: 123,
        last_error: 404,
        last_error_kind: "network_download",
        last_error_log_path: "/tmp/upgrade.log",
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
      channel: "release",
      checking: true,
      current_version: "v0.2.0",
      downloaded: true,
      last_checked_at: 123,
      last_error: "",
      last_error_kind: "network_download",
      last_error_log_path: "/tmp/upgrade.log",
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

  it("formats classified upgrade errors through translations", () => {
    const t = (key: string, params: Record<string, string | number> = {}) => {
      const labels: Record<string, string> = {
        upgradeErrorDetails: "Details: {detail}",
        upgradeErrorLocalInstall: "Local install is abnormal.",
        upgradeErrorLogPath: "Log: {path}",
        upgradeErrorNetworkOrService: "Network or service failed.",
      };
      return (labels[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
    };

    expect(
      upgradeErrorMessage(
        {
          ...baseUpgradeStatus,
          last_error: "write archive: stream error",
          last_error_kind: "network_download",
          last_error_log_path: "/tmp/upgrade-helper.log",
        },
        t,
      ),
    ).toBe("Network or service failed.\nDetails: write archive: stream error\nLog: /tmp/upgrade-helper.log");
    expect(
      upgradeErrorMessage(
        {
          ...baseUpgradeStatus,
          last_error: "fetch latest release metadata: unexpected status 503",
          last_error_kind: "http_metadata",
        },
        t,
      ),
    ).toBe("Network or service failed.\nDetails: fetch latest release metadata: unexpected status 503");
    expect(
      upgradeErrorMessage(
        {
          ...baseUpgradeStatus,
          last_error_kind: "missing_path",
        },
        t,
      ),
    ).toBe("Local install is abnormal.");
  });

  it("classifies desktop update failures for unsigned packages and missing feeds", () => {
    const t = (key: string) => key;

    expect(classifyDesktopUpdateErrorKind("The update is improperly signed")).toBe("signature");
    expect(
      classifyDesktopUpdateErrorKind(
        "The release channel does not provide a signed macOS auto-update package for 0.4.6. HTTP 404",
      ),
    ).toBe("missing_update_package");
    expect(classifyDesktopUpdateErrorKind("网络连接已中断。")).toBe("network_download");
    expect(classifyDesktopUpdateErrorKind("current installation is not an official csgclaw bundle")).toBe(
      "local_build",
    );
    expect(formatClassifiedUpgradeError("preview feed unavailable", t)).toBe("preview feed unavailable");
    expect(formatClassifiedUpgradeError("current installation is not an official csgclaw bundle", t)).toBe(
      "upgradeLocalBuildUnsupported",
    );
    expect(
      upgradeErrorMessage(
        {
          ...baseUpgradeStatus,
          last_error: "The update is improperly signed",
          last_error_kind: "desktop_update",
        },
        t,
      ),
    ).toBe("upgradeErrorSignature");
    expect(
      upgradeErrorMessage(
        {
          ...baseUpgradeStatus,
          last_error: "The release channel does not provide a signed macOS auto-update package for 0.4.6. HTTP 404",
          last_error_kind: "desktop_update",
        },
        t,
      ),
    ).toBe("upgradeErrorMissingUpdatePackage");
  });

  it("keeps log paths visible for unclassified upgrade errors", () => {
    const t = (key: string, params: Record<string, string | number> = {}) =>
      key === "upgradeErrorLogPath" ? `Log: ${params.path}` : key;

    expect(
      upgradeErrorMessage(
        {
          ...baseUpgradeStatus,
          last_error: "restart daemon: boom",
          last_error_log_path: "/tmp/upgrade-helper.log",
        },
        t,
      ),
    ).toBe("restart daemon: boom\nLog: /tmp/upgrade-helper.log");
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
  channel: "release",
  checking: false,
  current_version: "v0.2.0",
  downloaded: false,
  last_checked_at: "",
  last_error: "",
  last_error_kind: "",
  last_error_log_path: "",
  latest_version: "v0.2.0",
  manual_restart_required: false,
  update_available: false,
  upgrading: false,
};
