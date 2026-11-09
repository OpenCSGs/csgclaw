import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TypographyFoundations } from "./TypographyFoundations";

describe("TypographyFoundations", () => {
  it("updates samples and switches between design and production styles", async () => {
    render(<TypographyFoundations />);
    const user = userEvent.setup();
    const field = screen.getByRole("textbox", { name: "预览文案 / Preview text" });
    await user.clear(field);
    await user.type(field, "协作 ABC");
    const largest = screen.getByRole("region", { name: "display-2xl" });
    expect(within(largest).getAllByText("协作 ABC")).toHaveLength(4);
    expect(within(largest).getAllByText("协作 ABC")[0]).toHaveStyle({
      fontSize: "72px",
      fontWeight: "300",
      lineHeight: "90px",
    });
    await user.click(screen.getByRole("radio", { name: "现有代码" }));
    expect(screen.getByRole("radio", { name: "现有代码" })).toBeChecked();
    expect(within(largest).getAllByText("协作 ABC")[0].style.font).toContain("var(--text-7xl-light)");
  });
});
