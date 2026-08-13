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
    mockedSetUpgradeChannelRequest.mockResolvedValue(upgradeStatus({ channel: "beta", update_available: false }));
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

  it("switches the server update channel and stores the refreshed status", async () => {
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
    expect(setUpgradeStatusData).toHaveBeenCalledWith(expect.objectContaining({ channel: "beta" }));
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

  it("still switches when the stored channel already matches but the running version does not", async () => {
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

  it("returns false and keeps the previous channel when switching fails", async () => {
    mockedSetUpgradeChannelRequest.mockRejectedValueOnce(new Error("feed unavailable"));
    const setUpgradeStatusData = vi.fn();
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.3.18",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn(),
        setAppVersionData: vi.fn(),
        setUpgradeStatusData,
        t,
        upgradeStatus: upgradeStatus({ update_available: false }),
      }),
    );

    await act(async () => {
      await expect(result.current.changeUpgradeChannel("beta")).resolves.toBe(false);
    });

    expect(result.current.upgradeError).toContain("upgradeChannelSwitchFailed");
    expect(result.current.upgradeError).toContain("upgradeErrorUnknown");
    expect(setUpgradeStatusData).not.toHaveBeenCalled();
  });

  it("explains unsigned or missing desktop update packages when the native updater fails", async () => {
    mockedSetUpgradeChannelRequest.mockRejectedValueOnce(
      new Error("The release channel does not provide a signed macOS auto-update package for 0.4.6. HTTP 404"),
    );
    const { result } = renderHook(() =>
      useUpgradeController({
        appVersion: "v0.2.1-beta.1",
        refreshWorkspaceAppVersion: vi.fn(),
        refreshWorkspaceUpgradeStatus: vi.fn(),
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

    expect(result.current.upgradeError).toContain("upgradeChannelSwitchFailed");
    expect(result.current.upgradeError).toContain("upgradeErrorMissingUpdatePackage");

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
