import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { UiLabNavigation } from "./UiLabNavigation";
import { sectionFromHash } from "./navigation";

describe("UI Lab directory", () => {
  it("filters links, exposes the current page and recovers from empty results", async () => {
    render(<UiLabNavigation active="input-foundations" />);
    const user = userEvent.setup();
    const nav = within(screen.getByRole("navigation", { name: "UI Lab 组件导航" }));
    expect(nav.getByRole("link", { name: "输入框" })).toHaveAttribute("aria-current", "page");
    const search = nav.getByRole("searchbox", { name: "搜索组件目录" });
    await user.type(search, "按钮");
    expect(nav.getAllByRole("link")).toHaveLength(3);
    expect(nav.queryByRole("link", { name: "输入框" })).not.toBeInTheDocument();
    await user.clear(search);
    await user.type(search, "missing-component");
    expect(nav.getByRole("status")).toHaveTextContent("未找到匹配组件");
    await user.clear(search);
    expect(nav.getAllByRole("link")).toHaveLength(20);
  });

  it("closes the compact directory after selecting a destination", async () => {
    render(<UiLabNavigation active="input-foundations" />);
    const user = userEvent.setup();
    const toggle = screen.getByRole("button", { name: "展开目录" });
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    await user.click(screen.getByRole("link", { name: "输入框" }));
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(sectionFromHash("#input-foundations")).toBe("input-foundations");
    expect(sectionFromHash("#selection-controls")).toBe("selection-controls");
    expect(sectionFromHash("#unknown")).toBe("design-foundations");
  });
});
