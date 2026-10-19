import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { ConversationMessageActions } from "./ConversationMessageActions";

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
    expect(screen.getByText("本轮消耗 1.22 K tokens · 模型 gpt-test")).toHaveAttribute(
      "title",
      expect.stringContaining("输入：1000"),
    );
    rerender(<ConversationMessageActions metadata={metadata("auto")} content="Answer" t={t} />);
    expect(screen.getByText("本轮消耗 1.22 K tokens")).toBeVisible();
    expect(screen.queryByText(/模型/)).toBeNull();
    expect(screen.queryByText(/auto/i)).toBeNull();
  });
  it("distinguishes unavailable usage and partial accounting", () => {
    const { rerender } = render(<ConversationMessageActions metadata={metadata("auto", false)} t={t} />);
    expect(screen.getByText("本轮消耗暂不可用")).toBeVisible();
    rerender(<ConversationMessageActions metadata={metadata("auto", true, true)} t={t} />);
    expect(screen.getByText("本轮消耗 1.22 K tokens （部分统计）")).toBeVisible();
  });
});
