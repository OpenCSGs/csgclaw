import type { OpenCSGAuthGuard } from "@/hooks/workspace/useOpenCSGAuthGuard";

export function openCSGAuthGuardStub(authenticated = true): OpenCSGAuthGuard {
  return {
    authenticated,
    closeDialog: () => {},
    dialogOpen: false,
    handleAuthenticationError: () => false,
    login: async () => {},
    markAuthenticationExpired: () => {},
    requireAuthentication: () => authenticated,
  };
}
