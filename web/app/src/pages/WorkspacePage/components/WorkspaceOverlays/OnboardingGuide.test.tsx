import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OnboardingGuide } from "./OnboardingGuide";
import { openOnboardingGuide } from "@/shared/lib/onboardingGuide";
import { GUIDE_ACKNOWLEDGED_KEY, GUIDE_REVISION } from "./onboardingGuidePersistence";

const state = vi.hoisted(() => ({ locale: "en", ready: true, version: "dev" as string | undefined }));
vi.mock("@/hooks/workspace", () => ({
  useWorkspaceControllerContext: () => ({
    ready: state.ready,
    sidebarProps: { appVersion: state.version, locale: state.locale },
    t: () => (state.locale === "en" ? "Exit guide" : "退出导览"),
  }),
}));

function sendMessage(
  frame: HTMLIFrameElement,
  type = "csgclaw:guide-ready",
  origin = window.location.origin,
  source = frame.contentWindow,
) {
  fireEvent(window, new MessageEvent("message", { origin, source, data: { type } }));
}

describe("OnboardingGuide", () => {
  beforeEach(() => {
    localStorage.clear();
    state.locale = "en";
    state.ready = true;
    state.version = "dev";
  });

  it("does not acknowledge loading or initialization without a deliberate dismissal", async () => {
    const firstLaunch = render(<OnboardingGuide />);
    const frame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    fireEvent.load(frame);
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBeNull();
    sendMessage(frame);
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBeNull();
    firstLaunch.unmount();
    render(<OnboardingGuide />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it.each(["host", "Escape", "frame"])(
    "acknowledges %s dismissal of a ready guide across application upgrades",
    async (method) => {
      const firstLaunch = render(<OnboardingGuide />);
      sendMessage((await screen.findByRole("dialog")).querySelector("iframe")!);
      if (method === "frame") sendMessage(screen.getByRole("dialog").querySelector("iframe")!, "csgclaw:guide-exit");
      else if (method === "Escape") fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
      else fireEvent.click(screen.getByRole("button", { name: "Exit guide" }));
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBe(GUIDE_REVISION);
      firstLaunch.unmount();

      const nextLaunch = render(<OnboardingGuide />);
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      state.version = "v2.0.0";
      nextLaunch.rerender(<OnboardingGuide />);
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      nextLaunch.unmount();
      render(<OnboardingGuide />);
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    },
  );

  it("waits for the workspace but does not depend on application version loading", async () => {
    state.ready = false;
    state.version = undefined;
    const { rerender } = render(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    state.ready = true;
    rerender(<OnboardingGuide />);
    const frame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    state.version = "v1.0.0";
    rerender(<OnboardingGuide />);
    expect(screen.getByRole("dialog").querySelector("iframe")).toBe(frame);
  });

  it("carries over legacy app-version acknowledgments", () => {
    localStorage.setItem("csgclaw:onboarding-guide:seen:v0.3.0", "seen");
    render(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBe(GUIDE_REVISION);
  });

  it("presents a changed guide revision until the user acknowledges it", async () => {
    localStorage.setItem(GUIDE_ACKNOWLEDGED_KEY, "previous-revision");
    localStorage.setItem("csgclaw:onboarding-guide:seen:v0.3.0", "seen");
    const firstLaunch = render(<OnboardingGuide />);
    sendMessage((await screen.findByRole("dialog")).querySelector("iframe")!);
    fireEvent.click(screen.getByRole("button", { name: "Exit guide" }));
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBe(GUIDE_REVISION);
    firstLaunch.unmount();
    render(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("ignores ready and exit messages from other origins or windows", async () => {
    const firstLaunch = render(<OnboardingGuide />);
    const frame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    sendMessage(frame, "csgclaw:guide-ready", "https://example.com");
    sendMessage(frame, "csgclaw:guide-ready", window.location.origin, window);
    sendMessage(frame, "csgclaw:guide-exit", "https://example.com");
    sendMessage(frame, "csgclaw:guide-exit", window.location.origin, window);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Exit guide" }));
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBeNull();
    firstLaunch.unmount();
    render(<OnboardingGuide />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it("does not reuse readiness from a previous locale frame", async () => {
    const { rerender } = render(<OnboardingGuide />);
    const oldFrame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    const oldWindow = oldFrame.contentWindow;
    sendMessage(oldFrame);
    state.locale = "zh";
    rerender(<OnboardingGuide />);
    const frame = screen.getByRole("dialog").querySelector("iframe")!;
    expect(frame).not.toBe(oldFrame);
    sendMessage(frame, "csgclaw:guide-ready", window.location.origin, oldWindow);
    fireEvent.click(screen.getByRole("button", { name: "退出导览" }));
    expect(localStorage.getItem(GUIDE_ACKNOWLEDGED_KEY)).toBeNull();
  });

  it("still closes when persisting acknowledgment fails", async () => {
    render(<OnboardingGuide />);
    sendMessage((await screen.findByRole("dialog")).querySelector("iframe")!);
    const save = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("Storage unavailable");
    });
    try {
      fireEvent.click(screen.getByRole("button", { name: "Exit guide" }));
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    } finally {
      save.mockRestore();
    }
  });

  it("can close from the host when the frame cannot load or send messages", async () => {
    render(<OnboardingGuide />);
    const dialog = await screen.findByRole("dialog");
    expect(dialog.querySelector("iframe")?.getAttribute("src")).toBe(
      "onboarding-guide/guide.html?locale=en&embedded=1",
    );
    fireEvent.click(screen.getByRole("button", { name: "Exit guide" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("replays with the current locale and supports host Escape", async () => {
    localStorage.setItem(GUIDE_ACKNOWLEDGED_KEY, GUIDE_REVISION);
    const { rerender } = render(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    state.locale = "zh";
    rerender(<OnboardingGuide />);
    openOnboardingGuide();
    const dialog = await screen.findByRole("dialog");
    expect(dialog.querySelector("iframe")?.getAttribute("src")).toBe(
      "onboarding-guide/guide.html?locale=zh&embedded=1",
    );
    fireEvent.keyDown(screen.getByRole("button", { name: "退出导览" }), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("synchronizes the native controls inset on frame load and platform changes", async () => {
    const { container } = render(
      <div data-desktop-platform="win32">
        <OnboardingGuide />
      </div>,
    );
    await screen.findByRole("dialog");
    const frame = container.querySelector("iframe")!;
    const postMessage = vi.spyOn(frame.contentWindow!, "postMessage");
    frame.style.setProperty("--desktop-window-controls-width", "138px");
    fireEvent.load(frame);
    expect(postMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: "csgclaw:guide-layout", controlsWidth: 138 }),
      window.location.origin,
    );
    frame.style.setProperty("--desktop-window-controls-width", "0px");
    container.firstElementChild!.setAttribute("data-desktop-platform", "darwin");
    await waitFor(() =>
      expect(postMessage).toHaveBeenLastCalledWith(
        expect.objectContaining({ type: "csgclaw:guide-layout", controlsWidth: 0 }),
        window.location.origin,
      ),
    );
  });
});
