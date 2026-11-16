import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LayoutFoundations } from "./LayoutFoundations";

vi.mock("@/shared/styles/tokens.css?raw", async () => {
  const { readFileSync } = await import("node:fs");
  return { default: readFileSync("src/shared/styles/tokens.css", "utf8") };
});

describe("LayoutFoundations", () => {
  it("exposes naming conflicts separately from equivalent code candidates", async () => {
    render(<LayoutFoundations />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: "圆角" }));
    const xs = within(screen.getByRole("article", { name: "radius-xs" }));
    expect(xs.getByText("同名代码 --radius-xs：2px · 数值冲突")).toBeVisible();
    expect(xs.getByText("等值候选：--radius-sm")).toBeVisible();
    await user.click(screen.getByRole("radio", { name: "间距标尺" }));
    const six = within(screen.getByRole("article", { name: "1.5" }));
    expect(six.getByText("同名代码 --space-1-5：未定义")).toBeVisible();
  });

  it("switches source grid presets without treating them as breakpoints", async () => {
    render(<LayoutFoundations />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: "网格" }));
    expect(screen.getByRole("status")).toHaveTextContent("12 列 · 列间距 32px · 单侧边距 112px");
    await user.click(screen.getByRole("radio", { name: "Tablet" }));
    expect(screen.getByRole("status")).toHaveTextContent("768px 画布 · 6 列");
    await user.click(screen.getByRole("radio", { name: "Mobile" }));
    expect(screen.getByRole("status")).toHaveTextContent("375px 画布 · 4 列 · 列间距 16px");
    expect(screen.getByRole("link", { name: "查看当前网格节点 ↗" })).toHaveAttribute(
      "href",
      expect.stringContaining("5245-373048"),
    );
  });
});
