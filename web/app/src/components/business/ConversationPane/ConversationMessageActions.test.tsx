import { TooltipProvider } from "@/components/ui";
import type { ReactNode } from "react";
import { fireEvent, render as baseRender, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { ConversationMessageActions } from "./ConversationMessageActions";

function render(ui: ReactNode) {
  return baseRender(ui, { wrapper: ({ children }) => <TooltipProvider delayDuration={0}>{children}</TooltipProvider> });
}

describe("message consumption footer", () => {
  const t = createTranslator("zh");
  const metadata = (model: string, known = true, partial = false) => ({
    csgclaw: {
      turn_progress: {
        status: "succeeded",
        usage: { known, model_id: model, input_tokens: 1000, output_tokens: 220, total_tokens: 1220, partial },
      },
    },
  });
  it("shows consumption and the selected model, then hides the name for auto", () => {
    const { rerender } = render(<ConversationMessageActions metadata={metadata("gpt-test")} content="Answer" t={t} />);
    expect(screen.getByRole("button", { name: "本轮消耗 1.22 K tokens · 模型 gpt-test" })).toHaveAttribute(
      "title",
      expect.stringContaining("输入：1000"),
    );
    rerender(<ConversationMessageActions metadata={metadata("auto")} content="Answer" t={t} />);
    expect(screen.getByRole("button", { name: "本轮消耗 1.22 K tokens" })).toBeVisible();
    expect(screen.queryByText(/模型/)).toBeNull();
    expect(screen.queryByText(/auto/i)).toBeNull();
  });
  it("distinguishes unavailable usage and partial accounting", () => {
    const { rerender } = render(<ConversationMessageActions metadata={metadata("auto", false)} t={t} />);
    expect(screen.getByRole("button", { name: "本轮消耗暂不可用" })).toBeVisible();
    rerender(<ConversationMessageActions metadata={metadata("auto", true, true)} t={t} />);
    expect(screen.getByRole("button", { name: "本轮消耗 1.22 K tokens （部分统计）" })).toBeVisible();
  });
  it("collapses long usage to an info icon when space is limited", async () => {
    const rect = vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 300, 22));
    try {
      render(<ConversationMessageActions metadata={metadata("long-model-name")} t={t} />);
      const trigger = screen.getByRole("button", { name: "本轮消耗 1.22 K tokens · 模型 long-model-name" });
      expect(trigger.querySelector("svg")).not.toBeNull();
      fireEvent.focus(trigger);
      expect(await screen.findByRole("tooltip")).toHaveTextContent("输入：1000");
    } finally {
      rect.mockRestore();
    }
  });
  it("keeps exact counters available through keyboard focus", async () => {
    render(<ConversationMessageActions metadata={metadata("auto")} t={t} />);
    fireEvent.focus(screen.getByRole("button", { name: "本轮消耗 1.22 K tokens" }));
    expect(await screen.findByRole("tooltip")).toHaveTextContent("输入：1000 tokens；输出：220 tokens");
  });
  it("does not invent usage for historical or running replies", () => {
    const { rerender } = render(<ConversationMessageActions metadata={{}} t={t} />);
    expect(screen.queryByText(/本轮消耗/)).toBeNull();
    const running = metadata("auto");
    running.csgclaw.turn_progress.status = "running";
    rerender(<ConversationMessageActions metadata={running} t={t} />);
    expect(screen.queryByText(/本轮消耗/)).toBeNull();
  });
});
