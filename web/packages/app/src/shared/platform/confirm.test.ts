import { afterEach, describe, expect, it, vi } from "vitest";
import type { DesktopBridge } from "./desktopBridge";
import { confirmWithFocusRecovery } from "./confirm";

function installDesktopBridge(restoreFocusAfterDialog: () => Promise<void>) {
  window.csgclawDesktop = {
    getWindowState: vi.fn(),
    onWindowState: vi.fn(),
    browserCommand: vi.fn(),
    onBrowserState: vi.fn(),
    getRuntimeInfo: vi.fn(),
    openOAuth: vi.fn(),
    restoreFocusAfterDialog,
    checkForUpdates: vi.fn(),
    installDownloadedUpdate: vi.fn(),
    restartSidecar: vi.fn(),
    showWindow: vi.fn(),
    showTurnNotification: vi.fn(),
    onTurnNotificationClick: vi.fn(),
    setUpdateChannel: vi.fn(),
    setThemeSource: vi.fn(),
    onUpdateStatus: vi.fn(),
  } as DesktopBridge;
}

describe("confirmWithFocusRecovery", () => {
  afterEach(() => {
    delete window.csgclawDesktop;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("preserves browser confirm behavior without a desktop bridge", () => {
    vi.spyOn(window, "confirm").mockReturnValue(false);

    expect(confirmWithFocusRecovery("Discard changes?")).toBe(false);
    expect(window.confirm).toHaveBeenCalledWith("Discard changes?");
  });

  it("restores desktop focus after the native dialog has closed", async () => {
    vi.useFakeTimers();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const restoreFocusAfterDialog = vi.fn().mockResolvedValue(undefined);
    installDesktopBridge(restoreFocusAfterDialog);

    expect(confirmWithFocusRecovery("Leave this page?")).toBe(true);
    expect(restoreFocusAfterDialog).not.toHaveBeenCalled();

    await vi.runAllTimersAsync();
    expect(restoreFocusAfterDialog).toHaveBeenCalledOnce();
  });

  it("does not surface focus recovery failures", async () => {
    vi.useFakeTimers();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const restoreFocusAfterDialog = vi.fn().mockRejectedValue(new Error("window closed"));
    installDesktopBridge(restoreFocusAfterDialog);

    expect(confirmWithFocusRecovery("Continue?")).toBe(true);
    await vi.runAllTimersAsync();
    expect(restoreFocusAfterDialog).toHaveBeenCalledOnce();
  });
});
