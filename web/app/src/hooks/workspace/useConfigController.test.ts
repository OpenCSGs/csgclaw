// @vitest-environment jsdom

import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchServerConfig, restartServer, updateServerConfig } from "@/api/config";
import type { ConfigSettings } from "@/models/configSettings";
import type { TranslateFn } from "@/models/conversations";
import type { DesktopBridge } from "@/shared/platform/desktopBridge";
import { useConfigController } from "./useConfigController";

vi.mock("@/api/config", () => ({
  fetchServerConfig: vi.fn(),
  fetchServerRestartStatus: vi.fn(),
  restartServer: vi.fn(),
  updateServerConfig: vi.fn(),
}));

const t: TranslateFn = (key) => key;
const mockedFetchServerConfig = vi.mocked(fetchServerConfig);
const mockedRestartServer = vi.mocked(restartServer);
const mockedUpdateServerConfig = vi.mocked(updateServerConfig);

const settings: ConfigSettings = {
  path: "/tmp/config.toml",
  listen_addr: "0.0.0.0:18080",
  advertise_base_url: "",
  advertise_base_url_effective: "http://127.0.0.1:18080",
  access_token_set: true,
  access_token_preview: "pc-***",
  show_upgrade: true,
  sandbox_provider: "docker",
  supported_sandbox_providers: ["docker"],
  hub_local_path: "",
  default_manager_template: "builtin.manager-codex",
  default_worker_template: "builtin.picoclaw-worker",
};

describe("useConfigController", () => {
  beforeEach(() => {
    mockedFetchServerConfig.mockReset();
    mockedFetchServerConfig.mockResolvedValue(settings);
    mockedUpdateServerConfig.mockReset();
    mockedUpdateServerConfig.mockResolvedValue(settings);
    mockedRestartServer.mockReset();
    mockedRestartServer.mockResolvedValue(undefined);
  });

  afterEach(() => {
    delete window.csgclawDesktop;
    vi.restoreAllMocks();
  });

  it("uses the supervised sidecar restart in Electron", async () => {
    const restartSidecar = vi.fn().mockResolvedValue(undefined);
    const bridge: DesktopBridge = {
      getRuntimeInfo: vi.fn().mockResolvedValue({
        platform: "darwin",
        arch: "arm64",
        appVersion: "0.3.19",
        backendVersion: "0.3.19",
      }),
      openOAuth: vi.fn().mockResolvedValue({ opened: true }),
      checkForUpdates: vi.fn().mockResolvedValue({
        state: "idle",
        channel: "release",
        currentVersion: "0.3.19",
      }),
      installDownloadedUpdate: vi.fn().mockResolvedValue(undefined),
      restartSidecar,
      setUpdateChannel: vi.fn().mockResolvedValue({
        state: "idle",
        channel: "release",
        currentVersion: "0.3.19",
      }),
      setThemeSource: vi.fn().mockResolvedValue(undefined),
      onUpdateStatus: vi.fn().mockReturnValue(() => undefined),
    };
    window.csgclawDesktop = bridge;
    const refreshWorkspaceAppVersion = vi.fn().mockResolvedValue("0.3.19");

    const { result } = renderHook(() =>
      useConfigController({
        refreshWorkspaceAppVersion,
        t,
      }),
    );

    act(() => {
      result.current.openConfigModal();
    });
    await waitFor(() => {
      expect(result.current.configModalProps?.configBusy).toBe(false);
    });

    await act(async () => {
      await result.current.configModalProps?.onSaveAndRestart();
    });

    expect(restartSidecar).toHaveBeenCalledTimes(1);
    expect(mockedRestartServer).not.toHaveBeenCalled();
    expect(refreshWorkspaceAppVersion).toHaveBeenCalledWith({ cacheBust: true });
    expect(result.current.configModalProps?.configPhase).toBe("done");
  });
});
