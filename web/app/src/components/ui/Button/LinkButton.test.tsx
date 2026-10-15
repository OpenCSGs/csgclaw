import { createRef, type MouseEvent } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LinkButton } from "./Button";
import styles from "./Button.module.css";

test("navigation links retain native anchor behavior and scoped button presentation", async () => {
  const user = userEvent.setup();
  const ref = createRef<HTMLAnchorElement>();
  const activate = vi.fn((event: MouseEvent<HTMLAnchorElement>) => event.preventDefault());
  render(
    <LinkButton ref={ref} href="#/connectors" target="_blank" rel="noopener noreferrer" size="sm" onClick={activate}>
      Manage resources
    </LinkButton>,
  );
  const link = screen.getByRole("link", { name: "Manage resources" });
  expect(ref.current).toBe(link);
  expect(link).toHaveAttribute("href", "#/connectors");
  expect(link).toHaveAttribute("target", "_blank");
  expect(link).toHaveAttribute("rel", "noopener noreferrer");
  expect(link).toHaveClass(styles.btn, styles["btn-secondary-gray"], styles["btn-sm"]);
  expect(link.querySelector(`.${styles["btn-content"]}`)).toHaveTextContent("Manage resources");
  await user.tab();
  expect(link).toHaveFocus();
  await user.keyboard("{Enter}");
  expect(activate).toHaveBeenCalledTimes(1);
});
