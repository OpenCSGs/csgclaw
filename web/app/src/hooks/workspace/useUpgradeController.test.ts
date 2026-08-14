// @vitest-environment jsdom

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyUpgradeRequest, setUpgradeChannelRequest } from "@/api/upgrade";
import type { TranslateFn } from "@/models/conversations";
import type { UpgradeStatus } from "@/models/upgradeStatus";
import { scheduleUpgradePageReload, UPGRADE_PAGE_RELOAD_DELAY_MS, useUpgradeController } from "./useUpgradeController";

vi.mock("@/api/upgrade", () => ({
  applyUpgradeRequest: vi.fn(),
  setUpgradeChannelRequest: vi.fn(),
}));

const t: TranslateFn = (key) => key;
const mockedApplyUpgradeRequest = vi.mocked(applyUpgradeRequest);
const mockedSetUpgradeChannelRequest = vi.mocked(setUpgradeChannelRequest);

function upgradeStatus(overrides: Partial<UpgradeStatus> = {}): UpgradeStatus {
  return {
    auto_upgrade_supported: true,
    auto_upgrade_unsupported_reason: "",
    channel: "release",
    checking: false,
    current_version: "v0.3.18",
    downloaded: false,
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

describe("scheduleUpgradePageReload", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("reloads the page after the upgrade success delay", () => {
    vi.useFakeTimers();
    const reloadPage = vi.fn();

    scheduleUpgradePageReload(reloadPage);

    expect(reloadPage).not.toHaveBeenCalled();
    vi.advanceTimersByTime(UPGRADE_PAGE_RELOAD_DELAY_MS - 1);
    expect(reloadPage).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });
});

describe("useUpgradeController", () => {
  beforeEach(() => {
    mockedApplyUpgradeRequest.mockReset();
    mockedApplyUpgradeRequest.mockResolvedValue(undefined);
    mockedSetUpgradeChannelRequest.mockReset();
    mockedSetUpgradeChannelRequest.mockResolvedValue(upgradeStatus({ channel: "release", update_available: false }));
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("schedules a page reload after clicking apply and detecting the upgraded version", async () => {
    vi.useFakeTimers();
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const setUpgradeStatusData = vi.fn();
    const setAppVersionData = vi.fn();
    const refreshWorkspaceAppVersion = vi.fn().mockResolvedValue("v0.3.19");
    const refreshWorkspaceUpgradeStatus = vi.fn().mockResolvedValue(upgradeStatus());

    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.3.18",
        refreshWorkspaceAppVersion,
        refreshWorkspaceUpgradeStatus,
        setAppVersionData,
        setUpgradeStatusData,
        t,
        upgradeStatus: upgradeStatus(),
      }),
    );

    act(() => {
      result.current.openUpgradeModal();
    });
    expect(result.current.upgradeModalProps).not.toBeNull();

    await act(async () => {
      await result.current.upgradeModalProps?.onApply();
    });

    expect(mockedApplyUpgradeRequest).toHaveBeenCalledTimes(1);
    expect(refreshWorkspaceAppVersion).toHaveBeenCalledWith({ cacheBust: true });
    expect(setAppVersionData).toHaveBeenCalledWith("v0.3.19");
    expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), UPGRADE_PAGE_RELOAD_DELAY_MS);
  });

  it("requests a one-shot server channel switch while keeping the installed channel", async () => {
    const setUpgradeStatusData = vi.fn();
    const stableStatus = upgradeStatus({ update_available: false });
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.3.18",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn(),
        setAppVersionData: vi.fn(),
        setUpgradeStatusData,
        t,
        upgradeStatus: stableStatus,
      }),
    );

    await act(async () => result.current.changeUpgradeChannel("beta"));

    expect(mockedSetUpgradeChannelRequest).toHaveBeenCalledWith("beta");
    expect(setUpgradeStatusData).toHaveBeenCalledWith(expect.objectContaining({ channel: "release" }));
  });

  it("polls for the restarted server after a one-shot channel switch starts", async () => {
    vi.useFakeTimers();
    mockedSetUpgradeChannelRequest.mockResolvedValueOnce(
      upgradeStatus({
        channel: "release",
        latest_version: "v0.4.0-beta.1",
        upgrading: true,
      }),
    );
    const refreshWorkspaceAppVersion = vi.fn().mockResolvedValue("v0.4.0-beta.1");
    const setAppVersionData = vi.fn();
    const setUpgradeStatusData = vi.fn();
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.3.18",
        refreshWorkspaceAppVersion,
        refreshWorkspaceUpgradeStatus: vi.fn(),
        setAppVersionData,
        setUpgradeStatusData,
        t,
        upgradeStatus: upgradeStatus({ update_available: false }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("beta")).resolves.toBe(true);
    });

    expect(refreshWorkspaceAppVersion).toHaveBeenCalledWith({ cacheBust: true });
    expect(setAppVersionData).toHaveBeenCalledWith("v0.4.0-beta.1");
    const updater = setUpgradeStatusData.mock.calls.at(-1)?.[0];
    expect(updater(upgradeStatus({ channel: "release" }))).toEqual(
      expect.objectContaining({
        channel: "beta",
        current_version: "v0.4.0-beta.1",
      }),
    );
    expect(result.current.upgradePhase).toBe("done");
  });

  it("still switches channels when an update is already available", async () => {
    const setUpgradeStatusData = vi.fn();
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.2.0-beta.1",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn(),
        setAppVersionData: vi.fn(),
        setUpgradeStatusData,
        t,
        upgradeStatus: upgradeStatus({
          channel: "beta",
          current_version: "v0.2.0-beta.1",
          downloaded: true,
          latest_version: "v0.2.0-beta.2",
          update_available: true,
        }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("release")).resolves.toBe(true);
    });

    expect(mockedSetUpgradeChannelRequest).toHaveBeenCalledWith("release");
  });

  it("uses the running version instead of a stale status channel", async () => {
    const setUpgradeStatusData = vi.fn();
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.3.18",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn(),
        setAppVersionData: vi.fn(),
        setUpgradeStatusData,
        t,
        upgradeStatus: upgradeStatus({ channel: "beta", update_available: false }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("beta")).resolves.toBe(true);
    });

    expect(mockedSetUpgradeChannelRequest).toHaveBeenCalledWith("beta");
  });

  it("does not call the channel API when the running version already matches the requested channel", async () => {
    const setUpgradeStatusData = vi.fn();
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.3.18",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn(),
        setAppVersionData: vi.fn(),
        setUpgradeStatusData,
        t,
        upgradeStatus: upgradeStatus({ channel: "beta", current_version: "v0.3.18", update_available: false }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("release")).resolves.toBe(true);
    });

    expect(mockedSetUpgradeChannelRequest).not.toHaveBeenCalled();
    expect(setUpgradeStatusData).not.toHaveBeenCalled();
  });

  it("shows the message from a plain API error without issuing a second channel request", async () => {
    mockedSetUpgradeChannelRequest.mockRejectedValueOnce({ status: 502, message: "preview feed unavailable" });
    const setUpgradeStatusData = vi.fn();
    const refreshWorkspaceUpgradeStatus = vi.fn().mockResolvedValue(upgradeStatus({ update_available: false }));
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.3.18",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus,
        setAppVersionData: vi.fn(),
        setUpgradeStatusData,
        t,
        upgradeStatus: upgradeStatus({ update_available: false }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("beta")).resolves.toBe(false);
    });

    expect(mockedSetUpgradeChannelRequest).toHaveBeenCalledTimes(1);
    expect(mockedSetUpgradeChannelRequest).toHaveBeenCalledWith("beta");
    expect(refreshWorkspaceUpgradeStatus).not.toHaveBeenCalled();
    expect(setUpgradeStatusData).not.toHaveBeenCalled();
    expect(result.current.upgradeChannelError).toContain("upgradeChannelSwitchFailed");
    expect(result.current.upgradeChannelError).toContain("preview feed unavailable");
    expect(result.current.upgradeChannelError).not.toContain("upgradeErrorUnknown");
    expect(result.current.upgradeError).toBe("");
    expect(result.current.upgradePhase).toBe("idle");

    await act(async () => {
      await result.current.refreshUpgradeStatus();
    });

    expect(result.current.upgradeChannelError).toBe("");
  });

  it("blocks channel installation for a local development build", async () => {
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.5.0-beta.6-5-ge6a80cb1-dirty+local",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn(),
        setAppVersionData: vi.fn(),
        setUpgradeStatusData: vi.fn(),
        t,
        upgradeStatus: upgradeStatus({
          auto_upgrade_supported: false,
          auto_upgrade_unsupported_reason: "local_build",
          channel: "beta",
          current_version: "v0.5.0-beta.6-5-ge6a80cb1-dirty+local",
          update_available: false,
        }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("release")).resolves.toBe(false);
    });

    expect(mockedSetUpgradeChannelRequest).not.toHaveBeenCalled();
    expect(result.current.upgradeChannelError).toBe("upgradeLocalBuildUnsupported");
  });

  it("explains unsigned or missing desktop update packages when the native updater fails", async () => {
    mockedSetUpgradeChannelRequest.mockRejectedValueOnce(
      new Error("The release channel does not provide a signed macOS auto-update package for 0.4.6. HTTP 404"),
    );
    const restoredStatus = upgradeStatus({
      channel: "beta",
      current_version: "v0.2.1-beta.1",
      latest_version: "v0.2.1-beta.2",
      update_available: true,
    });
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.2.1-beta.1",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn().mockResolvedValue(restoredStatus),
        setAppVersionData: vi.fn(),
        setUpgradeStatusData: vi.fn(),
        t,
        upgradeStatus: upgradeStatus({
          channel: "beta",
          current_version: "v0.2.1-beta.1",
          update_available: false,
        }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("release")).resolves.toBe(false);
    });

    expect(result.current.upgradeChannelError).toContain("upgradeChannelSwitchFailed");
    expect(result.current.upgradeChannelError).toContain("upgradeErrorMissingUpdatePackage");
    expect(result.current.upgradeError).toBe("");
    expect(mockedSetUpgradeChannelRequest).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.handleUpgradeStatusChange({
        last_error: "The update is improperly signed",
        last_error_kind: "desktop_update",
      });
    });

    expect(result.current.upgradePhase).toBe("error");
    expect(result.current.upgradeError).toContain("upgradeErrorSignature");
  });
});
