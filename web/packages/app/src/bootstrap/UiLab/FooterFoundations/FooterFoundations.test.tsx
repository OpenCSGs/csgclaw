import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FooterFoundations } from "./FooterFoundations";
describe("Figma section footers", () => {
  it("renders eight actual variants and retains the source dimensions", () => {
    const { container } = render(<FooterFoundations />);
    expect(container.querySelectorAll("[data-node-id]")).toHaveLength(8);
    expect(container.querySelectorAll('[data-width="Desktop"]')).toHaveLength(4);
    expect(container.querySelectorAll('[data-width="Mobile"]')).toHaveLength(4);
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
  it("changes range selection and exposes action feedback", async () => {
    render(<FooterFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "区块页脚交互试用" }));
    await user.click(demo.getByLabelText("30 天"));
    expect(demo.getByLabelText("30 天")).toHaveAttribute("data-state", "on");
    expect(demo.getByLabelText("12 个月")).toHaveAttribute("data-state", "off");
    await user.click(demo.getByRole("button", { name: "主要" }));
    expect(demo.getByRole("status")).toHaveTextContent("主要");
  });
  it("controls optional footer content without affecting gallery samples", async () => {
    const { container } = render(<FooterFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "区块页脚交互试用" }));
    await user.click(demo.getByRole("checkbox", { name: "按钮组" }));
    expect(demo.queryByLabelText("30 天")).not.toBeInTheDocument();
    expect(container.querySelectorAll('[data-node-id] [aria-label="时间范围"]')).toHaveLength(4);
    await user.click(demo.getByRole("checkbox", { name: "操作按钮" }));
    expect(demo.queryByRole("button", { name: "主要" })).not.toBeInTheDocument();
  });
  it("selects ranges with the keyboard and retains selection when changing the footer variant", async () => {
    render(<FooterFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "区块页脚交互试用" }));
    await user.click(demo.getByRole("radio", { name: "12 个月" }));
    await user.keyboard("{ArrowRight} ");
    expect(demo.getByRole("radio", { name: "30 天" })).toBeChecked();
    expect(demo.getByRole("status")).toHaveTextContent("30 天");
    await user.click(demo.getByRole("radio", { name: "卡片" }));
    await user.click(demo.getByRole("radio", { name: "移动端" }));
    expect(demo.getByRole("radio", { name: "30 天" })).toBeChecked();
  });
});
