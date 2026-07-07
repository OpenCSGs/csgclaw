import { type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beginAuthLogin, fetchAuthStatus, logoutAuth } from "@/api/auth";
import { useAuthController } from "@/hooks/workspace/useAuthController";
import type { TranslateFn } from "@/models/conversations";

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
    expect(beginAuthLogin).toHaveBeenCalledWith(returnURL);
    expect(window.location.hash).toBe("#/opencsg-login?redirect_url=callback");
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBe("1");
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
});
