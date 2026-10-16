import { useState } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BrowserTab } from "@/components/business/ChatSidePanel/BrowserTab";
import { getDesktopBridge, type DesktopBridge, type DesktopBrowserState } from "@/shared/platform/desktopBridge";
import { createTranslator } from "@/shared/i18n";
import { TooltipProvider } from "@/components/ui";

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
  expect(address).toHaveAttribute("data-csg-control", "input");
  expect(address).toHaveAttribute("data-size", "sm");
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
it("shows back, forward, and refresh before the address in the web fallback and navigates entered URLs", async () => {
  vi.mocked(getDesktopBridge).mockReturnValue(null);
  const user = userEvent.setup();
  render(<Harness />);
  const address = screen.getByRole("textbox", { name: "Web address" });
  const toolbar = address.closest("form");
  expect(toolbar?.querySelectorAll("button")).toHaveLength(4);
  expect(
    Array.from(toolbar?.querySelectorAll("button") ?? [])
      .slice(0, 3)
      .map((button) => button.getAttribute("aria-label")),
  ).toEqual(["Back", "Forward", "Refresh"]);
  expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
  await user.clear(address);
  await user.type(address, "https://example.com/{enter}");
  expect(screen.getByRole("button", { name: "Back" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "Back" }));
  expect(address).toHaveValue("https://example.org/");
  await user.click(screen.getByRole("button", { name: "Forward" }));
  expect(address).toHaveValue("https://example.com/");
});
it("shows a bottom tooltip for disabled navigation", async () => {
  vi.mocked(getDesktopBridge).mockReturnValue(null);
  const user = userEvent.setup();
  render(
    <TooltipProvider delayDuration={0}>
      <Harness />
    </TooltipProvider>,
  );
  const back = screen.getByRole("button", { name: "Back" });
  expect(back).toBeDisabled();
  await user.hover(back.parentElement!);
  expect((await screen.findByRole("tooltip", { name: "Back" })).closest('[data-side="bottom"]')).not.toBeNull();
});
it("shows a bottom tooltip for the external link", async () => {
  vi.mocked(getDesktopBridge).mockReturnValue(null);
  const user = userEvent.setup();
  render(
    <TooltipProvider delayDuration={0}>
      <Harness />
    </TooltipProvider>,
  );
  await user.hover(screen.getAllByRole("link", { name: "Open in external browser" })[0]);
  expect(
    (await screen.findByRole("tooltip", { name: "Open in external browser" })).closest('[data-side="bottom"]'),
  ).not.toBeNull();
});
it("introduces browser navigation without a redundant action", () => {
  vi.mocked(getDesktopBridge).mockReturnValue(null);
  render(<BrowserTab url="" active onNavigate={vi.fn()} onPopup={popup} t={t} />);
  const empty = screen.getByRole("status");
  expect(empty).toHaveTextContent("Browse alongside your conversation");
  expect(empty).toHaveTextContent("Enter a URL above and press Enter");
  expect(empty).toHaveTextContent("Some sites may need to open in an external browser");
  expect(screen.queryByRole("button", { name: "Enter a URL" })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Web address" })).toHaveFocus();
});
