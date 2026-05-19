import { formatSidebarVersionLabel, normalizeUpgradeStatus, upgradeStatusLabel } from "@/models/upgradeStatus";

describe("upgrade status helpers", () => {
  it("formats sidebar versions without duplicating the v prefix", () => {
    expect(formatSidebarVersionLabel("")).toBe("csgclaw dev");
    expect(formatSidebarVersionLabel("v0.2.1")).toBe("csgclaw v0.2.1");
    expect(formatSidebarVersionLabel("0.2.1")).toBe("csgclaw v0.2.1");
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
        update_available: true,
        upgrading: "",
      }),
    ).toEqual({
      checking: true,
      current_version: "v0.2.0",
      last_checked_at: 123,
      last_error: "",
      latest_version: "v0.2.1",
      update_available: true,
      upgrading: false,
    });
  });

  it("maps upgrade phases through translated labels", () => {
    const t = (key: string) => `label:${key}`;

    expect(upgradeStatusLabel("starting", t)).toBe("label:upgradeStatusStarting");
    expect(upgradeStatusLabel("restarting", t)).toBe("label:upgradeStatusRestarting");
    expect(upgradeStatusLabel("done", t)).toBe("label:upgradeStatusDone");
    expect(upgradeStatusLabel("error", t)).toBe("label:upgradeStatusError");
    expect(upgradeStatusLabel("idle", t)).toBe("label:upgradeStatusReady");
  });
});
