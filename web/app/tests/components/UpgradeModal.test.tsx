import { render, screen } from "@testing-library/react";
import { UpgradeModal } from "@/pages/WorkspacePage/components/WorkspaceModals/UpgradeModal";
import type { UpgradeStatus } from "@/models/upgradeStatus";

const labels: Record<string, string> = {
  close: "Close",
  upgradeCurrentVersion: "Current version",
  upgradeLatestVersion: "Latest version",
  upgradeManualUpgradeBody: "Use the official installer or release bundle to upgrade manually.",
  upgradeManualUpgradeSubtitle: "Manual upgrade is required for this installation.",
  upgradeNoLatest: "Unknown",
  upgradeStatus: "Status",
  upgradeStatusManualUpgrade: "Manual upgrade required",
  upgradeSubtitle: "Upgrade directly from the app.",
  upgradeTitle: "New version available",
};

function t(key: string): string {
  return labels[key] ?? key;
}

const manualUpgradeStatus: UpgradeStatus = {
  auto_upgrade_supported: false,
  auto_upgrade_unsupported_reason: "not_official_bundle",
  checking: false,
  current_version: "v0.3.10",
  last_checked_at: "",
  last_error: "",
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
});
