import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentBrowserTab } from "./AgentBrowserTab";
import {
  browserCommand,
  fetchBrowserFrame,
  takeBrowserControl,
  releaseBrowserControl,
  type BrowserSession,
} from "@/api/browser";
import { createTranslator } from "@/shared/i18n";

vi.mock("@/api/browser", async (original) => ({
  ...(await original<typeof import("@/api/browser")>()),
  browserCommand: vi.fn(),
  takeBrowserControl: vi.fn(),
  releaseBrowserControl: vi.fn(),
  fetchBrowserFrame: vi.fn(),
}));
const session: BrowserSession = {
  id: "shared",
  room_id: "room",
  title: "Form",
  url: "https://example.test/form",
  revision: 1,
  width: 1280,
  height: 800,
  can_go_back: true,
  can_go_forward: false,
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(browserCommand).mockResolvedValue(session);
  vi.mocked(fetchBrowserFrame).mockResolvedValue({ blob: new Blob(["png"], { type: "image/png" }), revision: "1" });
  vi.mocked(takeBrowserControl).mockResolvedValue({ session, token: "test-control" });
  vi.mocked(releaseBrowserControl).mockResolvedValue(session);
  vi.stubGlobal(
    "URL",
    Object.assign(URL, { createObjectURL: vi.fn(() => "blob:browser-frame"), revokeObjectURL: vi.fn() }),
  );
});
afterEach(() => vi.unstubAllGlobals());

it("shares click coordinates and ordered text entry with the Agent browser session", async () => {
  const user = userEvent.setup();
  const view = render(<AgentBrowserTab roomID="room" session={session} active t={createTranslator("en")} />);
  await user.click(screen.getByRole("button", { name: "Take control" }));
  const image = await screen.findByRole("img", { name: "Interactive browser view" });
  fireEvent.load(image);
  await waitFor(() => expect(screen.getByRole("button", { name: "Type" })).toBeEnabled());
  Object.defineProperties(image, { naturalWidth: { value: 1280 }, naturalHeight: { value: 800 } });
  image.getBoundingClientRect = () => ({
    left: 10,
    top: 20,
    width: 640,
    height: 400,
    right: 650,
    bottom: 420,
    x: 10,
    y: 20,
    toJSON: () => ({}),
  });
  fireEvent.click(image, { clientX: 170, clientY: 120 });
  await waitFor(() =>
    expect(browserCommand).toHaveBeenCalledWith(
      "room",
      "shared",
      { action: "pointer", x: 320, y: 200 },
      "test-control",
      "1",
    ),
  );
  const input = screen.getByRole("textbox", { name: "Type into the selected field on the page" });
  await user.type(input, "用户输入");
  await user.click(screen.getByRole("button", { name: "Type" }));
  await waitFor(() =>
    expect(browserCommand).toHaveBeenCalledWith(
      "room",
      "shared",
      { action: "type", text: "用户输入" },
      "test-control",
      "1",
    ),
  );
  await waitFor(() => expect(input).toHaveValue(""));
  expect(screen.getByRole("link", { name: "Save page screenshot" })).toHaveAttribute(
    "href",
    "api/v1/rooms/room/browsers/shared?full_page=true",
  );
  view.unmount();
  expect(URL.revokeObjectURL).toHaveBeenCalled();
  expect(releaseBrowserControl).toHaveBeenCalledWith("room", "shared", "test-control");
});

it("retains entered text and offers recovery when the browser command fails", async () => {
  vi.mocked(browserCommand).mockRejectedValue(new Error("Disconnected"));
  const user = userEvent.setup();
  render(<AgentBrowserTab roomID="room" session={session} active t={createTranslator("zh")} />);
  await user.click(screen.getByRole("button", { name: "接管" }));
  fireEvent.load(await screen.findByRole("img", { name: "可操作的网页画面" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "输入" })).toBeEnabled());
  const input = screen.getByRole("textbox", { name: "向网页当前选中的输入框填写文字" });
  await user.type(input, "保留输入");
  await user.click(screen.getByRole("button", { name: "输入" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("浏览器操作失败");
  expect(input).toHaveValue("保留输入");
  expect(screen.getByRole("button", { name: createTranslator("zh")("chatBrowserRetry") })).toBeEnabled();
});

it("does not capture frames while its tab is hidden", () => {
  render(<AgentBrowserTab roomID="room" session={session} active={false} t={createTranslator("en")} />);
  expect(fetchBrowserFrame).not.toHaveBeenCalled();
});

it("requires takeover and a fresh frame before allowing user input", async () => {
  const user = userEvent.setup();
  let deliver: ((value: { blob: Blob; revision: string }) => void) | undefined;
  vi.mocked(fetchBrowserFrame).mockImplementation((_room, _id, _signal, token) =>
    token
      ? new Promise((resolve) => {
          deliver = resolve;
        })
      : Promise.resolve({ blob: new Blob(["old"]), revision: "1" }),
  );
  render(<AgentBrowserTab roomID="room" session={session} active t={createTranslator("en")} />);
  const old = await screen.findByRole("img");
  fireEvent.click(old);
  expect(browserCommand).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "Type" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Take control" }));
  await waitFor(() => expect(deliver).toBeDefined());
  expect(screen.getByRole("button", { name: "Type" })).toBeDisabled();
  deliver?.({ blob: new Blob(["fresh"]), revision: "2" });
  const image = await screen.findByRole("img");
  fireEvent.load(image);
  await waitFor(() => expect(screen.getByRole("button", { name: "Type" })).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "Return control" }));
  await waitFor(() => expect(releaseBrowserControl).toHaveBeenCalled());
  expect(screen.getByRole("button", { name: "Type" })).toBeDisabled();
});

it.each(["hidden", "hidden then shown", "unmounted"])(
  "releases a pending takeover when the original tab becomes %s",
  async (transition) => {
    let resolveControl!: (value: { session: BrowserSession; token: string }) => void;
    vi.mocked(takeBrowserControl).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveControl = resolve;
        }),
    );
    const user = userEvent.setup();
    const props = { roomID: "room", session, active: true, t: createTranslator("en") };
    const view = render(<AgentBrowserTab {...props} />);
    await user.click(screen.getByRole("button", { name: "Take control" }));
    await waitFor(() => expect(takeBrowserControl).toHaveBeenCalled());
    if (transition === "unmounted") view.unmount();
    else {
      view.rerender(<AgentBrowserTab {...props} active={false} />);
      if (transition === "hidden then shown") view.rerender(<AgentBrowserTab {...props} />);
    }
    await act(async () => {
      resolveControl({ session, token: "late-control" });
    });
    await waitFor(() => expect(releaseBrowserControl).toHaveBeenCalledWith("room", "shared", "late-control"));
    expect(fetchBrowserFrame).not.toHaveBeenCalledWith("room", "shared", expect.anything(), "late-control");
  },
);

it("forwards Space as a key so focused form controls can be activated", async () => {
  const user = userEvent.setup();
  render(<AgentBrowserTab roomID="room" session={session} active t={createTranslator("en")} />);
  await user.click(screen.getByRole("button", { name: "Take control" }));
  const image = await screen.findByRole("img", { name: "Interactive browser view" });
  fireEvent.load(image);
  await waitFor(() => expect(screen.getByRole("button", { name: "Type" })).toBeEnabled());
  fireEvent.keyDown(image, { key: " " });
  await waitFor(() =>
    expect(browserCommand).toHaveBeenCalledWith("room", "shared", { action: "key", key: "Space" }, "test-control", "1"),
  );
});
