import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Breadcrumb } from "./Breadcrumb";

test("renders semantic breadcrumbs and calls the selected path action", async () => {
  const navigate = vi.fn();
  const user = userEvent.setup();
  const view = render(
    <Breadcrumb
      aria-label="Workspace files"
      home={{ label: "workspace", onClick: navigate }}
      items={[
        { label: "reports", onClick: navigate },
        { label: "annual", current: true },
      ]}
    />,
  );

  expect(screen.getByRole("navigation", { name: "Workspace files" })).toBeInTheDocument();
  expect(view.container.querySelector('button[title="workspace"] img')).toHaveAttribute("width", "20");
  expect(screen.getByText("annual")).toHaveAttribute("aria-current", "page");
  await user.click(screen.getByRole("button", { name: "reports" }));
  expect(navigate).toHaveBeenCalledTimes(1);
});

test("collapses middle levels into an accessible menu and preserves navigable ancestors", async () => {
  const navigate = vi.fn();
  const user = userEvent.setup();
  render(
    <Breadcrumb
      home={{ label: "workspace", onClick: navigate }}
      items={[
        { label: "one", onClick: navigate },
        { label: "two", onClick: navigate },
        { label: "three", onClick: navigate },
        { label: "four", onClick: navigate },
        { label: "five", current: true },
      ]}
    />,
  );

  await user.click(screen.getByRole("button", { name: "Show earlier path segments" }));
  expect(screen.getByRole("menuitem", { name: "two" })).toBeInTheDocument();
  await user.click(screen.getByRole("menuitem", { name: "two" }));
  expect(navigate).toHaveBeenCalledTimes(1);
  expect(screen.getByText("one")).toBeInTheDocument();
  expect(screen.getByText("five")).toHaveAttribute("aria-current", "page");
});
