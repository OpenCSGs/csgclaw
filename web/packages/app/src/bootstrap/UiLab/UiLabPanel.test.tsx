import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { UiLabPanel } from "./UiLabPanel";
it("defers unvisited panels and retains edits when returning", async () => {
  const child = <input aria-label="Draft" defaultValue="" />;
  const { rerender } = render(
    <UiLabPanel id="example" active="other">
      {child}
    </UiLabPanel>,
  );
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  rerender(
    <UiLabPanel id="example" active="example">
      {child}
    </UiLabPanel>,
  );
  await userEvent.setup().type(screen.getByRole("textbox"), "Retained draft");
  rerender(
    <UiLabPanel id="example" active="other">
      {child}
    </UiLabPanel>,
  );
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  rerender(
    <UiLabPanel id="example" active="example">
      {child}
    </UiLabPanel>,
  );
  expect(screen.getByRole("textbox")).toHaveValue("Retained draft");
});
