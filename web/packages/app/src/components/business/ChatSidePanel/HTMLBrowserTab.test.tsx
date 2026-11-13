import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HTMLBrowserTab } from "./HTMLBrowserTab";
import { createTranslator } from "@/shared/i18n";
import type { AttachmentPreviewItem } from "@/models/attachments";

const desktop = vi.hoisted(() => ({
  browserCommand: vi.fn(),
  onBrowserState: vi.fn(() => () => {}),
}));
vi.mock("@/shared/platform/desktopBridge", () => ({ getDesktopBridge: () => desktop }));

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
