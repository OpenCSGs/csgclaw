import { type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beginAuthLogin, fetchAuthStatus, logoutAuth } from "@/api/auth";
import { useAuthController } from "@/hooks/workspace/useAuthController";
import type { TranslateFn } from "@/models/conversations";
import { authEnvironmentDraftFromPreset } from "@/models/authEnvironment";
import { normalizeAuthStatus } from "@/models/auth";
import { fetchHubTemplates } from "@/api/hub";
import { fetchRelayCatalog } from "@/api/relay";
import { cloudAgentFixture } from "../helpers/cloudAgentFixture";
import { createQueryWrapper } from "../helpers/queryClient";
import { useWorkspaceHubTemplatesQuery, workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { useCloudAgentCatalog } from "@/hooks/workspace/useCloudAgentCatalog";

vi.mock("@/api/auth", async () => {
  const actual = await vi.importActual<typeof import("@/api/auth")>("@/api/auth");
  return {
    ...actual,
    beginAuthLogin: vi.fn(),
    fetchAuthStatus: vi.fn(),
    logoutAuth: vi.fn(),
  };
});

vi.mock("@/api/hub", async (original) => ({
  ...(await original<typeof import("@/api/hub")>()),
  fetchHubTemplates: vi.fn(),
}));
vi.mock("@/api/relay", () => ({ fetchRelayCatalog: vi.fn() }));

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
    vi.mocked(fetchHubTemplates).mockReset();
    vi.mocked(fetchRelayCatalog).mockReset();
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

  it.each(["focus", "visibilitychange"])(
    "refreshes login status, cloud templates, and agents on %s even when the anonymous status is still fresh",
    async (event) => {
      const { client, wrapper } = createQueryWrapper();
      client.setDefaultOptions({ queries: { retry: false, staleTime: 60_000, refetchOnWindowFocus: false } });
      const local = { id: "local.worker", name: "Local worker", source: { kind: "local" } };
      const remote = { id: "alice/worker", name: "Cloud worker", source: { kind: "remote" } };
      let signedIn = false;
      client.setQueryData(workspaceQueryKeys.authStatus(), normalizeAuthStatus({ authenticated: false }));
      vi.mocked(fetchAuthStatus).mockImplementation(async () => ({
        authenticated: signedIn,
        user_id: signedIn ? "alice" : "",
        logged_in_at: signedIn ? "new-session" : "",
      }));
      vi.mocked(fetchHubTemplates).mockImplementation(async (scope) =>
        scope === "local" ? [local] : signedIn ? [remote] : [],
      );
      vi.mocked(fetchRelayCatalog).mockImplementation(async () => (signedIn ? [...cloudAgentFixture] : []));
      const { result } = renderHook(
        () => {
          const auth = useAuthController(t);
          const templates = useWorkspaceHubTemplatesQuery();
          const cloud = useCloudAgentCatalog(auth.status);
          return { auth, templates, cloud };
        },
        { wrapper },
      );
      await waitFor(() => expect(result.current.templates.isFetching).toBe(false));
      expect(result.current.templates.data).toEqual([local]);
      expect(fetchAuthStatus).not.toHaveBeenCalled();
      signedIn = true;
      act(() => {
        if (event === "focus") window.dispatchEvent(new Event("focus"));
        else {
          vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
          document.dispatchEvent(new Event("visibilitychange"));
        }
      });
      await waitFor(() => expect(result.current.auth.status.authenticated).toBe(true));
      await waitFor(() => expect(result.current.templates.data).toEqual([local, remote]));
      await waitFor(() => expect(result.current.cloud.data).toEqual(cloudAgentFixture));
      expect(window.location.hash).toBe("#/settings");
    },
  );

  it("reloads cloud templates after the same account receives a new login session", async () => {
    const { client, wrapper } = createQueryWrapper();
    const local = { id: "local.worker", name: "Local worker", source: { kind: "local" } };
    const remote = { id: "alice/worker", name: "Cloud worker", source: { kind: "remote" } };
    const previous = normalizeAuthStatus({ authenticated: true, user_id: "alice", logged_in_at: "old-session" });
    vi.mocked(fetchAuthStatus).mockResolvedValue(previous);
    let renewed = false;
    vi.mocked(fetchHubTemplates).mockImplementation(async (scope) =>
      scope === "local" ? [local] : renewed ? [remote] : [],
    );
    const { result } = renderHook(
      () => {
        useAuthController(t);
        return useWorkspaceHubTemplatesQuery();
      },
      { wrapper },
    );
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(result.current.data).toEqual([local]);
    renewed = true;
    act(() => client.setQueryData(workspaceQueryKeys.authStatus(), { ...previous, logged_in_at: "new-session" }));
    await waitFor(() => expect(result.current.data).toEqual([local, remote]));
  });
});
