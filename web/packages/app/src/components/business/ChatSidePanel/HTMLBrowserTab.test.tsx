import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HTMLBrowserTab } from "./HTMLBrowserTab";
import { createTranslator } from "@/shared/i18n";
import type { AttachmentPreviewItem } from "@/models/attachments";

const desktop = vi.hoisted(() => ({
  available: true,
  browserCommand: vi.fn(),
  onBrowserState: vi.fn(() => () => {}),
}));
vi.mock("@/shared/platform/desktopBridge", () => ({ getDesktopBridge: () => (desktop.available ? desktop : null) }));

beforeEach(() => {
  vi.clearAllMocks();
  desktop.available = true;
});

it("delivers web HTML only to its sandboxed frame and initializes a fresh document on refresh", async () => {
  desktop.available = false;
  const html = "<h1>Pelican Rider</h1><script>window.started = true</script>";
  const item: AttachmentPreviewItem = {
    id: "pelican",
    name: "pelican.html",
    file: new File([html], "pelican.html", { type: "text/html" }),
    mediaType: "text/html",
    sizeBytes: html.length,
  };
  const user = userEvent.setup();
  const view = render(
    <HTMLBrowserTab item={item} sourceKey="room:pelican" active t={createTranslator("en")} onPopup={vi.fn()} />,
  );
  const address = await screen.findByRole("textbox", { name: "Web address" });
  expect(address).toHaveValue(new URL("html-preview.html", document.baseURI).href);
  const frame = document.querySelector("iframe")!;
  expect(frame).toHaveAttribute("sandbox", "allow-scripts allow-forms");
  expect(frame).not.toHaveAttribute("srcdoc");
  expect(screen.queryByRole("link", { name: "Open in system browser" })).not.toBeInTheDocument();
  const post = vi.spyOn(frame.contentWindow!, "postMessage");
  const ready = { type: "csgclaw:html-preview-ready" };
  window.dispatchEvent(new MessageEvent("message", { source: window, data: ready }));
  expect(post).not.toHaveBeenCalled();
  window.dispatchEvent(new MessageEvent("message", { source: frame.contentWindow, data: ready, origin: "null" }));
  expect(post).toHaveBeenCalledExactlyOnceWith({ type: "csgclaw:html-preview", html }, "*");
  window.dispatchEvent(new MessageEvent("message", { source: frame.contentWindow, data: ready, origin: "null" }));
  expect(post).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(document.querySelector("iframe")).not.toBe(frame));
  const refreshed = document.querySelector("iframe")!;
  const postRefresh = vi.spyOn(refreshed.contentWindow!, "postMessage");
  window.dispatchEvent(new MessageEvent("message", { source: frame.contentWindow, data: ready }));
  expect(postRefresh).not.toHaveBeenCalled();
  window.dispatchEvent(new MessageEvent("message", { source: refreshed.contentWindow, data: ready, origin: "null" }));
  expect(postRefresh).toHaveBeenCalledExactlyOnceWith({ type: "csgclaw:html-preview", html }, "*");
  view.unmount();
  expect(desktop.browserCommand).not.toHaveBeenCalled();
});

it("keeps the source identity when refreshing and changes it between rooms", async () => {
  let revision = 0;
  desktop.browserCommand.mockImplementation(async (request) =>
    request.action === "previewHTML"
      ? { id: request.id, url: `http://127.0.0.1:19433/${++revision}/index.html` }
      : null,
  );
  const item: AttachmentPreviewItem = {
    id: "workspace:agent:game.html",
    name: "game.html",
    file: new File(["<h1>Game</h1>"], "game.html", { type: "text/html" }),
    mediaType: "text/html",
    sizeBytes: 13,
  };
  const t = createTranslator("en");
  const props = { item, active: true, t, onPopup: vi.fn() };
  const firstKey = JSON.stringify(["room-a", item.id]);
  const nextKey = JSON.stringify(["room-b", item.id]);
  const user = userEvent.setup();
  const view = render(<HTMLBrowserTab {...props} sourceKey={firstKey} />);
  const previews = () =>
    desktop.browserCommand.mock.calls.map(([request]) => request).filter((request) => request.action === "previewHTML");
  await screen.findByRole("textbox", { name: "Web address" });
  expect(previews()).toEqual([expect.objectContaining({ sourceKey: firstKey, html: "<h1>Game</h1>" })]);
  await user.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(previews()).toHaveLength(2));
  expect(previews()[1].sourceKey).toBe(firstKey);
  expect(previews()[1].id).not.toBe(previews()[0].id);
  expect(desktop.browserCommand).toHaveBeenCalledWith({ id: previews()[0].id, action: "releaseHTML" });
  view.rerender(<HTMLBrowserTab {...props} sourceKey={nextKey} />);
  await waitFor(() => expect(previews()).toHaveLength(3));
  expect(previews()[2].sourceKey).toBe(nextKey);
  view.unmount();
  expect(desktop.browserCommand).toHaveBeenCalledWith({ id: previews()[2].id, action: "releaseHTML" });
});

it("opens a new browser tab for an entered address without replacing the HTML view", async () => {
  const sourceURL = "http://127.0.0.1:19433/preview/index.html";
  desktop.browserCommand.mockImplementation(async (request) =>
    request.action === "previewHTML" ? { id: request.id, url: sourceURL } : null,
  );
  const item: AttachmentPreviewItem = {
    id: "game",
    name: "game.html",
    file: new File(["<h1>Game</h1>"], "game.html", { type: "text/html" }),
    mediaType: "text/html",
    sizeBytes: 13,
  };
  const onPopup = vi.fn();
  const user = userEvent.setup();
  render(<HTMLBrowserTab item={item} sourceKey="room:game" active t={createTranslator("en")} onPopup={onPopup} />);
  const address = await screen.findByRole("textbox", { name: "Web address" });
  await user.clear(address);
  await user.type(address, "https://example.org/page{Enter}");
  expect(onPopup).toHaveBeenCalledExactlyOnceWith("https://example.org/page");
  expect(address).toHaveValue(sourceURL);
  expect(desktop.browserCommand).not.toHaveBeenCalledWith(expect.objectContaining({ action: "releaseHTML" }));
  expect(desktop.browserCommand).not.toHaveBeenCalledWith(
    expect.objectContaining({ action: "navigate", url: "https://example.org/page" }),
  );
  await user.keyboard("{Enter}");
  expect(onPopup).toHaveBeenCalledTimes(1);
  expect(desktop.browserCommand).toHaveBeenCalledWith(expect.objectContaining({ action: "reload" }));
});
