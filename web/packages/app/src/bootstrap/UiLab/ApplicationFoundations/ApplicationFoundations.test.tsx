import { selectLabOption } from "../selectLabOption.test-helper";
import { LabView } from "../labViewContext";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ApplicationFoundations } from "./ApplicationFoundations";
import { monthDays, shiftMonth } from "./calendarModel";

function mount(kind: Parameters<typeof ApplicationFoundations>[0]["kind"]) {
  return render(<ApplicationFoundations kind={kind} />);
}

describe("application component playgrounds", () => {
  it("dismisses and restores the inline alert", async () => {
    render(<ApplicationFoundations kind="alerts" />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "了解更多" }));
    expect(screen.getByText("此提醒仅用于展示，不会修改真实设置。")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "关闭提醒" }));
    expect(screen.queryByText("请注意，这可能会带来问题")).not.toBeInTheDocument();
    expect(screen.queryByText("此提醒仅用于展示，不会修改真实设置。")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重新显示提醒" }));
    expect(screen.getByText("请注意，这可能会带来问题")).toBeVisible();
  });

  it("runs a notification action and dismisses the toast", async () => {
    render(<ApplicationFoundations kind="alerts" />);
    const user = userEvent.setup();
    await selectLabOption(user, "设计断点", "移动端");
    await user.click(screen.getByRole("button", { name: "触发警告通知" }));
    expect(screen.getByText("项目已取消发布", { exact: true })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "撤销操作" }));
    expect(screen.queryByRole("button", { name: "关闭通知" })).not.toBeInTheDocument();
    expect(screen.getByText("撤销操作", { exact: true })).toHaveAttribute("role", "status");
  });

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
  it("sends messages through the production composer and resets the conversation", async () => {
    mount("messaging");
    const user = userEvent.setup();
    const editor = screen.getByRole("textbox", { name: /输入消息/ });
    const send = screen.getByRole("button", { name: "发送" });
    expect(send).toBeDisabled();
    await user.type(editor, "Message from UI Lab");
    await user.click(send);
    expect(screen.getByText("Message from UI Lab")).toBeVisible();
    expect(editor).toBeEmptyDOMElement();
    await user.click(screen.getByRole("button", { name: "重置示例" }));
    expect(screen.queryByText("Message from UI Lab")).not.toBeInTheDocument();
  });
  it("opens the production thread panel and sends a local reply", async () => {
    mount("messaging");
    const user = userEvent.setup();
    const [reply] = screen.getAllByRole("button", { name: "在话题中回复" });
    await user.click(reply);
    const thread = within(screen.getByRole("complementary"));
    await user.type(thread.getByRole("textbox", { name: "回复这个话题" }), "Reply from UI Lab");
    await user.click(thread.getByRole("button", { name: "发送" }));
    expect(thread.getByText("Reply from UI Lab")).toBeVisible();
  });
  it("replaces rejected files and completes a local upload", async () => {
    mount("file-upload");
    const user = userEvent.setup();
    const input = screen.getByLabelText("选择文件");
    fireEvent.change(input, {
      target: { files: [new File(["x"], "invalid.exe", { type: "application/octet-stream" })] },
    });
    expect(screen.getByRole("alert")).toHaveTextContent("文件类型不支持");
    await user.click(screen.getByRole("button", { name: "重新选择" }));
    fireEvent.change(input, { target: { files: [new File(["x"], "demo.png", { type: "image/png" })] } });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    const progress = screen.getByRole("progressbar", { name: "demo.png" });
    await waitFor(() => expect(progress).toHaveValue(100), { timeout: 2500 });
    await user.click(screen.getByRole("button", { name: "移除文件 demo.png" }));
    expect(progress).not.toBeInTheDocument();
  });
  it("queues multiple dropped files and rejects drops while disabled", async () => {
    mount("file-upload");
    const user = userEvent.setup();
    const target = screen.getByRole("button", { name: /点击上传/ });
    const dataTransfer = {
      files: [new File(["x"], "one.png", { type: "image/png" }), new File(["x"], "two.gif", { type: "image/gif" })],
    };
    fireEvent.drop(target, { dataTransfer });
    expect(screen.getAllByRole("progressbar")).toHaveLength(2);
    await selectLabOption(user, "上传区状态", "禁用");
    expect(target).toBeDisabled();
    fireEvent.drop(target, { dataTransfer });
    expect(screen.getAllByRole("progressbar")).toHaveLength(2);
  });
  it("searches and selects a command using the keyboard", async () => {
    mount("command-menus");
    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: "命令菜单" }));
    await user.type(screen.getByRole("combobox", { name: "搜索命令" }), "帮助");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(screen.getByRole("status")).toHaveTextContent("已选择命令：帮助中心");
  });
  it("opens the interactive example by default and filters the design index", async () => {
    const { container } = render(<ApplicationFoundations kind="metrics" />);
    const user = userEvent.setup();
    expect(screen.getByRole("button", { name: "重置示例" })).toBeVisible();
    expect(container.querySelector('[data-node-id="1560:266242"]')).toBeNull();
    await user.click(screen.getByText("Figma 原尺寸参考", { selector: "summary" }));
    expect(await screen.findByRole("region", { name: "Figma 原尺寸参考" })).toBeVisible();
    expect(container.querySelector('[data-node-id="1560:266242"]')).toBeInTheDocument();
    await user.click(screen.getByText("设计节点索引", { exact: false }));
    await user.type(screen.getByRole("searchbox", { name: "筛选设计节点" }), "not-a-node");
    expect(await screen.findByText("未找到匹配节点。")).toBeVisible();
  });
  it("shows the interaction and Figma references in their respective views", () => {
    const { rerender } = render(
      <LabView.Provider value="preview">
        <ApplicationFoundations kind="page-headers" />
      </LabView.Provider>,
    );
    expect(screen.getByRole("button", { name: "重置示例" })).toBeVisible();
    expect(screen.queryByText("Figma 原尺寸参考")).not.toBeInTheDocument();
    rerender(
      <LabView.Provider value="design">
        <ApplicationFoundations kind="page-headers" />
      </LabView.Provider>,
    );
    expect(screen.queryByRole("button", { name: "重置示例" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看 Figma 设计来源 ↗" })).toBeVisible();
    expect(screen.getByText("Figma 原尺寸参考")).toBeVisible();
    expect(screen.getByText(/设计节点索引/)).toBeVisible();
  });
  it("shows breadcrumb interactions directly and navigates collapsed ancestors with the keyboard", async () => {
    render(<ApplicationFoundations kind="breadcrumbs" />);
    const user = userEvent.setup();
    expect(screen.queryByRole("region", { name: "Figma 原尺寸参考" })).not.toBeInTheDocument();
    const trigger = screen.getByRole("button", { name: "查看上级路径" });
    expect(trigger).toBeVisible();
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
