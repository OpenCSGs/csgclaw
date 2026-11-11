import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ApplicationFoundations } from "./ApplicationFoundations";
import { monthDays, shiftMonth } from "./calendarModel";

function mount(kind: Parameters<typeof ApplicationFoundations>[0]["kind"]) {
  const view = render(<ApplicationFoundations kind={kind} />);
  fireEvent.click(screen.getByText("辅助行为验证", { selector: "summary" }));
  return view;
}

describe("application component playgrounds", () => {
  it("filters table rows and retains selection", async () => {
    mount("tables");
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "选择 Olivia Rhye" }));
    await user.type(screen.getByRole("searchbox", { name: "筛选成员" }), "missing");
    expect(screen.getByText("没有匹配的成员。")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("已选择 1 / 3");
    await user.clear(screen.getByRole("searchbox", { name: "筛选成员" }));
    expect(screen.getByRole("checkbox", { name: "选择 Olivia Rhye" })).toBeChecked();
  });
  it("discards unsaved modal edits and restores trigger focus", async () => {
    mount("modals");
    const user = userEvent.setup();
    const trigger = screen.getByRole("button", { name: "打开弹窗" });
    await user.click(trigger);
    await user.type(screen.getByRole("textbox", { name: "草稿名称" }), "Unsaved");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
    await user.click(trigger);
    expect(screen.getByRole("textbox", { name: "草稿名称" })).toHaveValue("");
  });
  it("requires apply before committing a calendar date", async () => {
    mount("date-pickers");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "选择日期：2025-01-10" }));
    await user.click(screen.getByRole("button", { name: "2025-01-15" }));
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.getByRole("status")).toHaveTextContent("2025-01-10");
    await user.click(screen.getByRole("button", { name: "选择日期：2025-01-10" }));
    await user.click(screen.getByRole("button", { name: "2025-01-15" }));
    await user.click(screen.getByRole("button", { name: "应用日期" }));
    expect(screen.getByRole("status")).toHaveTextContent("2025-01-15");
  });
  it("validates event names and adds an event to the selected day", async () => {
    mount("calendars");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "添加日程" }));
    expect(screen.getByText("请输入日程名称。")).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "日程名称" }), "Review");
    await user.click(screen.getByRole("button", { name: "添加日程" }));
    expect(screen.getByText("Review")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "2025-01-15" }));
    expect(screen.getByText("当天没有日程。")).toBeVisible();
  });
  it("validates messages and preserves multiline content", async () => {
    mount("messaging");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "发送消息" }));
    expect(screen.getByText("请输入消息内容。")).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "消息内容" }), "First{Enter}Second");
    await user.click(screen.getByRole("button", { name: "发送消息" }));
    expect(screen.getByRole("textbox", { name: "消息内容" })).toHaveValue("");
    expect(screen.getByText(/First\s+Second/)).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("已发送 1");
    await user.click(screen.getByRole("button", { name: "重置示例" }));
    expect(screen.queryByText(/First\s+Second/)).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("已发送 0");
  });
  it("rejects invalid uploads and recovers with a valid local file", () => {
    mount("file-upload");
    const input = screen.getByLabelText("选择文件");
    fireEvent.change(input, {
      target: { files: [new File(["x"], "invalid.exe", { type: "application/octet-stream" })] },
    });
    expect(screen.getByRole("alert")).toHaveTextContent("文件类型不支持");
    fireEvent.change(input, { target: { files: [new File(["x"], "demo.png", { type: "image/png" })] } });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("demo.png");
  });
  it("searches and selects a command using the keyboard", async () => {
    mount("command-menus");
    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: "命令菜单" }));
    await user.type(screen.getByRole("combobox", { name: "搜索命令" }), "帮助");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(screen.getByRole("status")).toHaveTextContent("已选择命令：帮助中心");
  });
  it("opens the local reference by default and filters its design index", async () => {
    const { container } = render(<ApplicationFoundations kind="metrics" />);
    const user = userEvent.setup();
    expect(container.querySelector('[data-node-id="1560:266242"]')).toBeInTheDocument();
    expect(screen.getByText("辅助行为验证", { selector: "summary" }).closest("details")).not.toHaveAttribute("open");
    expect(await screen.findByRole("region", { name: "Figma 原尺寸参考" })).toBeVisible();
    await user.click(screen.getByText("Figma 原尺寸参考", { selector: "summary" }));
    expect(container.querySelector('[data-node-id="1560:266242"]')).toBeNull();
    await user.click(screen.getByText("Figma 原尺寸参考", { selector: "summary" }));
    expect(await screen.findByRole("region", { name: "Figma 原尺寸参考" })).toBeVisible();
    await user.click(screen.getByText("设计节点索引", { exact: false }));
    await user.type(screen.getByRole("searchbox", { name: "筛选设计节点" }), "not-a-node");
    expect(await screen.findByText("未找到匹配节点。")).toBeVisible();
  });
  it("opens breadcrumb interactions and navigates collapsed ancestors with the keyboard", async () => {
    render(<ApplicationFoundations kind="breadcrumbs" />);
    const user = userEvent.setup();
    expect(screen.getByText("设计交互", { selector: "summary" }).closest("details")).toHaveAttribute("open");
    expect(screen.queryByRole("region", { name: "Figma 原尺寸参考" })).not.toBeInTheDocument();
    const trigger = screen.getByRole("button", { name: "查看上级路径" });
    await user.click(trigger);
    await user.keyboard("{ArrowDown}{Enter}");
    expect(screen.getByRole("status")).toHaveTextContent("当前位置：工作区");
    expect(trigger).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "恢复完整路径" }));
    expect(screen.getByRole("button", { name: "查看上级路径" })).toBeVisible();
  });
});
describe("calendar model", () => {
  it("handles leap days and year boundaries", () => {
    expect(monthDays(2024, 1).find((d) => d.iso === "2024-02-29")).toMatchObject({ day: 29, inMonth: true });
    expect(monthDays(2025, 1).some((d) => d.iso === "2025-02-29")).toBe(false);
    expect(shiftMonth(2025, 0, -1)).toEqual({ year: 2024, month: 11 });
    expect(shiftMonth(2025, 11, 1)).toEqual({ year: 2026, month: 0 });
  });
});
