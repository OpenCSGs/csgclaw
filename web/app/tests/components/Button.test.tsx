import { render, screen } from "@testing-library/react";
import { Button, IconButton } from "@/components/ui";
import userEvent from "@testing-library/user-event";
import styles from "@/components/ui/Button/Button.module.css";

describe("Button", () => {
  it("maps the expanded design-system variants and sizes to CSS classes", () => {
    const { rerender } = render(
      <Button variant="tertiaryColor" size="xl">
        Edit
      </Button>,
    );

    expect(screen.getByRole("button", { name: "Edit" })).toHaveClass(styles["btn-tertiary-color"], styles["btn-xl"]);

    rerender(
      <Button variant="linkDanger" size="2xl">
        Delete
      </Button>,
    );

    expect(screen.getByRole("button", { name: "Delete" })).toHaveClass(styles["btn-link-danger"], styles["btn-2xl"]);
  });

  it("defaults buttons to the medium design-system size", () => {
    render(<Button variant="ghost">Open</Button>);

    expect(screen.getByRole("button", { name: "Open" })).toHaveClass(styles["btn-tertiary-gray"], styles["btn-md"]);
  });

  it("provides an accessible name and medium default for icon-only button variants", () => {
    render(<IconButton icon={<span>+</span>} label="Create" variant="danger" />);

    expect(screen.getByRole("button", { name: "Create" })).toHaveClass(
      styles["btn-danger"],
      styles["btn-md"],
      styles["csg-icon-button"],
    );
  });

  it("renders a stable loading state", () => {
    render(
      <Button loading loadingLabel="Saving" disabled>
        Save
      </Button>,
    );

    const button = screen.getByRole("button", { name: "Saving" });
    expect(button).toHaveClass(styles["btn-loading"]);
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button.querySelector(`.${styles["btn-loading-spinner"]}`)).toBeInTheDocument();
    expect(screen.getByText("Saving")).toBeVisible();
    expect(screen.queryByText("Save")).not.toBeInTheDocument();
  });

  it("prevents repeated activation while loading and restores the action when ready", async () => {
    const click = vi.fn();
    const user = userEvent.setup();
    const view = render(
      <Button loading loadingLabel={<span>正在保存</span>} onClick={click}>
        保存
      </Button>,
    );
    const busy = screen.getByRole("button", { name: "正在保存" });
    await user.click(busy);
    expect(click).not.toHaveBeenCalled();
    view.rerender(<Button onClick={click}>保存</Button>);
    await user.click(screen.getByRole("button", { name: "保存" }));
    expect(click).toHaveBeenCalledOnce();
    expect(screen.getByRole("button")).not.toHaveAttribute("aria-busy");
  });

  it("replaces decorative icons with a spinner and keeps fallback child text", () => {
    render(
      <Button loading>
        <svg aria-hidden="true" data-testid="action-icon" />
        保存
      </Button>,
    );
    expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
    expect(screen.getByText("保存")).toBeVisible();
    expect(screen.queryByTestId("action-icon")).not.toBeInTheDocument();
  });

  it("keeps icon-only loading buttons named without rendering the loading text", () => {
    render(<IconButton loading loadingLabel="Creating" icon={<span>+</span>} label="Create" />);
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
    expect(screen.queryByText("Creating")).not.toBeInTheDocument();
    expect(screen.queryByText("+")).not.toBeInTheDocument();
  });
});
