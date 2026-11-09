import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ProgressFoundations } from "./ProgressFoundations";

describe("ProgressFoundations", () => {
  it("keeps semantic and visual progress in sync at both boundaries and across label changes", async () => {
    render(<ProgressFoundations />);
    const user = userEvent.setup();
    const demo = within(screen.getByRole("region", { name: "进度交互试用" }));
    const progress = demo.getByRole("progressbar", { name: "任务完成进度" });
    expect(progress).toHaveAttribute("aria-valuenow", "40");
    await user.click(demo.getByRole("button", { name: "从零开始" }));
    expect(progress).toHaveAttribute("aria-valuenow", "0");
    await user.click(screen.getByRole("radio", { name: "顶部浮动标签" }));
    expect(progress).toHaveAttribute("aria-valuenow", "0");
    const slider = demo.getByRole("slider", { name: "任务进度" });
    slider.focus();
    await user.keyboard("{ArrowRight}");
    expect(progress).toHaveAttribute("aria-valuenow", "1");
    await user.keyboard("{End}");
    expect(progress).toHaveAttribute("aria-valuenow", "100");
    expect(demo.getByRole("button", { name: "增加 10%" })).toBeDisabled();
    expect(demo.getByRole("status")).toHaveTextContent("演示已完成 · 100%");
    await user.click(demo.getByRole("button", { name: "重置示例" }));
    expect(progress).toHaveAttribute("aria-valuenow", "40");
    expect(screen.getAllByRole("progressbar")).toHaveLength(1);
  });

  it("provides source references for every bar layout and both sets of ring sizes", async () => {
    render(<ProgressFoundations />);
    const user = userEvent.setup();
    expect(screen.getAllByRole("link", { name: "查看设计节点 ↗" })).toHaveLength(11);
    await user.click(screen.getByRole("radio", { name: "无标签" }));
    expect(screen.getByRole("heading", { name: "无标签 · 0%" })).toBeVisible();
    await user.click(screen.getByRole("radio", { name: "圆环与半圆" }));
    expect(screen.getAllByRole("link", { name: "查看设计节点 ↗" })).toHaveLength(5);
    await user.click(screen.getByRole("radio", { name: "半圆" }));
    expect(screen.getByRole("heading", { name: "xxs · 半圆" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "lg · 半圆" })).toBeVisible();
  });
});
