import { render, screen } from "@testing-library/react";
import { Button, IconButton } from "@/components/ui";

describe("Button", () => {
  it("maps the expanded design-system variants and sizes to CSS classes", () => {
    const { rerender } = render(
      <Button variant="tertiaryColor" size="xl">
        Edit
      </Button>,
    );

    expect(screen.getByRole("button", { name: "Edit" })).toHaveClass("btn-tertiary-color", "btn-xl");

    rerender(
      <Button variant="linkDanger" size="2xl">
        Delete
      </Button>,
    );

    expect(screen.getByRole("button", { name: "Delete" })).toHaveClass("btn-link-danger", "btn-2xl");
  });

  it("keeps legacy ghost usage aligned with tertiary gray styling", () => {
    render(<Button variant="ghost">Open</Button>);

    expect(screen.getByRole("button", { name: "Open" })).toHaveClass("btn-tertiary-gray");
  });

  it("provides an accessible name for icon-only button variants", () => {
    render(<IconButton icon={<span>+</span>} label="Create" variant="danger" size="md" />);

    expect(screen.getByRole("button", { name: "Create" })).toHaveClass("btn-danger", "btn-md", "csg-icon-button");
  });
});
