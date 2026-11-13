import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentBrowserTab } from "./AgentBrowserTab";
import { browserCommand, fetchBrowserFrame, type BrowserSession } from "@/api/browser";
import { createTranslator } from "@/shared/i18n";

vi.mock("@/api/browser", async (original) => ({
  ...(await original<typeof import("@/api/browser")>()),
  browserCommand: vi.fn(),
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
  vi.mocked(fetchBrowserFrame).mockResolvedValue(new Blob(["png"], { type: "image/png" }));
  vi.stubGlobal(
    "URL",
    Object.assign(URL, { createObjectURL: vi.fn(() => "blob:browser-frame"), revokeObjectURL: vi.fn() }),
  );
});
afterEach(() => vi.unstubAllGlobals());

it("shares click coordinates and ordered text entry with the Agent browser session", async () => {
  const user = userEvent.setup();
  const view = render(<AgentBrowserTab roomID="room" session={session} active t={createTranslator("en")} />);
  const image = await screen.findByRole("img", { name: "Interactive browser view" });
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
    expect(browserCommand).toHaveBeenCalledWith("room", "shared", { action: "pointer", x: 320, y: 200 }),
  );
  const input = screen.getByRole("textbox", { name: "Type into the selected field on the page" });
  await user.type(input, "用户输入");
  await user.click(screen.getByRole("button", { name: "Type" }));
  await waitFor(() =>
    expect(browserCommand).toHaveBeenCalledWith("room", "shared", { action: "type", text: "用户输入" }),
  );
  await waitFor(() => expect(input).toHaveValue(""));
  expect(screen.getByRole("link", { name: "Save page screenshot" })).toHaveAttribute(
    "href",
    "api/v1/rooms/room/browsers/shared?full_page=true",
  );
  view.unmount();
  expect(URL.revokeObjectURL).toHaveBeenCalled();
});

it("retains entered text and offers recovery when the browser command fails", async () => {
  vi.mocked(browserCommand).mockRejectedValue(new Error("Disconnected"));
  const user = userEvent.setup();
  render(<AgentBrowserTab roomID="room" session={session} active t={createTranslator("zh")} />);
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
