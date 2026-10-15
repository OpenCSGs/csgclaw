import { render, screen } from "@testing-library/react";
import { DropdownMenuContent, DropdownMenuItem, DropdownMenuRoot } from "./DropdownMenu";
import styles from "./DropdownMenu.module.css";

test.each(["warning", "danger"] as const)("%s menu actions retain their local tone styles", (tone) => {
  render(
    <DropdownMenuRoot defaultOpen>
      <DropdownMenuContent>
        <DropdownMenuItem tone={tone}>Action</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenuRoot>,
  );
  expect(screen.getByRole("menuitem", { name: "Action" })).toHaveClass(styles[tone]);
});

test("the danger shorthand retains its tone", () => {
  render(
    <DropdownMenuRoot defaultOpen>
      <DropdownMenuContent>
        <DropdownMenuItem danger>Delete</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenuRoot>,
  );
  expect(screen.getByRole("menuitem", { name: "Delete" })).toHaveClass(styles.danger);
});
