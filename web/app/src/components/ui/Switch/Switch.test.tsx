import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Switch } from "./Switch";

function Harness() {
  const [checked, setChecked] = useState(false);
  return <Switch aria-label="Code review" checked={checked} onCheckedChange={setChecked} />;
}

describe("Switch", () => {
  it("supports pointer and keyboard changes with an accessible checked state", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const control = screen.getByRole("switch", { name: "Code review" });
    expect(control).not.toBeChecked();
    await user.click(control);
    expect(control).toBeChecked();
    await user.keyboard(" ");
    expect(control).not.toBeChecked();
  });

  it("does not change a disabled switch", async () => {
    const onChange = vi.fn();
    render(<Switch aria-label="Code review" checked disabled onCheckedChange={onChange} />);
    const control = screen.getByRole("switch", { name: "Code review" });
    await userEvent.click(control);
    expect(control).toBeChecked();
    expect(onChange).not.toHaveBeenCalled();
  });
});
