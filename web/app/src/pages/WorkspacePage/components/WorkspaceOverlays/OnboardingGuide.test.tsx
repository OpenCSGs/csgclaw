import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OnboardingGuide } from "./OnboardingGuide";
import { openOnboardingGuide } from "@/shared/lib/onboardingGuide";

const state = vi.hoisted(() => ({ locale: "en" }));
vi.mock("@/hooks/workspace", () => ({
  useWorkspaceControllerContext: () => ({
    ready: true,
    sidebarProps: { appVersion: "test", locale: state.locale },
    t: () => (state.locale === "en" ? "Exit guide" : "退出导览"),
  }),
}));

describe("OnboardingGuide", () => {
  beforeEach(() => {
    localStorage.clear();
    state.locale = "en";
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
    const { rerender } = render(<OnboardingGuide />);
    fireEvent.click(await screen.findByRole("button", { name: "Exit guide" }));
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
