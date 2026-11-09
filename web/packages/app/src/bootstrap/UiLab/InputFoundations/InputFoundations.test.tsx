import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { InputFoundations } from "./InputFoundations";

describe("InputFoundations", () => {
  it("associates actionable validation errors and preserves edits across size and state changes", async () => {
    render(<InputFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "输入框交互试用" }));
    const email = demo.getByRole("textbox", { name: "邮箱" });
    const description = demo.getByRole("textbox", { name: "描述" });
    await user.type(email, "invalid");
    await user.click(demo.getByRole("button", { name: "校验示例" }));
    expect(email).toHaveAttribute("aria-invalid", "true");
    expect(email).toHaveAccessibleDescription("请输入有效邮箱，例如 name@example.com。");
    expect(description).toHaveAccessibleDescription("请输入描述。");
    await user.click(demo.getByRole("radio", { name: "sm · 40px" }));
    await user.click(demo.getByRole("radio", { name: "禁用" }));
    expect(email).toHaveValue("invalid");
    expect(email).toBeDisabled();
    expect(description).toBeDisabled();
    expect(demo.getByRole("button", { name: "校验示例" })).toBeDisabled();
    await user.click(demo.getByRole("radio", { name: "正常" }));
    await user.clear(email);
    await user.type(email, "name@example.com");
    await user.type(description, "第一行{Enter}第二行");
    expect(description).toHaveValue("第一行\n第二行");
    await user.click(demo.getByRole("button", { name: "校验示例" }));
    expect(email).not.toHaveAttribute("aria-invalid", "true");
    expect(demo.getByRole("status")).toHaveTextContent("本地校验通过");
    await user.click(demo.getByRole("button", { name: "重置示例" }));
    expect(email).toHaveValue("");
    expect(description).toHaveValue("");
  });

  it("exposes source links and distinguishes verified references from pending nodes", async () => {
    render(<InputFoundations />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: "组合输入" }));
    expect(screen.getByRole("heading", { name: "Payment input" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Trailing button" })).toBeVisible();
    await user.click(screen.getByRole("radio", { name: "多行文本" }));
    expect(screen.getByRole("heading", { name: "错误 · 聚焦" })).toBeVisible();
    expect(screen.getAllByRole("link", { name: "查看设计节点 ↗" })).toHaveLength(7);
    await user.click(screen.getByText("全部节点索引与待补项（130）"));
    expect(screen.getAllByText(/样式待读取/).length).toBeGreaterThan(1);
  });
});
