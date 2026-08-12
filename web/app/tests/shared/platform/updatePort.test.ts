import { desktopUpdateStatus } from "@/shared/platform/updatePort";

describe("desktop update status", () => {
  it("shows the update action as soon as the native updater finds a release", () => {
    expect(
      desktopUpdateStatus({
        state: "available",
        channel: "beta",
        currentVersion: "0.5.0-beta.4",
        availableVersion: "0.5.0-beta.5",
      }),
    ).toMatchObject({
      channel: "beta",
      checking: false,
      downloaded: false,
      latest_version: "0.5.0-beta.5",
      update_available: true,
      upgrading: false,
    });
  });

  it("keeps the progress visible after the user starts downloading", () => {
    expect(
      desktopUpdateStatus({
        state: "downloading",
        channel: "beta",
        currentVersion: "0.5.0-beta.4",
        availableVersion: "0.5.0-beta.5",
      }),
    ).toMatchObject({
      downloaded: false,
      update_available: true,
      upgrading: true,
    });
  });

  it("marks an update ready for an immediate restart after background download", () => {
    expect(
      desktopUpdateStatus({
        state: "downloaded",
        channel: "beta",
        currentVersion: "0.5.0-beta.4",
        availableVersion: "0.5.0-beta.5",
      }),
    ).toMatchObject({
      downloaded: true,
      update_available: true,
      upgrading: false,
    });
  });
});
