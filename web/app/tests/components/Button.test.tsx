import { render, screen } from "@testing-library/react";
import { Button, IconButton } from "@/components/ui";
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
  });
});
