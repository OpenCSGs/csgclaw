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

  it("opens OpenCSG login in a new tab without returning that tab to CSGClaw", async () => {
    vi.mocked(fetchAuthStatus).mockResolvedValue({ authenticated: false });
    vi.mocked(beginAuthLogin).mockResolvedValue({ login_url: "https://opencsg.com/sso/login?redirect_url=callback" });
    const openedTab = {
      close: vi.fn(),
      location: {
        href: "",
      },
    };
    const openSpy = vi.spyOn(window, "open").mockReturnValue(openedTab as unknown as Window);

    const { result } = renderHook(() => useAuthController(t), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.login();
    });

    expect(openSpy).toHaveBeenCalledWith("about:blank", "_blank");
    expect(beginAuthLogin).toHaveBeenCalledWith("", { suppressReturnURL: true });
    expect(openedTab.location.href).toBe("https://opencsg.com/sso/login?redirect_url=callback");
    expect(window.sessionStorage.getItem(loginPendingStorageKey)).toBe("1");
  });
});
