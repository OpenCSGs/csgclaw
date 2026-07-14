import { act, fireEvent, render, screen } from "@testing-library/react";
import { GlobalScrollbarController } from "@/bootstrap/GlobalScrollbarController";

const SCROLLBAR_HIDE_DELAY_MS = 700;

describe("GlobalScrollbarController", () => {
  it("reveals any scrolling element until scrolling becomes idle", () => {
    vi.useFakeTimers();

    try {
      render(
        <>
          <GlobalScrollbarController />
          <div data-testid="scroll-region" />
        </>,
      );
      const scrollRegion = screen.getByTestId("scroll-region");

      fireEvent.scroll(scrollRegion);
      expect(scrollRegion).toHaveAttribute("data-scrollbar-active", "true");

      act(() => vi.advanceTimersByTime(SCROLLBAR_HIDE_DELAY_MS - 1));
      expect(scrollRegion).toHaveAttribute("data-scrollbar-active", "true");

      fireEvent.scroll(scrollRegion);
      act(() => vi.advanceTimersByTime(SCROLLBAR_HIDE_DELAY_MS));
      expect(scrollRegion).not.toHaveAttribute("data-scrollbar-active");
    } finally {
      vi.useRealTimers();
    }
  });

  it("cleans up active scrollbar state when it unmounts", () => {
    vi.useFakeTimers();

    try {
      const { unmount } = render(
        <>
          <GlobalScrollbarController />
          <div data-testid="scroll-region" />
        </>,
      );
      const scrollRegion = screen.getByTestId("scroll-region");

      fireEvent.scroll(scrollRegion);
      unmount();

      expect(scrollRegion).not.toHaveAttribute("data-scrollbar-active");
    } finally {
      vi.useRealTimers();
    }
  });
});
