import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ScheduledTaskStart } from "@/pages/TasksPage/components/ScheduledTaskStart";
import { createTranslator } from "@/shared/i18n";
const agents = [
  { id: "agent-manager", name: "Manager", role: "manager" },
  { id: "agent-linda", name: "测试工程师 Linda", role: "worker" },
];
const t = createTranslator("zh");

it("selects an executor after Chinese text and keeps Enter as a newline", async () => {
  const user = userEvent.setup();
  const send = vi.fn().mockResolvedValue(true);
  render(<ScheduledTaskStart agents={agents} t={t} focusToken={0} onSend={send} />);
  const editor = screen.getByRole("textbox", { name: "定时任务描述" });
  await user.type(editor, "汇总一下@Lin");
  await user.click(screen.getByRole("option", { name: "测试工程师 Linda" }));
  expect(editor).toHaveValue("汇总一下@测试工程师 Linda ");
  await user.type(editor, "每周一上午10点{Enter}输出报告");
  expect(editor).toHaveValue("汇总一下@测试工程师 Linda 每周一上午10点\n输出报告");
  expect(send).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "发给 Manager" }));
  expect(send).toHaveBeenCalledWith("汇总一下@测试工程师 Linda 每周一上午10点\n输出报告");
  await waitFor(() => expect(editor).toHaveValue(""));
});
it("retains the sentence after failure and focuses the existing draft", async () => {
  const user = userEvent.setup();
  const send = vi.fn().mockResolvedValue(false);
  const view = render(<ScheduledTaskStart agents={agents} t={t} focusToken={0} onSend={send} />);
  const editor = screen.getByRole("textbox", { name: "定时任务描述" });
  await user.type(editor, "明天上午10点让Manager输出报告");
  await user.click(screen.getByRole("button", { name: "发给 Manager" }));
  await waitFor(() => expect(send).toHaveBeenCalled());
  view.rerender(<ScheduledTaskStart agents={agents} t={t} focusToken={1} onSend={send} error="连接失败" />);
  expect(screen.getByRole("textbox")).toHaveValue("明天上午10点让Manager输出报告");
  expect(screen.getByRole("textbox")).toHaveFocus();
  expect(screen.getByRole("alert")).toHaveTextContent("连接失败");
});
it("does not select mentions while composing with an IME", () => {
  render(<ScheduledTaskStart agents={agents} t={t} focusToken={0} />);
  const editor = screen.getByRole("textbox");
  fireEvent.change(editor, { target: { value: "@", selectionStart: 1 } });
  fireEvent.keyDown(editor, { key: "Enter", isComposing: true });
  expect(editor).toHaveValue("@");
});
