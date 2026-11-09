import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ButtonFoundations } from "./ButtonFoundations";

describe("ButtonFoundations", () => {
  it("keeps live code samples interactive while disabled and loading prevent actions", async () => {
    render(<ButtonFoundations />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "代码样本 standard Primary Default 普通" }));
    expect(screen.getByRole("status")).toHaveTextContent("代码样本已点击 1 次");
    const disabled = screen.getByRole("button", { name: "代码样本 standard Primary Disabled 普通" });
    const loading = screen.getByRole("button", { name: "代码样本 standard Primary Loading 普通" });
    expect(disabled).toBeDisabled();
    expect(loading).toBeDisabled();
    expect(loading).toHaveAttribute("aria-busy", "true");
    await user.click(disabled);
    expect(screen.getByRole("status")).toHaveTextContent("代码样本已点击 1 次");
  });

  it("marks unread styles and resets to a supported size when switching families", async () => {
    render(<ButtonFoundations />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: "lg" }));
    expect(within(screen.getByRole("article", { name: "Primary Hover" })).getByText(/样式待读取/)).toBeVisible();
    await user.click(screen.getByRole("radio", { name: "工具按钮" }));
    expect(screen.getByRole("radio", { name: "sm" })).toHaveAttribute("aria-checked", "true");
    await user.click(screen.getByRole("radio", { name: "xs" }));
    expect(screen.getAllByText("现有 Button 无 xs 尺寸，待映射").length).toBeGreaterThan(0);
    await user.click(screen.getByRole("radio", { name: "普通按钮" }));
    expect(screen.getByRole("radio", { name: "md" })).toHaveAttribute("aria-checked", "true");
    await user.click(screen.getByRole("radio", { name: "仅图标" }));
    expect(screen.queryByRole("region", { name: "Link color" })).not.toBeInTheDocument();
  });
});
