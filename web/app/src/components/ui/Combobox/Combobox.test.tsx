import { useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Combobox } from "./Combobox";
import { Field } from "@/components/ui/FormControls";
import { DialogContent, DialogRoot, DialogTitle } from "@/components/ui/Dialog";

const options = [
  { value: "alpha", label: "Alpha" },
  { value: "disabled", label: "Disabled", disabled: true },
  { value: "beta", label: "Beta" },
];

function Harness({ onChange = () => {} }: { onChange?: (value: string) => void }) {
  const [value, setValue] = useState("alpha");
  return (
    <>
      <Field label="Model" hint="Choose a model" error="Check this value">
        <Combobox
          value={value}
          options={options}
          searchPlaceholder="Search models"
          emptyLabel="No matches"
          onValueChange={(next) => {
            setValue(next);
            onChange(next);
          }}
        />
      </Field>
      <button>Next field</button>
    </>
  );
}

test("search, keyboard selection and focus restoration work without selecting disabled options", async () => {
  const user = userEvent.setup();
  const changed = vi.fn();
  render(<Harness onChange={changed} />);
  const trigger = screen.getByRole("combobox", { name: "Model" });
  expect(trigger).toHaveAttribute("aria-invalid", "true");
  expect(trigger).toHaveAccessibleDescription("Choose a model Check this value");
  await user.click(trigger);
  const input = screen.getByRole("combobox", { name: "Search models" });
  expect(input).toHaveFocus();
  await user.keyboard("{ArrowDown}{Enter}");
  expect(changed).toHaveBeenCalledWith("beta");
  await waitFor(() => expect(trigger).toHaveFocus());
  expect(trigger).toHaveTextContent("Beta");
  await user.click(trigger);
  await user.type(screen.getByRole("combobox", { name: "Search models" }), "absent");
  expect(screen.getByRole("status")).toHaveTextContent("No matches");
  await user.keyboard("{Enter}");
  expect(changed).toHaveBeenCalledTimes(1);
  await user.keyboard("{Escape}");
  expect(trigger).toHaveTextContent("Beta");
});

test("IME confirmation does not select an option and Tab leaves the picker", async () => {
  const user = userEvent.setup();
  const changed = vi.fn();
  render(<Harness onChange={changed} />);
  await user.click(screen.getByRole("combobox", { name: "Model" }));
  const input = screen.getByRole("combobox", { name: "Search models" });
  fireEvent.compositionStart(input);
  fireEvent.keyDown(input, { key: "Enter", keyCode: 229, isComposing: true });
  expect(changed).not.toHaveBeenCalled();
  fireEvent.compositionEnd(input);
  await user.tab();
  await waitFor(() => expect(screen.getByRole("button", { name: "Next field" })).toHaveFocus());
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
});

test("Escape dismisses the picker while leaving its containing dialog open", async () => {
  const user = userEvent.setup();
  const closed = vi.fn();
  render(
    <DialogRoot defaultOpen onOpenChange={closed}>
      <DialogContent aria-describedby={undefined}>
        <DialogTitle>Configure</DialogTitle>
        <Harness />
      </DialogContent>
    </DialogRoot>,
  );
  await user.click(screen.getByRole("combobox", { name: "Model" }));
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog", { name: "Configure" })).toBeInTheDocument();
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  expect(closed).not.toHaveBeenCalled();
});

test("business empty values submit as empty and unavailable saved values remain visible", async () => {
  const user = userEvent.setup();
  const changed = vi.fn();
  const view = render(
    <form aria-label="Configuration">
      <Combobox
        name="model"
        defaultValue="old"
        options={[
          { value: "", label: "None" },
          { value: "old", label: "Saved model", disabled: true },
        ]}
        searchPlaceholder="Find"
        onValueChange={changed}
      />
    </form>,
  );
  const trigger = screen.getByRole("combobox");
  expect(trigger).toHaveTextContent("Saved model");
  await user.click(trigger);
  await user.click(screen.getByRole("option", { name: "Saved model" }));
  expect(changed).not.toHaveBeenCalled();
  await user.click(screen.getByRole("option", { name: "None" }));
  expect(changed).toHaveBeenCalledWith("");
  expect(new FormData(view.container.querySelector("form")!).get("model")).toBe("");
});

test("a disabled picker cannot be opened", async () => {
  const user = userEvent.setup();
  render(<Combobox disabled value="alpha" options={options} searchPlaceholder="Find" />);
  await user.click(screen.getByRole("combobox"));
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
});

test("Tab and Shift+Tab leave a portalled picker within the containing dialog", async () => {
  const user = userEvent.setup();
  render(
    <DialogRoot defaultOpen>
      <DialogContent aria-describedby={undefined}>
        <DialogTitle>Configure</DialogTitle>
        <button>Previous field</button>
        <Harness />
      </DialogContent>
    </DialogRoot>,
  );
  const trigger = screen.getByRole("combobox", { name: "Model" });
  await user.click(trigger);
  expect(screen.getByRole("listbox").closest('[role="dialog"]')).toBeNull();
  await user.tab();
  await waitFor(() => expect(screen.getByRole("button", { name: "Next field" })).toHaveFocus());
  await user.click(trigger);
  await user.tab({ shift: true });
  await waitFor(() => expect(screen.getByRole("button", { name: "Previous field" })).toHaveFocus());
  expect(screen.getByRole("dialog", { name: "Configure" })).toBeInTheDocument();
});

test("a controlled saved value stays visible while async options load and then becomes selectable", async () => {
  const user = userEvent.setup();
  const changed = vi.fn();
  const props = { value: "old", searchPlaceholder: "Find", onValueChange: changed };
  const view = render(<Combobox {...props} options={[]} loading loadingLabel="Loading models" />);
  const trigger = screen.getByRole("combobox");
  expect(trigger).toHaveTextContent("old");
  await user.click(trigger);
  expect(screen.getByRole("status")).toHaveTextContent("Loading models");
  await user.keyboard("{Enter}");
  expect(changed).not.toHaveBeenCalled();
  view.rerender(<Combobox {...props} options={[{ value: "old", label: "Saved model" }, ...options]} />);
  await user.type(screen.getByRole("combobox", { name: "Find" }), "beta");
  await user.keyboard("{Enter}");
  expect(changed).toHaveBeenCalledWith("beta");
  expect(trigger).toHaveTextContent("Saved model");
});
