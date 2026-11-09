import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TagFoundations } from "./TagFoundations";

describe("TagFoundations", () => {
  it("supports checkbox keyboard interaction, removal and restoration", async () => {
    render(<TagFoundations />);
    const user = userEvent.setup();
    const checkbox = screen.getByRole("checkbox", { name: "选择标签" });
    checkbox.focus();
    await user.keyboard(" ");
    expect(checkbox).toBeChecked();
    await user.click(screen.getByRole("button", { name: "移除标签" }));
    expect(screen.getByRole("status")).toHaveTextContent("标签已移除");
    expect(screen.queryByRole("checkbox", { name: "选择标签" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "恢复标签" }));
    expect(screen.getByRole("checkbox", { name: "选择标签" })).not.toBeChecked();
  });

  it("previews edited labels and marks unread combinations explicitly", async () => {
    render(<TagFoundations />);
    const user = userEvent.setup();
    await user.clear(screen.getByRole("textbox", { name: "标签文案" }));
    await user.type(screen.getByRole("textbox", { name: "标签文案" }), "产品设计");
    expect(within(screen.getByRole("region", { name: "标签交互试用" })).getByText("产品设计")).toBeVisible();
    await user.click(screen.getByRole("radio", { name: "lg" }));
    await user.click(screen.getByRole("radio", { name: "头像" }));
    await user.click(screen.getByRole("radio", { name: "计数" }));
    expect(
      within(screen.getByRole("article", { name: "Avatar Count 带复选框" })).getByText("样式待读取"),
    ).toBeVisible();
  });
});
