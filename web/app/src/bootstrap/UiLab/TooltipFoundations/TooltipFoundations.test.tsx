import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui/Tooltip";
import { TooltipFoundations } from "./TooltipFoundations";

describe("TooltipFoundations", () => {
  it("opens from pointer hover, associates its text with the trigger and dismisses on Escape", async () => {
    render(
      <TooltipProvider delayDuration={0}>
        <TooltipFoundations />
      </TooltipProvider>,
    );
    const user = userEvent.setup();
    const trigger = screen.getByRole("button", { name: "悬浮或聚焦查看提示" });
    await user.hover(trigger);
    expect(await screen.findByRole("tooltip")).toHaveTextContent("This is a tooltip");
    expect(trigger).toHaveAccessibleDescription("This is a tooltip");
    const demo = within(screen.getByRole("region", { name: "气泡提示交互试用" }));
    expect(demo.queryByRole("tooltip")).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("tooltip")).not.toBeInTheDocument());
  });

  it("previews edited supporting text on keyboard focus and keeps reference variants separate", async () => {
    render(
      <TooltipProvider delayDuration={0}>
        <TooltipFoundations />
      </TooltipProvider>,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: "带说明" }));
    const title = screen.getByRole("textbox", { name: "提示标题" });
    await user.clear(title);
    await user.type(title, "权限说明");
    const description = screen.getByRole("textbox", { name: "提示说明" });
    await user.clear(description);
    await user.type(description, "只有管理员可以修改设置。");
    await user.click(screen.getByRole("radio", { name: "右侧" }));
    screen.getByRole("button", { name: "悬浮或聚焦查看提示" }).focus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("权限说明 只有管理员可以修改设置。");
    expect(screen.getAllByRole("link", { name: "查看设计节点 ↗" })).toHaveLength(7);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("tooltip")).not.toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: "重置文案" }));
    expect(title).toHaveValue("This is a tooltip");
  });
});
