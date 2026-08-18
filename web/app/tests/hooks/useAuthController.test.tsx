import { type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beginAuthLogin, fetchAuthStatus, logoutAuth } from "@/api/auth";
import { useAuthController } from "@/hooks/workspace/useAuthController";
import type { TranslateFn } from "@/models/conversations";
import { authEnvironmentDraftFromPreset } from "@/models/authEnvironment";

vi.mock("@/api/auth", async () => {
  const actual = await vi.importActual<typeof import("@/api/auth")>("@/api/auth");
  return {
    ...actual,
    beginAuthLogin: vi.fn(),
    fetchAuthStatus: vi.fn(),
    logoutAuth: vi.fn(),
  };
});

const loginPendingStorageKey = "csgclaw.auth.loginPending";

const t: TranslateFn = (key, params = {}) => {
  if (key === "csghubLoginCompleted") {
    return `User ${params.user} signed in.`;
  }
  if (key === "csghubLoginEnvironmentCompleted") {
    return `User ${params.user} signed in to ${params.environment}.`;
  }
  return key;
};

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe("useAuthController", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/#/settings");
    window.localStorage.clear();
    window.sessionStorage.clear();
    vi.mocked(beginAuthLogin).mockReset();
    vi.mocked(fetchAuthStatus).mockReset();
    vi.mocked(logoutAuth).mockReset();
    vi.restoreAllMocks();
  });

  it("shows a one-time notice after returning from a completed login", async () => {
    window.sessionStorage.setItem(loginPendingStorageKey, "1");
    vi.mocked(fetchAuthStatus).mockResolvedValue({
      authenticated: true,
      user_id: "alice",
      user_uuid: "user-1",
    });

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.notice?.message).toBe("User alice signed in."));
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBeNull();

    act(() => result.current.dismissNotice());
    expect(result.current.notice).toBeNull();
  });

  it("uses the localized status failure message when the status request cannot be fetched", async () => {
    vi.mocked(fetchAuthStatus).mockRejectedValue(new TypeError("Failed to fetch"));

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.error).toBe("csghubStatusFailed"));
  });

  it("exposes logout progress until logout cleanup completes", async () => {
    vi.mocked(fetchAuthStatus).mockResolvedValue({
      authenticated: true,
      user_id: "alice",
      user_uuid: "user-1",
    });
    let resolveLogout!: (value: unknown) => void;
    vi.mocked(logoutAuth).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveLogout = resolve;
        }),
    );

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.status.authenticated).toBe(true));

    let logoutPromise!: Promise<void>;
    act(() => {
      logoutPromise = result.current.logout();
    });

    await waitFor(() => expect(result.current.loggingOut).toBe(true));
    expect(result.current.busy).toBe(true);

    await act(async () => {
      resolveLogout({ authenticated: false });
      await logoutPromise;
    });

    expect(result.current.loggingOut).toBe(false);
    expect(result.current.busy).toBe(false);
  });

  it("restores the completed login notice from the callback result", async () => {
    window.history.replaceState({}, "", "/#/settings?auth_result=success");
    vi.mocked(fetchAuthStatus).mockResolvedValue({
      authenticated: true,
      user_id: "alice",
      user_uuid: "user-1",
    });

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.notice?.message).toBe("User alice signed in."));
    expect(window.location.hash).toBe("#/settings");
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBeNull();
  });

  it("keeps the default completed login notice for production", async () => {
    window.sessionStorage.setItem(loginPendingStorageKey, "1");
    vi.mocked(fetchAuthStatus).mockResolvedValue({
      authenticated: true,
      user_id: "alice",
      user_uuid: "user-1",
      opencsg_base_url: "https://opencsg.com",
      base_url: "https://hub.opencsg.com",
      ai_gateway_base_url: "https://ai.space.opencsg.com/v1",
    });

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.notice?.message).toBe("User alice signed in."));
  });

  it("includes the non-default environment in the completed login notice", async () => {
    window.sessionStorage.setItem(loginPendingStorageKey, "1");
    vi.mocked(fetchAuthStatus).mockResolvedValue({
      authenticated: true,
      user_id: "alice",
      user_uuid: "user-1",
      opencsg_base_url: "https://opencsg-stg.com",
      base_url: "https://opencsg-stg.com",
      ai_gateway_base_url: "https://aigateway.opencsg-stg.com/v1",
    });

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.notice?.message).toBe("User alice signed in to opencsg-stg.com."));
  });

  it("redirects the current tab to OpenCSG login and back to CSGClaw", async () => {
    vi.mocked(fetchAuthStatus).mockResolvedValue({ authenticated: false });
    vi.mocked(beginAuthLogin).mockResolvedValue({ login_url: "#/opencsg-login?redirect_url=callback" });
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    const returnURL = window.location.href;

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.login();
    });

    expect(openSpy).not.toHaveBeenCalled();
    expect(beginAuthLogin).toHaveBeenCalledWith(returnURL, {
      opencsg_base_url: "https://opencsg.com",
      csghub_base_url: "https://hub.opencsg.com",
      ai_gateway_base_url: "https://ai.space.opencsg.com/v1",
    });
    expect(window.location.hash).toBe("#/opencsg-login?redirect_url=callback");
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBe("1");
  });

  it("ends the previous pending attempt before starting authorization again", async () => {
    vi.mocked(fetchAuthStatus).mockResolvedValue({ authenticated: false });
    vi.mocked(beginAuthLogin).mockResolvedValueOnce({ login_url: "#/opencsg-login?attempt=first" });

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.login();
    });
    expect(result.current.pending).toBe(true);
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBe("1");

    let resolveRetry!: (value: unknown) => void;
    vi.mocked(beginAuthLogin).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRetry = resolve;
        }),
    );

    let retryPromise!: Promise<void>;
    act(() => {
      retryPromise = result.current.login();
    });

    await waitFor(() => expect(result.current.pending).toBe(false));
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBeNull();

    await act(async () => {
      resolveRetry({ login_url: "#/opencsg-login?attempt=retry" });
      await retryPromise;
    });

    expect(beginAuthLogin).toHaveBeenCalledTimes(2);
    expect(result.current.pending).toBe(true);
    expect(window.location.hash).toBe("#/opencsg-login?attempt=retry");
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBe("1");
  });

  it("uses the localized login failure message when the login request cannot be fetched", async () => {
    vi.mocked(fetchAuthStatus).mockResolvedValue({ authenticated: false });
    vi.mocked(beginAuthLogin).mockRejectedValue(new TypeError("Failed to fetch"));
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.login();
    });

    expect(result.current.error).toBe("csghubLoginFailed");
    expect(openSpy).not.toHaveBeenCalled();
    expect(window.location.hash).toBe("#/settings");
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBeNull();
  });

  it("uses the shared selected environment when login is triggered without an override", async () => {
    vi.mocked(fetchAuthStatus).mockResolvedValue({ authenticated: false });
    vi.mocked(beginAuthLogin).mockResolvedValue({ login_url: "#/opencsg-login?redirect_url=callback" });
    const returnURL = window.location.href;
    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    act(() => result.current.setEnvironment(authEnvironmentDraftFromPreset("stage")));
    await act(async () => {
      await result.current.login();
    });

    expect(beginAuthLogin).toHaveBeenCalledWith(returnURL, {
      opencsg_base_url: "https://opencsg-stg.com",
      csghub_base_url: "https://opencsg-stg.com",
      ai_gateway_base_url: "https://aigateway.opencsg-stg.com/v1",
    });
  });

  it("sends derived custom service URLs without requiring the optional fields in the draft", async () => {
    vi.mocked(fetchAuthStatus).mockResolvedValue({ authenticated: false });
    vi.mocked(beginAuthLogin).mockResolvedValue({ login_url: "#/opencsg-login?redirect_url=callback" });
    const returnURL = window.location.href;

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.login({
        preset: "custom",
        opencsgBaseURL: "https://openeast.opencsg.com",
        csgHubBaseURL: "",
        aiGatewayBaseURL: "",
      });
    });

    expect(beginAuthLogin).toHaveBeenCalledWith(
      returnURL,
      expect.objectContaining({
        opencsg_base_url: "https://openeast.opencsg.com",
        csghub_base_url: "https://openeast.opencsg.com",
        ai_gateway_base_url: "https://openeast.opencsg.com/aigateway/v1",
      }),
    );
  });

  it("surfaces a safe callback failure and removes it from the URL", async () => {
    window.history.replaceState({}, "", "/#/settings?auth_result=failed&auth_reason=invalid_callback");
    window.sessionStorage.setItem(loginPendingStorageKey, "1");
    vi.mocked(fetchAuthStatus).mockResolvedValue({ authenticated: false });

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.error).toBe("csghubCallbackInvalid"));
    expect(window.location.hash).toBe("#/settings");
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBeNull();
  });
});
