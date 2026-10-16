import { useState } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BrowserTab } from "@/components/business/ChatSidePanel/BrowserTab";
import { getDesktopBridge, type DesktopBridge, type DesktopBrowserState } from "@/shared/platform/desktopBridge";
import { createTranslator } from "@/shared/i18n";

vi.mock("@/shared/platform/desktopBridge", async (original) => ({
  ...(await original<object>()),
  getDesktopBridge: vi.fn(),
}));
const t = createTranslator("en");
let listener: (state: DesktopBrowserState) => void;
const unsubscribe = vi.fn();
const command = vi.fn().mockResolvedValue(null);
const popup = vi.fn();
function Harness({ active = true }: { active?: boolean }) {
  const [url, setURL] = useState("https://example.org/");
  return <BrowserTab url={url} active={active} onNavigate={setURL} onPopup={popup} t={t} />;
}
function status(id: string, overrides: Partial<DesktopBrowserState> = {}): DesktopBrowserState {
  return { id, url: "https://example.org/", loading: false, canGoBack: false, canGoForward: false, ...overrides };
}
afterEach(() => vi.unstubAllGlobals());
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  command.mockReset().mockResolvedValue(null);
  popup.mockReset();
  unsubscribe.mockReset();
  const bridge: DesktopBridge = {
    getRuntimeInfo: vi.fn(),
    openOAuth: vi.fn(),
    checkForUpdates: vi.fn(),
    installDownloadedUpdate: vi.fn(),
    restartSidecar: vi.fn(),
    showWindow: vi.fn(),
    showTurnNotification: vi.fn(),
    onTurnNotificationClick: vi.fn(),
    setUpdateChannel: vi.fn(),
    setThemeSource: vi.fn(),
    onUpdateStatus: vi.fn(),
    getWindowState: vi.fn(),
    onWindowState: vi.fn(),
    browserCommand: command,
    onBrowserState: (callback) => {
      listener = callback;
      return unsubscribe;
    },
  };
  vi.mocked(getDesktopBridge).mockReturnValue(bridge);
});
it("uses a native host, preserves address drafts on page navigation, and closes the native page on unmount", async () => {
  const user = userEvent.setup();
  const view = render(<Harness />);
  await waitFor(() =>
    expect(command).toHaveBeenCalledWith(expect.objectContaining({ action: "navigate", url: "https://example.org/" })),
  );
  const id = command.mock.calls[0][0].id;
  expect(document.querySelector("iframe")).toBeNull();
  act(() => listener(status(id, { url: "https://example.org/next", canGoBack: true })));
  const address = screen.getByRole("textbox", { name: "Web address" });
  await waitFor(() => expect(address).toHaveValue("https://example.org/next"));
  await user.clear(address);
  await user.type(address, "draft.example");
  act(() => listener(status(id, { url: "https://example.org/redirect", canGoBack: true })));
  expect(address).toHaveValue("draft.example");
  await user.click(screen.getByRole("button", { name: "Back" }));
  expect(command).toHaveBeenCalledWith({ id, action: "back" });
  act(() => listener(status(id, { popupURL: "https://example.org/popup" })));
  expect(popup).toHaveBeenCalledWith("https://example.org/popup");
  view.rerender(<Harness active={false} />);
  await waitFor(() =>
    expect(command).toHaveBeenCalledWith(expect.objectContaining({ action: "sync", visible: false })),
  );
  view.unmount();
  expect(unsubscribe).toHaveBeenCalled();
  expect(command).toHaveBeenCalledWith({ id, action: "close" });
});
it("shows a retryable native load error and forwards Escape to the sidebar", async () => {
  const user = userEvent.setup();
  render(<Harness />);
  await waitFor(() => expect(command).toHaveBeenCalled());
  const id = command.mock.calls[0][0].id;
  act(() => listener(status(id, { error: "load_-105" })));
  expect(screen.getByRole("alert")).toHaveTextContent("The page could not load");
  await user.click(screen.getByRole("button", { name: "Retry loading" }));
  expect(command).toHaveBeenCalledWith({ id, action: "reload" });
  const escaped = vi.fn();
  window.addEventListener("keydown", escaped);
  act(() => listener(status(id, { shortcut: "escape" })));
  expect(escaped).toHaveBeenCalled();
  expect(screen.getByRole("textbox", { name: "Web address" })).toHaveFocus();
  window.removeEventListener("keydown", escaped);
});
