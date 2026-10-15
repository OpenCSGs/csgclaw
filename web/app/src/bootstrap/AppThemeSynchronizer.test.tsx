import { act, render } from "@testing-library/react";
import { AppThemeSynchronizer } from "./AppThemeSynchronizer";
import { useWorkspaceUiStore } from "@/hooks/workspace/workspaceUiStore";
import { THEME_STORAGE_KEY } from "@/shared/storage/keys";
import { initializeMermaidTheme } from "@/components/business/MessageContent/mermaid";

const { setThemeSource } = vi.hoisted(() => ({ setThemeSource: vi.fn().mockResolvedValue(undefined) }));

vi.mock("@/components/business/MessageContent/mermaid", () => ({ initializeMermaidTheme: vi.fn() }));
vi.mock("@/shared/platform/desktopBridge", () => ({ getDesktopBridge: () => ({ setThemeSource }) }));

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  useWorkspaceUiStore.setState({ theme: "dark" });
});

test("application theme follows OS changes without replacing the saved system preference", () => {
  const listeners = new Set<() => void>();
  const media = {
    matches: false,
    addEventListener: (_: string, listener: () => void) => listeners.add(listener),
    removeEventListener: (_: string, listener: () => void) => listeners.delete(listener),
  };
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => media),
  );
  useWorkspaceUiStore.setState({ theme: "system" });
  const view = render(<AppThemeSynchronizer />);
  expect(document.documentElement.dataset.theme).toBe("light");
  act(() => {
    media.matches = true;
    listeners.forEach((listener) => listener());
  });
  expect(document.documentElement.dataset.theme).toBe("dark");
  expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("system");
  expect(setThemeSource).toHaveBeenLastCalledWith("system");
  expect(initializeMermaidTheme).toHaveBeenLastCalledWith("dark");
  view.unmount();
  expect(listeners.size).toBe(0);
});

test("cross-tab changes update the canonical preference and unsupported system APIs fall back", () => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => {
      throw new Error("Unsupported");
    }),
  );
  useWorkspaceUiStore.setState({ theme: "light" });
  render(<AppThemeSynchronizer />);
  act(() => window.dispatchEvent(new StorageEvent("storage", { key: THEME_STORAGE_KEY, newValue: "system" })));
  expect(useWorkspaceUiStore.getState().theme).toBe("system");
  expect(document.documentElement.dataset.theme).toBe("dark");
});

test("storage failures do not prevent theme changes", () => {
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("Restricted storage");
  });
  useWorkspaceUiStore.setState({ theme: "light" });
  render(<AppThemeSynchronizer />);
  expect(document.documentElement.dataset.theme).toBe("light");
  expect(document.documentElement.style.colorScheme).toBe("light");
});
