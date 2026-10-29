import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OnboardingGuide } from "./OnboardingGuide";
import { openOnboardingGuide } from "@/shared/lib/onboardingGuide";

const state = vi.hoisted(() => ({ locale: "en", ready: true, version: "test" as string | undefined }));
vi.mock("@/hooks/workspace", () => ({
  useWorkspaceControllerContext: () => ({
    ready: state.ready,
    sidebarProps: { appVersion: state.version, locale: state.locale },
    t: () => (state.locale === "en" ? "Exit guide" : "退出导览"),
  }),
}));

const seenKey = (version = "test") => `csgclaw:onboarding-guide:seen:${version}`;

function sendReady(frame: HTMLIFrameElement, origin = window.location.origin, source = frame.contentWindow) {
  fireEvent(window, new MessageEvent("message", { origin, source, data: { type: "csgclaw:guide-ready" } }));
}

describe("OnboardingGuide", () => {
  beforeEach(() => {
    localStorage.clear();
    state.locale = "en";
    state.ready = true;
    state.version = "test";
  });

  it("keeps the guide usable when persisting readiness fails", async () => {
    render(<OnboardingGuide />);
    const frame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    const save = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("Storage unavailable");
    });
    try {
      sendReady(frame);
      expect(screen.getByRole("dialog")).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Exit guide" }));
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    } finally {
      save.mockRestore();
    }
  });

  it("retries next launch when the frame loaded without successfully initializing", async () => {
    const firstLaunch = render(<OnboardingGuide />);
    const frame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    expect(localStorage.getItem(seenKey())).toBeNull();
    fireEvent.load(frame);
    expect(localStorage.getItem(seenKey())).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Exit guide" }));
    firstLaunch.unmount();

    render(<OnboardingGuide />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it("records a successful display and skips automatic replay for the same version", async () => {
    const firstLaunch = render(<OnboardingGuide />);
    sendReady((await screen.findByRole("dialog")).querySelector("iframe")!);
    expect(localStorage.getItem(seenKey())).toBe("seen");
    firstLaunch.unmount();

    const { rerender } = render(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    state.version = "next";
    rerender(<OnboardingGuide />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(localStorage.getItem(seenKey("next"))).toBeNull();
  });

  it("rejects readiness messages from other origins or windows", async () => {
    render(<OnboardingGuide />);
    const frame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    sendReady(frame, "https://example.com");
    sendReady(frame, window.location.origin, window);
    expect(localStorage.getItem(seenKey())).toBeNull();
    sendReady(frame);
    expect(localStorage.getItem(seenKey())).toBe("seen");
  });

  it("waits for workspace readiness and the actual app version before opening", async () => {
    state.ready = false;
    state.version = undefined;
    const { rerender } = render(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    state.ready = true;
    rerender(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    state.version = "";
    rerender(<OnboardingGuide />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    state.version = "test";
    rerender(<OnboardingGuide />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(localStorage.getItem(seenKey("dev"))).toBeNull();
  });

  it("supports an actual development build version", async () => {
    state.version = "dev";
    render(<OnboardingGuide />);
    sendReady((await screen.findByRole("dialog")).querySelector("iframe")!);
    expect(localStorage.getItem(seenKey("dev"))).toBe("seen");
  });

  it("allows manual replay before version loading without recording a temporary version", async () => {
    state.version = "";
    const { rerender } = render(<OnboardingGuide />);
    openOnboardingGuide();
    const oldFrame = (await screen.findByRole("dialog")).querySelector("iframe")!;
    const oldWindow = oldFrame.contentWindow;
    sendReady(oldFrame);
    expect(localStorage.getItem(seenKey("dev"))).toBeNull();

    state.version = "test";
    rerender(<OnboardingGuide />);
    const frame = screen.getByRole("dialog").querySelector("iframe")!;
    expect(frame).not.toBe(oldFrame);
    sendReady(frame, window.location.origin, oldWindow);
    expect(localStorage.getItem(seenKey())).toBeNull();
    sendReady(frame);
    expect(localStorage.getItem(seenKey())).toBe("seen");
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
    localStorage.setItem(seenKey(), "seen");
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
});
