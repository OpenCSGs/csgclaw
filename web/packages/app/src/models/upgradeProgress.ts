export function nextSimulatedUpgradeProgress(progress: number): number {
  if (progress < 20) {
    return Math.min(progress + 7, 20);
  }
  if (progress < 60) {
    return Math.min(progress + 4, 60);
  }
  if (progress < 85) {
    return Math.min(progress + 2, 85);
  }
  return Math.min(progress + 1, 94);
}
