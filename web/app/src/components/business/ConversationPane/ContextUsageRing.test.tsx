import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ContextUsageRing } from "./ContextUsageRing";
import { messages } from "@/shared/i18n/messages";
import type { TranslateFn } from "@/models/conversations";
import type { ContextUsage } from "@/models/modelMetadata";
const t: TranslateFn = (key, params) => {
  let text = String(messages.zh[key as keyof typeof messages.zh] || key);
  for (const [name, value] of Object.entries(params || {})) text = text.replace(`{${name}}`, String(value));
  return text;
};
const usage: ContextUsage = {
  session_id: "s",
  model_id: "m",
  used_tokens: 20316,
  context_window: 32768,
  context_source: "default",
  auto_compact: true,
  compact_threshold: 24576,
  compacting: false,
  estimated: false,
  updated_at: "2026-09-21T08:00:00Z",
};
describe("ContextUsageRing", () => {
  it("shows the current ratio and details on hover", async () => {
    render(<ContextUsageRing usage={usage} t={t} />);
    await userEvent.hover(screen.getByRole("button", { name: "上下文已使用 62%" }));
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("20.32 K tokens");
    expect(tooltip).toHaveTextContent("12.45 K tokens");
    expect(tooltip).toHaveTextContent("默认值");
    expect(tooltip).toHaveTextContent("最近一次运行时上报");
  });
  it("distinguishes unknown from zero", () => {
    const view = render(<ContextUsageRing t={t} />);
    expect(screen.getByRole("button")).toHaveAccessibleName("尚未获得上下文用量");
    expect(view.container.querySelector(".context-usage-value")).toBeNull();
    view.rerender(<ContextUsageRing usage={{ ...usage, used_tokens: 0 }} t={t} />);
    expect(screen.getByRole("button")).toHaveAccessibleName("上下文已使用 0%");
  });
  it("retains overflow details and supports keyboard focus", async () => {
    render(<ContextUsageRing usage={{ ...usage, used_tokens: 65536, compacting: true }} t={t} />);
    await userEvent.tab();
    expect(screen.getByRole("button")).toHaveFocus();
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("200%");
    expect(tooltip).toHaveTextContent("剩余：0 K tokens");
    expect(tooltip).toHaveTextContent("正在整理对话");
  });
});
