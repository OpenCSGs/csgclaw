import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Checkbox } from "./Checkbox";

describe("Checkbox indicator", () => {
  it("keeps the legacy default checkbox keyboard behavior", async () => {
    render(<Checkbox aria-label="默认复选框" defaultChecked />);
    const user = userEvent.setup();
    const checkbox = screen.getByRole("checkbox", { name: "默认复选框" });
    expect(checkbox).toBeChecked();
    checkbox.focus();
    await user.keyboard(" ");
    expect(checkbox).not.toBeChecked();
  });

  it("preserves mixed semantics and disabled activation with a custom indicator", async () => {
    const change = vi.fn();
    const { rerender } = render(
      <Checkbox
        aria-label="自定义复选框"
        checked="indeterminate"
        indicator={<span aria-hidden="true">−</span>}
        onCheckedChange={change}
      />,
    );
    const checkbox = screen.getByRole("checkbox", { name: "自定义复选框" });
    expect(checkbox).toBePartiallyChecked();
    const user = userEvent.setup();
    checkbox.focus();
    await user.keyboard(" ");
    expect(change).toHaveBeenLastCalledWith(true);
    rerender(
      <Checkbox
        aria-label="自定义复选框"
        checked="indeterminate"
        indicator={<span aria-hidden="true">−</span>}
        disabled
        onCheckedChange={change}
      />,
    );
    change.mockClear();
    await user.click(checkbox);
    expect(change).not.toHaveBeenCalled();
  });
});
