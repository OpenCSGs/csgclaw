import { hasUpgradeAttention, type UpgradePhase, type UpgradeStatus } from "@/models/upgradeStatus";

export function shouldShowUpgradeAlertDot({
  busy = false,
  controlsAvailable,
  phase,
  status,
}: {
  busy?: boolean;
  controlsAvailable: boolean;
  phase: UpgradePhase;
  status: UpgradeStatus | null | undefined;
}): boolean {
  if (!controlsAvailable || phase === "done") {
    return false;
  }
  return hasUpgradeAttention(status, phase, busy);
}
