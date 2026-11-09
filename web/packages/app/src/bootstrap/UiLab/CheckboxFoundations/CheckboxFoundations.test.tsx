import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { CheckboxFoundations } from "./CheckboxFoundations";

describe("CheckboxFoundations", () => {
  it("derives mixed/all/empty states and retains selections through size and disabled changes", async () => {
    render(<CheckboxFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "复选框交互试用" }));
    const all = demo.getByRole("checkbox", { name: "全选通知" });
    expect(all).toBePartiallyChecked();
    expect(all).toHaveAccessibleDescription("选择部分通知后，全选复选框显示半选状态。");
    await user.click(demo.getByText("全选通知"));
    expect(all).toBeChecked();
    expect(demo.getByRole("checkbox", { name: "桌面通知" })).toBeChecked();
    all.focus();
    await user.keyboard(" ");
    expect(all).not.toBeChecked();
    expect(demo.getByRole("checkbox", { name: "邮件通知" })).not.toBeChecked();
    await user.click(demo.getByRole("checkbox", { name: "声音提醒" }));
    expect(all).toBePartiallyChecked();
    await user.click(screen.getByRole("radio", { name: "sm · 16px" }));
    await user.click(demo.getByRole("radio", { name: "禁用" }));
    expect(all).toBeDisabled();
    expect(demo.getByRole("checkbox", { name: "声音提醒" })).toBeDisabled();
    expect(demo.getByRole("status")).toHaveTextContent("已选择 1 / 3 项 · 半选（已禁用）");
    await user.click(demo.getByRole("button", { name: "重置示例" }));
    expect(all).toBeEnabled();
    expect(all).toBePartiallyChecked();
    expect(demo.getByRole("checkbox", { name: "邮件通知" })).toBeChecked();
  });

  it("supports edited labels and exposes only the actual source combinations", async () => {
    render(<CheckboxFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "复选框交互试用" }));
    const title = demo.getByRole("textbox", { name: "复选框标题" });
    await user.clear(title);
    await user.type(title, "选择全部方式");
    expect(demo.getByRole("checkbox", { name: "选择全部方式" })).toBePartiallyChecked();
    await user.click(demo.getByRole("radio", { name: "隐藏说明" }));
    expect(demo.getByRole("checkbox", { name: "选择全部方式" })).not.toHaveAttribute("aria-describedby");
    expect(screen.getAllByRole("link", { name: "查看设计节点 ↗" })).toHaveLength(8);
    await user.click(screen.getByRole("radio", { name: "仅复选框" }));
    expect(screen.getAllByRole("link", { name: "查看设计节点 ↗" })).toHaveLength(12);
    expect(screen.getByRole("heading", { name: "禁用 · 半选" })).toBeVisible();
    expect(demo.getByRole("checkbox", { name: "选择全部方式" })).toBePartiallyChecked();
  });
});
