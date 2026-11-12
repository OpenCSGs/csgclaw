import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Field, TextArea, TextInput } from "csgclaw-ui";

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

test("a field action is separate from the label and the label still focuses the input", async () => {
  const user = userEvent.setup();
  render(
    <Field label="Path" labelAction={<button type="button">Help</button>} hint="A local directory">
      <TextInput />
      <button type="button">Browse</button>
    </Field>,
  );
  const input = screen.getByRole("textbox", { name: "Path" });
  const help = screen.getByRole("button", { name: "Help" });
  const browse = screen.getByRole("button", { name: "Browse" });
  expect(help.closest("label")).toBeNull();
  expect(browse.closest("label")).toBeNull();
  expect(input).toHaveAccessibleDescription("A local directory");
  await user.click(screen.getByText("Path"));
  expect(input).toHaveFocus();
});
