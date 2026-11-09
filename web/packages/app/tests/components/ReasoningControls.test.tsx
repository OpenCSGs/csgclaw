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
  profileReasoningMax: "Maximum",
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

it("offers only the selected model's reasoning levels", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(
    <ReasoningControls
      value="minimal"
      supportedEfforts={["none", "low", "medium", "high", "xhigh", "max"]}
      onChange={onChange}
      t={t}
    />,
  );
  const selector = screen.getByRole("combobox", { name: "Reasoning strategy" });
  expect(selector).not.toHaveTextContent("Low");
  expect(onChange).not.toHaveBeenCalled();
  await user.click(selector);
  expect(screen.queryByRole("option", { name: "Minimal" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: "Maximum" }));
  expect(onChange).toHaveBeenCalledWith("max");
});

it("does not invent levels for a model with no advertised reasoning support", async () => {
  const user = userEvent.setup();
  render(<ReasoningControls value="auto" supportedEfforts={[]} onChange={vi.fn()} t={t} />);
  await user.click(screen.getByRole("combobox", { name: "Reasoning strategy" }));
  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getByRole("option", { name: "Model default" })).toBeInTheDocument();
});
