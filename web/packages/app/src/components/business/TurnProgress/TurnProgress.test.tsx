import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TurnProgress } from "./TurnProgress";
import type { TurnProgress as Progress } from "@/models/turnProgress";
import { createTranslator } from "@/shared/i18n";
const t = createTranslator("zh");
const progress: Progress = {
  id: "turn",
  revision: 1,
  status: "running",
  started_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  items: [
    { id: "intro", kind: "commentary", text: "正在检查文件" },
    {
      id: "read",
      kind: "tool",
      tool: {
        name: "Shell",
        actions: ["read"],
        status: "completed",
        summary: "README.md",
        command: "cat README.md",
        output: "contents",
      },
    },
    { id: "reasoning", kind: "reasoning", text: "Detailed reasoning" },
  ],
};
const renderText = (text: string) => <p>{text}</p>;
describe("TurnProgress", () => {
  it("shows intermediate text while running and collapses it on completion", () => {
    const { rerender } = render(<TurnProgress progress={progress} t={t} renderText={renderText} />);
    expect(screen.getByText("正在检查文件")).toBeTruthy();
    expect(document.querySelector('[data-preserve-scroll="true"]')).toBeNull();
    expect(document.querySelector("[data-scroll-anchor-toggle]")).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("README.md")).toBeVisible();
    expect(screen.getByText("工具进度 1/1")).toBeVisible();
    expect(screen.queryByText("cat README.md")).not.toBeInTheDocument();
    expect(screen.queryByText("contents")).toBeNull();
    expect(screen.getByText("Detailed reasoning")).toBeTruthy();
    rerender(
      <TurnProgress
        progress={{ ...progress, revision: 2, status: "succeeded", ended_at: new Date().toISOString() }}
        t={t}
        renderText={renderText}
        answer={<p>最终答案</p>}
      />,
    );
    expect(screen.queryByText("正在检查文件")).toBeNull();
    expect(screen.queryByText("Detailed reasoning")).toBeNull();
    expect(screen.getByText("最终答案")).toBeTruthy();
    expect(screen.getByText("1 个工具步骤")).toBeVisible();
    expect(screen.getByRole("button", { name: /^回复用时/ }).getAttribute("aria-expanded")).toBe("false");
  });
  it("groups consecutive reasoning chunks and allows the reasoning section to collapse independently", async () => {
    const items: Progress["items"] = [...progress.items, { id: "next", kind: "reasoning", text: "Next chunk" }];
    render(<TurnProgress progress={{ ...progress, items }} t={t} renderText={renderText} />);
    const reasoning = screen.getByRole("region", { name: "思考过程" });
    expect(within(reasoning).getByText("Detailed reasoning")).toBeVisible();
    expect(within(reasoning).getByText("Next chunk")).toBeVisible();
    const toggle = screen.getByRole("button", { name: "正在思考" });
    await userEvent.setup().click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(reasoning).not.toBeInTheDocument();
    expect(screen.getByText("正在检查文件")).toBeVisible();
  });

  it("follows streamed reasoning only while the reader stays near the bottom", () => {
    const frame = (text: string) => (
      <TurnProgress
        progress={{ ...progress, items: [{ id: "reasoning", kind: "reasoning", text }] }}
        t={t}
        renderText={renderText}
      />
    );
    const { rerender } = render(frame("First chunk"));
    const reasoning = screen.getByRole("region", { name: "思考过程" });
    let height = 500;
    Object.defineProperties(reasoning, { scrollHeight: { get: () => height }, clientHeight: { value: 150 } });
    rerender(frame("New chunk"));
    expect(reasoning.scrollTop).toBe(500);

    reasoning.scrollTop = 10;
    fireEvent.scroll(reasoning);
    height = 700;
    rerender(frame("Another chunk"));
    expect(reasoning.scrollTop).toBe(10);

    reasoning.scrollTop = 540;
    fireEvent.scroll(reasoning);
    height = 750;
    rerender(frame("Last chunk"));
    expect(reasoning.scrollTop).toBe(750);
  });

  it("collapses the whole process even if a command was expanded while running", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<TurnProgress progress={progress} t={t} renderText={renderText} />);
    await user.click(screen.getByRole("button", { name: /第 1 步：读取文件.*README.md/ }));
    expect(screen.getByText("contents")).toBeTruthy();
    rerender(
      <TurnProgress progress={{ ...progress, revision: 2, status: "succeeded" }} t={t} renderText={renderText} />,
    );
    expect(screen.queryByText("contents")).toBeNull();
    expect(screen.getByRole("button", { name: /回复用时/ })).toHaveAttribute("aria-expanded", "false");
    await user.click(screen.getByRole("button", { name: /回复用时/ }));
    expect(screen.getByRole("button", { name: /第 1 步/ })).toBeTruthy();
    expect(screen.queryByText("cat README.md")).not.toBeInTheDocument();
  });
  it("keeps a failure visible outside the collapsed process", () => {
    render(
      <TurnProgress progress={{ ...progress, status: "failed", error: "连接失败" }} t={t} renderText={renderText} />,
    );
    expect(screen.getByRole("status").textContent).toBe("连接失败");
  });
  it("does not mount an empty process for a completed text-only answer", () => {
    render(
      <TurnProgress
        progress={{ ...progress, status: "succeeded", items: [] }}
        t={t}
        renderText={renderText}
        answer={<p>回答</p>}
      />,
    );
    expect(screen.queryByRole("button")).toBeNull();
  });
  it.each(["/workspace/project", String.raw`C:\Users\Alice\My Project`])(
    "lists commands separately and preserves the selected command directory %s",
    async (cwd) => {
      const user = userEvent.setup();
      const items: Progress["items"] = [
        {
          id: "one",
          kind: "tool",
          tool: {
            name: "Run shell command",
            actions: ["execute"],
            status: "completed",
            command: "cat README.md",
            output: "first output",
            cwd,
            exit_code: 0,
            duration_ms: 100,
          },
        },
        {
          id: "two",
          kind: "tool",
          tool: {
            name: "Run shell command",
            actions: ["execute"],
            status: "completed",
            command: "rg missing",
            output: "second output",
            exit_code: 1,
          },
        },
      ];
      render(<TurnProgress progress={{ ...progress, items }} t={t} renderText={renderText} />);
      const list = screen.getByRole("list", { name: "工具调用" });
      expect(within(list).getAllByRole("listitem")).toHaveLength(2);
      expect(screen.getByText("工具进度 2/2")).toBeVisible();
      expect(screen.getByText("1 个未成功")).toBeVisible();
      expect(within(list).getByText("失败")).toBeTruthy();
      expect(screen.getByText("100 毫秒")).toBeVisible();
      expect(screen.queryByText("cat README.md")).not.toBeInTheDocument();
      expect(screen.queryByText("rg missing")).not.toBeInTheDocument();
      expect(screen.queryByText("first output")).toBeNull();
      expect(screen.queryByText(/工作目录/)).toBeNull();
      const first = within(list).getByRole("button", { name: /第 1 步：运行命令/ });
      first.focus();
      await user.keyboard("{Enter}");
      expect(first.getAttribute("aria-expanded")).toBe("true");
      expect(screen.getByText("cat README.md")).toBeVisible();
      expect(screen.getByText("first output")).toBeTruthy();
      expect(screen.queryByText("second output")).toBeNull();
      expect(screen.getByText(`工作目录：${cwd}`)).toBeTruthy();
      expect(screen.getByText("退出码：0")).toBeTruthy();
      await user.click(first);
      expect(screen.queryByText("first output")).toBeNull();
    },
  );
  it("preserves manual collapse during a run and allows reopening after completion", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <TurnProgress progress={progress} t={t} renderText={renderText} answer={<p>最终答案</p>} />,
    );
    await user.click(screen.getByRole("button", { name: /正在处理/ }));
    expect(screen.queryByText("正在检查文件")).toBeNull();
    expect(screen.getByText("最终答案")).toBeTruthy();
    rerender(
      <TurnProgress
        progress={{ ...progress, status: "succeeded" }}
        t={t}
        renderText={renderText}
        answer={<p>最终答案</p>}
      />,
    );
    expect(screen.queryByText("正在检查文件")).toBeNull();
    await user.click(screen.getByRole("button", { name: /回复用时/ }));
    expect(screen.getByText("正在检查文件")).toBeTruthy();
    expect(screen.getByText("Detailed reasoning")).toBeTruthy();
    expect(screen.getByRole("button", { name: /第 1 步/ })).toBeTruthy();
  });
  it("numbers all commands in the turn even when explanations split groups", () => {
    const first: Progress["items"][number] = {
      id: "a",
      kind: "tool",
      tool: { name: "Shell", actions: ["execute"], status: "completed", command: "pwd" },
    };
    const last: Progress["items"][number] = {
      id: "b",
      kind: "tool",
      tool: { name: "Shell", actions: ["read"], status: "completed", command: "cat README.md" },
    };
    const explanation: Progress["items"][number] = { id: "text", kind: "commentary", text: "Check files next." };
    const { rerender } = render(
      <TurnProgress progress={{ ...progress, items: [first, explanation, last] }} t={t} renderText={renderText} />,
    );
    expect(screen.getByRole("button", { name: /第 1 步：运行命令/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /第 2 步：读取文件/ })).toBeTruthy();
    // The last group's items keep their identity, so its memo must also track the number offset.
    const inserted: Progress["items"][number] = {
      id: "inserted",
      kind: "tool",
      tool: { name: "Shell", actions: ["execute"], status: "completed", command: "ls" },
    };
    rerender(
      <TurnProgress
        progress={{ ...progress, revision: 2, items: [first, inserted, explanation, last] }}
        t={t}
        renderText={renderText}
      />,
    );
    expect(screen.getByRole("button", { name: /第 2 步：运行命令/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /第 3 步：读取文件/ })).toBeTruthy();
    expect(screen.getAllByRole("list", { name: "工具调用" })[1].getAttribute("start")).toBe("3");
  });
  it("keeps running controls outside the disclosure and removes them at completion", async () => {
    const user = userEvent.setup();
    const controls = <button type="button">Stop this turn</button>;
    const { rerender } = render(
      <TurnProgress progress={progress} t={t} renderText={renderText} headerControls={controls} />,
    );
    await user.click(screen.getByRole("button", { name: /正在处理/ }));
    expect(screen.getByRole("button", { name: "Stop this turn" })).toBeVisible();
    rerender(
      <TurnProgress
        progress={{ ...progress, status: "succeeded" }}
        t={t}
        renderText={renderText}
        headerControls={controls}
      />,
    );
    expect(screen.queryByRole("button", { name: "Stop this turn" })).toBeNull();
  });
  it("places context controls beside the time without nesting interactive buttons", async () => {
    const user = userEvent.setup();
    render(
      <TurnProgress
        progress={progress}
        t={t}
        renderText={renderText}
        headerControls={<button type="button">Context usage</button>}
      />,
    );
    const context = screen.getByRole("button", { name: "Context usage" });
    expect(context.parentElement?.closest("button")).toBeNull();
    await user.click(context);
    expect(screen.getByText("正在检查文件")).toBeVisible();
    await user.click(screen.getByRole("button", { name: /正在处理/ }));
    expect(context).toBeVisible();
  });
  it("loads finished turns collapsed and preserves a manual reopen across updates", async () => {
    const user = userEvent.setup();
    const completed = { ...progress, status: "succeeded" };
    const { rerender } = render(<TurnProgress progress={completed} t={t} renderText={renderText} />);
    expect(screen.queryByText("正在检查文件")).toBeNull();
    await user.click(screen.getByRole("button", { name: /回复用时/ }));
    rerender(<TurnProgress progress={{ ...completed, revision: 2 }} t={t} renderText={renderText} />);
    expect(screen.getByText("正在检查文件")).toBeVisible();
  });
});

it("keeps the video clock running without restoring completed runtime stop controls", () => {
  vi.useFakeTimers();
  const origin = new Date("2026-09-30T02:44:00Z");
  vi.setSystemTime(new Date(origin.getTime() + 10000));
  const completed = {
    ...progress,
    status: "succeeded",
    started_at: origin.toISOString(),
    ended_at: new Date(origin.getTime() + 9000).toISOString(),
  };
  const video = {
    content: "",
    metadata: {
      video_generation: { state: "generating", started_at: new Date(origin.getTime() + 6000).toISOString() },
    },
  };
  try {
    const view = render(
      <TurnProgress
        progress={completed}
        videoMessages={[video]}
        t={t}
        renderText={renderText}
        controls={<button>停止</button>}
      />,
    );
    expect(screen.getByRole("button", { name: /正在生成视频.*10s/ })).toBeTruthy();
    act(() => vi.advanceTimersByTime(60000));
    expect(screen.getByRole("button", { name: /正在生成视频.*1m 10s/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "停止" })).toBeNull();
    view.rerender(
      <TurnProgress
        progress={completed}
        videoMessages={[
          {
            ...video,
            metadata: {
              video_generation: {
                ...video.metadata.video_generation,
                state: "completed",
                ended_at: new Date(origin.getTime() + 70000).toISOString(),
              },
            },
          },
        ]}
        t={t}
        renderText={renderText}
      />,
    );
    act(() => vi.advanceTimersByTime(60000));
    expect(screen.getByRole("button", { name: /总用时.*1m 10s/ })).toBeTruthy();
    view.unmount();
  } finally {
    vi.useRealTimers();
  }
});
