import { act, renderHook } from "@testing-library/react";
import type { AuthStatus } from "@/models/auth";
import { emptyAuthStatus } from "@/models/auth";
import {
  isOpenCSGAuthenticationError,
  isOpenCSGRuntimeAuthenticationError,
  useOpenCSGAuthGuard,
} from "@/hooks/workspace/useOpenCSGAuthGuard";

function authenticatedStatus(loggedInAt: string): AuthStatus {
  return {
    ...emptyAuthStatus(),
    authenticated: true,
    logged_in_at: loggedInAt,
  };
}

describe("useOpenCSGAuthGuard", () => {
  it("opens the shared login dialog when authentication is required", () => {
    const login = vi.fn(async () => {});
    const { result, rerender } = renderHook(
      ({ status }: { status: AuthStatus }) => useOpenCSGAuthGuard({ login, status }),
      { initialProps: { status: emptyAuthStatus() } },
    );

    act(() => {
      expect(result.current.requireAuthentication()).toBe(false);
    });

    expect(result.current.authenticated).toBe(false);
    expect(result.current.dialogOpen).toBe(true);

    rerender({ status: authenticatedStatus("2026-09-15T08:00:00Z") });
    expect(result.current.authenticated).toBe(true);
    expect(result.current.dialogOpen).toBe(false);
  });

  it("keeps an expired session invalid until a new login is observed", () => {
    const initialStatus = authenticatedStatus("2026-09-15T08:00:00Z");
    const login = vi.fn(async () => {});
    const { result, rerender } = renderHook(
      ({ status }: { status: AuthStatus }) => useOpenCSGAuthGuard({ login, status }),
      { initialProps: { status: initialStatus } },
    );

    const authenticationError = { status: 401 };
    act(() => {
      expect(result.current.handleAuthenticationError(authenticationError)).toBe(true);
    });

    expect(result.current.authenticated).toBe(false);
    expect(result.current.dialogOpen).toBe(true);

    rerender({ status: { ...initialStatus } });
    expect(result.current.authenticated).toBe(false);

    rerender({ status: authenticatedStatus("2026-09-15T08:05:00Z") });
    expect(result.current.authenticated).toBe(true);
    expect(result.current.dialogOpen).toBe(false);

    act(() => {
      expect(result.current.handleAuthenticationError(authenticationError)).toBe(true);
    });
    expect(result.current.authenticated).toBe(true);
    expect(result.current.dialogOpen).toBe(false);
  });

  it("handles the same authentication error again after the dialog is closed", () => {
    const login = vi.fn(async () => {});
    const { result } = renderHook(() => useOpenCSGAuthGuard({ login, status: emptyAuthStatus() }));
    const authenticationError = { status: 401 };

    act(() => {
      expect(result.current.handleAuthenticationError(authenticationError)).toBe(true);
    });
    expect(result.current.dialogOpen).toBe(true);

    act(() => result.current.closeDialog());
    expect(result.current.dialogOpen).toBe(false);

    act(() => {
      expect(result.current.handleAuthenticationError(authenticationError)).toBe(true);
    });
    expect(result.current.dialogOpen).toBe(true);
  });

  it("recognizes API and runtime authentication failures", () => {
    expect(isOpenCSGAuthenticationError({ code: "AUTH_REQUIRED" })).toBe(true);
    expect(isOpenCSGAuthenticationError({ status: 401 })).toBe(true);
    expect(isOpenCSGAuthenticationError({ status: 403 })).toBe(false);
    expect(
      isOpenCSGRuntimeAuthenticationError({
        metadata: {
          csgclaw: {
            error_code: "invalid_api_key",
            runtime_error: true,
          },
        },
      }),
    ).toBe(true);
  });
});
