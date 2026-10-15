import { render, screen } from "@testing-library/react";
import { Field, TextArea, TextInput } from "./FormControls";

test("fields associate their labels, hints and errors with the actual text control", () => {
  render(
    <Field label="Token" hint="Stored locally" error="Required" required>
      <TextInput aria-describedby="external" />
      <span id="external">More information</span>
    </Field>,
  );
  const input = screen.getByRole("textbox", { name: "Token" });
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(input).toHaveAttribute("aria-required", "true");
  expect(input).toHaveAccessibleDescription("Stored locally Required More information");
});

test("explicit names and IDs are preserved", () => {
  render(
    <Field label="Document">
      <TextArea id="content" aria-label="Instructions" />
    </Field>,
  );
  expect(screen.getByRole("textbox", { name: "Instructions" })).toHaveAttribute("id", "content");
});

test("explicit required accessibility state survives field defaults", () => {
  render(
    <Field label="Model">
      <TextInput aria-required="true" />
    </Field>,
  );
  expect(screen.getByRole("textbox", { name: "Model" })).toHaveAttribute("aria-required", "true");
});
