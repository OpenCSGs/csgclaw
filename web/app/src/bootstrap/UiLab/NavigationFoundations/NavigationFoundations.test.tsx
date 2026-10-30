import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TabsFoundations, FooterFoundations, PaginationFoundations, StepsFoundations } from "./NavigationFoundations";

describe("navigation foundations", () => {
  it("switches associated tab panels with the keyboard", async () => {
    render(<TabsFoundations />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "个人信息" }));
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "个人资料" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveAccessibleName("个人资料");
    expect(screen.getByText("编辑公开资料。")).toBeVisible();
  });
  it("updates records and disables boundary navigation", async () => {
    render(<PaginationFoundations />);
    const user = userEvent.setup();
    expect(screen.getByRole("button", { name: "上一页" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "页码 5" }));
    expect(screen.getByText("示例记录 13")).toBeVisible();
    expect(screen.getByRole("button", { name: "下一页" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "上一页" }));
    expect(screen.getByRole("status")).toHaveTextContent("4 / 5");
  });
  it("restores the saved draft when cancelling edits", async () => {
    render(<FooterFoundations />);
    const user = userEvent.setup();
    const input = screen.getByRole("textbox");
    await user.type(input, "Saved draft");
    await user.click(screen.getByRole("button", { name: "保存" }));
    await user.clear(input);
    await user.type(input, "Unsaved");
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(input).toHaveValue("Saved draft");
    expect(screen.getByRole("status")).toHaveTextContent("已取消本次修改。");
  });
  it("completes and restarts the step flow", async () => {
    const { container } = render(<StepsFoundations />);
    const user = userEvent.setup();
    expect(screen.getByRole("button", { name: "上一步" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "下一步" }));
    await user.click(screen.getByRole("button", { name: "下一步" }));
    await user.click(screen.getByRole("button", { name: "完成" }));
    expect(screen.getByRole("status")).toHaveTextContent("所有步骤已完成。");
    expect(container.querySelector('[aria-current="step"]')).toBeNull();
    await user.click(screen.getByRole("button", { name: "重新开始" }));
    expect(container.querySelector('[aria-current="step"]')).toHaveTextContent("个人信息");
  });
});
