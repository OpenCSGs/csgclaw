import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ToggleFoundations } from "./ToggleFoundations";

describe("ToggleFoundations", () => {
  it("supports label and keyboard toggling, retains values between variants and blocks disabled activation", async () => {
    render(<ToggleFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "开关交互试用" }));
    const toggle = demo.getByRole("switch", { name: "记住我" });
    expect(toggle).toHaveAccessibleDescription("保存登录信息，以便下次使用。");
    await user.click(demo.getByText("记住我"));
    expect(toggle).toBeChecked();
    toggle.focus();
    await user.keyboard(" ");
    expect(toggle).not.toBeChecked();
    await user.click(toggle);
    await user.click(screen.getByRole("radio", { name: "Slim" }));
    await user.click(screen.getByRole("radio", { name: "sm" }));
    expect(toggle).toBeChecked();
    await user.click(demo.getByRole("radio", { name: "禁用" }));
    expect(toggle).toBeDisabled();
    await user.click(demo.getByText("记住我"));
    expect(toggle).toBeChecked();
    await user.click(demo.getByRole("button", { name: "重置示例" }));
    expect(toggle).not.toBeChecked();
    expect(toggle).toBeEnabled();
  });

  it("edits labels, hides supporting text and provides all eight source states per selection", async () => {
    render(<ToggleFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "开关交互试用" }));
    const label = demo.getByRole("textbox", { name: "开关标题" });
    await user.clear(label);
    await user.type(label, "开启通知");
    expect(demo.getByRole("switch", { name: "开启通知" })).toBeInTheDocument();
    await user.click(demo.getByRole("radio", { name: "隐藏说明" }));
    expect(demo.getByRole("switch", { name: "开启通知" })).not.toHaveAttribute("aria-describedby");
    await user.click(screen.getByRole("radio", { name: "仅开关" }));
    expect(demo.getByRole("switch", { name: "切换示例" })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "查看设计节点 ↗" })).toHaveLength(8);
    expect(screen.getByRole("heading", { name: "聚焦 · 开启" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "禁用 · 关闭" })).toBeInTheDocument();
  });
});
