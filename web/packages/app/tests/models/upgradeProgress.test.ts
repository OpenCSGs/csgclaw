import { nextSimulatedUpgradeProgress } from "@/models/upgradeProgress";

describe("simulated upgrade progress", () => {
  it("advances quickly at first and never claims completion before the download finishes", () => {
    expect(nextSimulatedUpgradeProgress(3)).toBe(10);
    expect(nextSimulatedUpgradeProgress(20)).toBe(24);
    expect(nextSimulatedUpgradeProgress(60)).toBe(62);
    expect(nextSimulatedUpgradeProgress(85)).toBe(86);
    expect(nextSimulatedUpgradeProgress(94)).toBe(94);
  });
});
