import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ReasoningControls } from "@/components/business/ProfileControls";

const labels: Record<string, string> = {
  profileReasoning: "Reasoning",
  profileReasoningAuto: "Model default",
  profileReasoningDisabled: "Off",
  profileReasoningEffort: "Reasoning strategy",
  profileReasoningHigh: "High",
  profileReasoningLow: "Low",
  profileReasoningMedium: "Medium",
  profileReasoningMinimal: "Minimal",
  profileReasoningXHigh: "Extra high",
};

function t(key: string): string {
  return labels[key] ?? key;
}

describe("ReasoningControls", () => {
  it("uses one selector for model default, off, and explicit efforts", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = render(<ReasoningControls value="" onChange={onChange} t={t} />);

    const selector = screen.getByRole("combobox", { name: "Reasoning strategy" });
    expect(selector).toHaveTextContent("Model default");
    await user.click(selector);
    await user.click(screen.getByRole("option", { name: "High" }));
    expect(onChange).toHaveBeenLastCalledWith("high");

    rerender(<ReasoningControls value="none" onChange={onChange} t={t} />);
    expect(screen.getByRole("combobox", { name: "Reasoning strategy" })).toHaveTextContent("Off");
  });
});
