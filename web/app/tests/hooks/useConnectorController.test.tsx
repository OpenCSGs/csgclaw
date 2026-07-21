import { type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  disconnectGitHubConnectorRequest,
  fetchConnectors,
  saveGitHubConnectorConfigRequest,
  startGitHubConnectorAppInstallRequest,
  startGitHubConnectorOAuthRequest,
} from "@/api/connectors";
import { useConnectorController } from "@/hooks/workspace/useConnectorController";
import type { TranslateFn } from "@/models/conversations";

vi.mock("@/api/connectors", async () => {
  const actual = await vi.importActual<typeof import("@/api/connectors")>("@/api/connectors");
  return {
    ...actual,
    disconnectGitHubConnectorRequest: vi.fn(),
    fetchConnectors: vi.fn(),
    saveGitHubConnectorConfigRequest: vi.fn(),
    startGitHubConnectorAppInstallRequest: vi.fn(),
    startGitHubConnectorOAuthRequest: vi.fn(),
  };
});

const connectorPendingStorageKey = "csgclaw.connectors.github.loginPending";

const t: TranslateFn = (key) => key;

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        refetchOnWindowFocus: false,
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe("useConnectorController", () => {
  beforeEach(() => {
    vi.useRealTimers();
    window.sessionStorage.clear();
    vi.mocked(disconnectGitHubConnectorRequest).mockReset();
    vi.mocked(fetchConnectors).mockReset();
    vi.mocked(saveGitHubConnectorConfigRequest).mockReset();
    vi.mocked(startGitHubConnectorAppInstallRequest).mockReset();
    vi.mocked(startGitHubConnectorOAuthRequest).mockReset();
    vi.restoreAllMocks();
  });

  it("starts GitHub OAuth and navigates the auth tab to GitHub before marking pending auth", async () => {
    vi.mocked(fetchConnectors).mockResolvedValue({
      connectors: [
        {
          provider: "github",
          configured: true,
          connected: false,
          client_id: "client-id",
          client_secret_set: true,
          scopes: ["repo", "read:user", "user:email"],
        },
      ],
    });
    vi.mocked(startGitHubConnectorOAuthRequest).mockResolvedValue({
      provider: "github",
      authorization_url: "https://github.com/login/oauth/authorize?client_id=client-id",
    });
    const authTab = { close: vi.fn(), location: { href: "about:blank" }, opener: window } as unknown as Window;
    const openSpy = vi.spyOn(window, "open").mockReturnValue(authTab);
    const returnURL = window.location.href;

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.configured).toBe(true));
    await act(async () => {
      await result.current.connectGitHub();
    });

    expect(openSpy).toHaveBeenCalledWith("about:blank", "_blank");
    expect(startGitHubConnectorOAuthRequest).toHaveBeenCalledWith(returnURL);
    expect(authTab.location.href).toBe("https://github.com/login/oauth/authorize?client_id=client-id");
    expect(window.sessionStorage.getItem(connectorPendingStorageKey)).toBe("1");
    expect(result.current.pending).toBe(true);
  });

  it("starts GitHub OAuth even when GitHub has no user config", async () => {
    vi.mocked(fetchConnectors).mockResolvedValue({
      connectors: [{ provider: "github", configured: false, connected: false }],
    });
    vi.mocked(startGitHubConnectorOAuthRequest).mockResolvedValue({
      provider: "github",
      authorization_url: "https://github.com/login/oauth/authorize?client_id=managed-client-id",
    });
    const authTab = { close: vi.fn(), location: { href: "about:blank" } } as unknown as Window;
    const openSpy = vi.spyOn(window, "open").mockReturnValue(authTab);

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.configured).toBe(false));
    await act(async () => {
      await result.current.connectGitHub();
    });

    expect(openSpy).toHaveBeenCalledWith("about:blank", "_blank");
    expect(startGitHubConnectorOAuthRequest).toHaveBeenCalledWith(window.location.href);
    expect(authTab.location.href).toBe("https://github.com/login/oauth/authorize?client_id=managed-client-id");
    expect(window.sessionStorage.getItem(connectorPendingStorageKey)).toBe("1");
    expect(result.current.pending).toBe(true);
    expect(result.current.error).toBe("");
  });

  it("does not mark pending when the GitHub authorization tab is blocked", async () => {
    vi.mocked(fetchConnectors).mockResolvedValue({
      connectors: [{ provider: "github", configured: true, connected: false }],
    });
    vi.mocked(startGitHubConnectorOAuthRequest).mockResolvedValue({
      provider: "github",
      authorization_url: "https://github.com/login/oauth/authorize?client_id=client-id",
    });
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.configured).toBe(true));
    await act(async () => {
      await result.current.connectGitHub();
    });

    expect(openSpy).toHaveBeenNthCalledWith(1, "about:blank", "_blank");
    expect(openSpy).toHaveBeenNthCalledWith(
      2,
      "https://github.com/login/oauth/authorize?client_id=client-id",
      "_blank",
    );
    expect(window.sessionStorage.getItem(connectorPendingStorageKey)).toBeNull();
    expect(result.current.pending).toBe(false);
    expect(result.current.error).toBe("connectorOAuthPopupBlocked");
  });

  it("does not mark pending when GitHub OAuth start fails", async () => {
    vi.mocked(fetchConnectors).mockResolvedValue({
      connectors: [{ provider: "github", configured: false, connected: false }],
    });
    vi.mocked(startGitHubConnectorOAuthRequest).mockRejectedValue({
      status: 400,
      message: "github oauth app is not configured",
    });
    const authTab = { close: vi.fn(), location: { href: "about:blank" } } as unknown as Window;
    vi.spyOn(window, "open").mockReturnValue(authTab);

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.connected).toBe(false));
    await act(async () => {
      await result.current.connectGitHub();
    });

    expect(startGitHubConnectorOAuthRequest).toHaveBeenCalledWith(window.location.href);
    expect(authTab.close).toHaveBeenCalledTimes(1);
    expect(window.sessionStorage.getItem(connectorPendingStorageKey)).toBeNull();
    expect(result.current.pending).toBe(false);
    expect(result.current.error).toBe("github oauth app is not configured");
  });

  it("clears stale browser pending auth when the server has no pending OAuth state", async () => {
    window.sessionStorage.setItem(connectorPendingStorageKey, "1");
    vi.mocked(fetchConnectors).mockResolvedValue({
      connectors: [{ provider: "github", configured: true, connected: false, oauth_pending: false }],
    });

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.configured).toBe(true));
    expect(window.sessionStorage.getItem(connectorPendingStorageKey)).toBeNull();
    expect(result.current.pending).toBe(false);
  });

  it("polls connector status after OAuth starts and clears pending when GitHub connects", async () => {
    vi.mocked(fetchConnectors)
      .mockResolvedValueOnce({ connectors: [{ provider: "github", configured: true, connected: false }] })
      .mockResolvedValueOnce({
        connectors: [
          {
            provider: "github",
            configured: true,
            connected: true,
            oauth_pending: false,
            account: { login: "octocat" },
            scopes: ["repo", "read:user", "user:email"],
          },
        ],
      });
    vi.mocked(startGitHubConnectorOAuthRequest).mockResolvedValue({
      provider: "github",
      authorization_url: "https://github.com/login/oauth/authorize?client_id=client-id",
    });
    vi.spyOn(window, "open").mockReturnValue({} as Window);

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.connected).toBe(false));
    await act(async () => {
      await result.current.connectGitHub();
    });

    expect(result.current.pending).toBe(true);

    await waitFor(() => expect(result.current.github.connected).toBe(true), { timeout: 3000 });
    expect(result.current.github.account?.login).toBe("octocat");
    expect(result.current.pending).toBe(false);
    expect(window.sessionStorage.getItem(connectorPendingStorageKey)).toBeNull();
  });

  it("refreshes GitLab status when the page regains focus", async () => {
    vi.mocked(fetchConnectors)
      .mockResolvedValueOnce({
        connectors: [{ provider: "gitlab", configured: true, connected: false }],
      })
      .mockResolvedValueOnce({
        connectors: [
          {
            provider: "gitlab",
            configured: true,
            connected: true,
            base_url: "https://gitlab.example.com/",
            account: { login: "octocat" },
          },
        ],
      });

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.gitlab.connected).toBe(false));
    act(() => {
      window.dispatchEvent(new Event("focus"));
    });

    await waitFor(() => expect(result.current.gitlab.connected).toBe(true));
    expect(result.current.gitlab.account?.login).toBe("octocat");
    expect(fetchConnectors).toHaveBeenCalledTimes(2);
  });

  it("saves GitHub config and refreshes status without exposing secrets", async () => {
    vi.mocked(fetchConnectors).mockResolvedValue({ connectors: [{ provider: "github" }] });
    vi.mocked(saveGitHubConnectorConfigRequest).mockResolvedValue({
      provider: "github",
      configured: true,
      connected: false,
      client_id: "client-id",
      client_secret_set: true,
      scopes: ["repo"],
      client_secret: "secret",
    });

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.saveGitHubConfig({
        client_id: "client-id",
        client_secret: "secret",
        scopes: ["repo"],
      });
    });

    expect(saveGitHubConnectorConfigRequest).toHaveBeenCalledWith({
      client_id: "client-id",
      client_secret: "secret",
      scopes: ["repo"],
    });
    expect(result.current.github).toEqual(
      expect.objectContaining({
        client_id: "client-id",
        client_secret_set: true,
        configured: true,
      }),
    );
    expect(Object.prototype.hasOwnProperty.call(result.current.github, "client_secret")).toBe(false);
  });

  it("opens GitHub App management in a new tab when GitHub is connected", async () => {
    vi.mocked(fetchConnectors).mockResolvedValue({
      connectors: [{ provider: "github", connected: true, app_manageable: true }],
    });
    vi.mocked(startGitHubConnectorAppInstallRequest).mockResolvedValue({
      provider: "github",
      install_url: "https://github.com/apps/csgclaw/installations/select_target?state=install-state",
    });
    const manageTab = { close: vi.fn(), location: { href: "about:blank" }, opener: window } as unknown as Window;
    const openSpy = vi.spyOn(window, "open").mockReturnValue(manageTab);

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.connected).toBe(true));
    await act(async () => {
      await result.current.manageGitHub();
    });

    expect(openSpy).toHaveBeenCalledWith("about:blank", "_blank");
    expect(startGitHubConnectorAppInstallRequest).toHaveBeenCalledTimes(1);
    expect(manageTab.location.href).toBe(
      "https://github.com/apps/csgclaw/installations/select_target?state=install-state",
    );
    expect(result.current.pending).toBe(false);
    expect(result.current.error).toBe("");
  });

  it("shows an error when GitHub App management is blocked", async () => {
    vi.mocked(fetchConnectors).mockResolvedValue({
      connectors: [{ provider: "github", connected: true, app_manageable: true }],
    });
    vi.mocked(startGitHubConnectorAppInstallRequest).mockResolvedValue({
      provider: "github",
      install_url: "https://github.com/apps/csgclaw/installations/select_target?state=install-state",
    });
    vi.spyOn(window, "open").mockReturnValue(null);

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.github.connected).toBe(true));
    await act(async () => {
      await result.current.manageGitHub();
    });

    expect(result.current.pending).toBe(false);
    expect(result.current.error).toBe("connectorManagePopupBlocked");
  });

  it("disconnects GitHub and clears pending auth", async () => {
    window.sessionStorage.setItem(connectorPendingStorageKey, "1");
    vi.mocked(fetchConnectors).mockResolvedValue({ connectors: [{ provider: "github", connected: true }] });
    vi.mocked(disconnectGitHubConnectorRequest).mockResolvedValue({
      provider: "github",
      configured: true,
      connected: false,
    });

    const { result } = renderHook(() => useConnectorController(t), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.disconnectGitHub();
    });

    expect(result.current.github.connected).toBe(false);
    expect(window.sessionStorage.getItem(connectorPendingStorageKey)).toBeNull();
    expect(result.current.pending).toBe(false);
  });
});
