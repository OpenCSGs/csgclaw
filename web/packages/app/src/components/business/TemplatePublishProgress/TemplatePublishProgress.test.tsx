import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TemplatePublishProgress } from "./TemplatePublishProgress";

const labels = {
  preparing: "Preparing publication…",
  uploading: "Publishing to the community…",
  complete: "Published",
  failed: "Publishing failed",
};

describe("TemplatePublishProgress", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("does not render before a publication starts", () => {
    render(<TemplatePublishProgress active={false} labels={labels} />);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("advances without claiming completion before the request settles", () => {
    render(<TemplatePublishProgress active labels={labels} />);
    act(() => vi.advanceTimersByTime(1800));
    const percent = Number(screen.getByRole("status").getAttribute("aria-valuenow"));
    expect(percent).toBeGreaterThan(12);
    expect(percent).toBeLessThan(100);
    expect(screen.getByText(labels.uploading)).toBeInTheDocument();
  });

  it("shows completion only after the caller confirms success", () => {
    const { rerender } = render(<TemplatePublishProgress active labels={labels} />);
    act(() => vi.advanceTimersByTime(700));
    rerender(<TemplatePublishProgress active={false} complete labels={labels} />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-valuenow", "100");
    expect(screen.getByText(labels.complete)).toBeInTheDocument();
  });

  it("keeps the current progress visible on failure", () => {
    const { rerender } = render(<TemplatePublishProgress active labels={labels} />);
    act(() => vi.advanceTimersByTime(700));
    rerender(<TemplatePublishProgress active={false} failed labels={labels} />);
    expect(screen.getByText(labels.failed)).toBeInTheDocument();
    expect(screen.getByRole("status")).not.toHaveAttribute("aria-valuenow", "100");
  });
});
