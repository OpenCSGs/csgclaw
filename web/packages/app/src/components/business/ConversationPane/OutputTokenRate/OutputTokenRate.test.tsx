import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { OutputTokenRate } from "./OutputTokenRate";

const t = createTranslator("zh");

afterEach(() => vi.useRealTimers());

describe("OutputTokenRate", () => {
  it.each([
    [19.9, 20, "medium"],
    [34.9, 35, "fast"],
  ])("uses the displayed value to classify %s", (rate, rounded, tier) => {
    render(<OutputTokenRate rate={rate} expiresAt={performance.now() + 2500} t={t} />);
    expect(screen.getByLabelText(t("tokenRateLabel", { rate: rounded }))).toHaveAttribute("data-tier", tier);
  });

  it.each([
    [18, "slow"],
    [20, "medium"],
    [35, "fast"],
  ])("renders %s t/s directly and expires its sample", (rate, tier) => {
    vi.useFakeTimers();
    render(<OutputTokenRate rate={rate} expiresAt={performance.now() + 2500} t={t} />);
    const badge = screen.getByLabelText(t("tokenRateLabel", { rate }));
    expect(badge).toHaveAttribute("data-tier", tier);
    expect(badge).toHaveTextContent(new RegExp(`^${rate}t/s$`));
    act(() => vi.advanceTimersByTime(2500));
    expect(screen.queryByLabelText(t("tokenRateLabel", { rate }))).toBeNull();
  });
});
