import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ApplicationFoundations } from "./ApplicationFoundations";
import { monthDays, shiftMonth } from "./calendarModel";

describe("application component playgrounds", () => {
  it("filters table rows and retains selection", async () => {
    render(<ApplicationFoundations kind="tables" />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "选择 Olivia Rhye" }));
    await user.type(screen.getByRole("searchbox", { name: "筛选成员" }), "missing");
    expect(screen.getByText("没有匹配的成员。")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("已选择 1 / 3");
    await user.clear(screen.getByRole("searchbox", { name: "筛选成员" }));
    expect(screen.getByRole("checkbox", { name: "选择 Olivia Rhye" })).toBeChecked();
  });
  it("discards unsaved modal edits and restores trigger focus", async () => {
    render(<ApplicationFoundations kind="modals" />);
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
    render(<ApplicationFoundations kind="date-pickers" />);
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
    render(<ApplicationFoundations kind="calendars" />);
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
    render(<ApplicationFoundations kind="messaging" />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "发送消息" }));
    expect(screen.getByText("请输入消息内容。")).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "消息内容" }), "First{Enter}Second");
    await user.click(screen.getByRole("button", { name: "发送消息" }));
    expect(screen.getByRole("textbox", { name: "消息内容" })).toHaveValue("");
    expect(screen.getByText(/First\s+Second/)).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("已发送 1");
  });
  it("rejects invalid uploads and recovers with a valid local file", () => {
    render(<ApplicationFoundations kind="file-upload" />);
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
    render(<ApplicationFoundations kind="command-menus" />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: "命令菜单" }));
    await user.type(screen.getByRole("combobox", { name: "搜索命令" }), "帮助");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(screen.getByRole("status")).toHaveTextContent("已选择命令：帮助中心");
  });
  it("renders a local reference on demand and filters its design index", async () => {
    const { container } = render(<ApplicationFoundations kind="metrics" />);
    const user = userEvent.setup();
    expect(container.querySelector('[data-node-id="1560:266242"]')).toBeNull();
    await user.click(screen.getByText("Figma 原尺寸参考", { exact: true }));
    expect(await screen.findByRole("region", { name: "Figma 原尺寸参考" })).toBeVisible();
    expect(container.querySelector('[data-node-id="1560:266242"]')).toBeInTheDocument();
    await user.click(screen.getByText("设计节点索引", { exact: false }));
    await user.type(screen.getByRole("searchbox", { name: "筛选设计节点" }), "not-a-node");
    expect(await screen.findByText("未找到匹配节点。")).toBeVisible();
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
