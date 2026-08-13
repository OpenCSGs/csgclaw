import { render, screen } from "@testing-library/react";
import { UpgradeModal } from "@/pages/WorkspacePage/components/WorkspaceModals/UpgradeModal";
import type { UpgradeStatus } from "@/models/upgradeStatus";

const labels: Record<string, string> = {
  close: "Close",
  upgradeChannel: "Update channel",
  upgradeChannelBeta: "Preview",
  upgradeChannelRelease: "Stable",
  upgradeCurrentVersion: "Current version",
  upgradeLatestVersion: "Latest version",
  upgradeManualUpgradeBody: "Use the official installer or release bundle to upgrade manually.",
  upgradeManualUpgradeSubtitle: "Manual upgrade is required for this installation.",
  upgradeErrorDetails: "Details: {detail}",
  upgradeErrorLogPath: "Log: {path}",
  upgradeErrorNetworkOrService: "Network or service issue.",
  upgradeNoLatest: "Unknown",
  upgradeStatus: "Status",
  upgradeStatusManualUpgrade: "Manual upgrade required",
  upgradeSubtitle: "Upgrade directly from the app.",
  upgradeTitle: "New version available",
};

function t(key: string, params: Record<string, string | number> = {}): string {
  return (labels[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
}

const manualUpgradeStatus: UpgradeStatus = {
  auto_upgrade_supported: false,
  auto_upgrade_unsupported_reason: "not_official_bundle",
  channel: "release",
  checking: false,
  current_version: "v0.3.10",
  downloaded: false,
  last_checked_at: "",
  last_error: "",
  last_error_kind: "",
  last_error_log_path: "",
  latest_version: "v0.3.11",
  manual_restart_required: false,
  update_available: true,
  upgrading: false,
};

describe("UpgradeModal", () => {
  it("uses manual upgrade copy for non-official installs", () => {
    render(
      <UpgradeModal
        onApply={() => {}}
        onClose={() => {}}
        t={t}
        upgradePhase="idle"
        upgradeStatus={manualUpgradeStatus}
      />,
    );

    expect(screen.getByText("Manual upgrade is required for this installation.")).toBeInTheDocument();
    expect(screen.getByText("Use the official installer or release bundle to upgrade manually.")).toBeInTheDocument();
    expect(screen.getByText("Manual upgrade required")).toBeInTheDocument();
    expect(screen.queryByText("Upgrade directly from the app.")).not.toBeInTheDocument();
  });

  it("renders localized classified upgrade errors with log path", () => {
    render(
      <UpgradeModal
        onApply={() => {}}
        onClose={() => {}}
        t={t}
        upgradePhase="error"
        upgradeStatus={{
          ...manualUpgradeStatus,
          auto_upgrade_supported: true,
          last_error: "write archive: stream error",
          last_error_kind: "network_download",
          last_error_log_path: "/tmp/upgrade-helper.log",
        }}
      />,
    );

    expect(screen.getByText(/Network or service issue/)).toBeInTheDocument();
    expect(screen.getByText(/Details: write archive: stream error/)).toBeInTheDocument();
    expect(screen.getByText(/Log: \/tmp\/upgrade-helper.log/)).toBeInTheDocument();
  });

  it("keeps channel switching out of the confirm-upgrade dialog", () => {
    render(
      <UpgradeModal
        onApply={() => {}}
        onClose={() => {}}
        t={t}
        upgradePhase="idle"
        upgradeStatus={{
          ...manualUpgradeStatus,
          auto_upgrade_supported: true,
          channel: "beta",
          current_version: "v0.2.1-beta.1",
          downloaded: true,
          latest_version: "v0.5.0-beta.7",
        }}
      />,
    );

    expect(screen.queryByText("Update channel")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Stable" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Preview" })).not.toBeInTheDocument();
  });
});
