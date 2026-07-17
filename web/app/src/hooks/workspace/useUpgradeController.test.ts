// @vitest-environment jsdom

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyUpgradeRequest } from "@/api/upgrade";
import type { TranslateFn } from "@/models/conversations";
import type { UpgradeStatus } from "@/models/upgradeStatus";
import { scheduleUpgradePageReload, UPGRADE_PAGE_RELOAD_DELAY_MS, useUpgradeController } from "./useUpgradeController";

vi.mock("@/api/upgrade", () => ({
  applyUpgradeRequest: vi.fn(),
}));

const t: TranslateFn = (key) => key;
const mockedApplyUpgradeRequest = vi.mocked(applyUpgradeRequest);

function upgradeStatus(overrides: Partial<UpgradeStatus> = {}): UpgradeStatus {
  return {
    auto_upgrade_supported: true,
    auto_upgrade_unsupported_reason: "",
    checking: false,
    current_version: "v0.3.18",
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
});
