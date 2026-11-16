import { selectLabOption } from "../selectLabOption.test-helper";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ButtonFoundations } from "./ButtonFoundations";

describe("ButtonFoundations", () => {
  it("keeps live code samples interactive while disabled and loading prevent actions", async () => {
    render(<ButtonFoundations />);
    const user = userEvent.setup();
    const sample = screen.getByRole("button", { name: "代码样本 standard Primary Default 普通" });
    await user.click(sample);
    expect(sample).toHaveFocus();
    const disabled = screen.getByRole("button", { name: "代码样本 standard Primary Disabled 普通" });
    const loading = screen.getByRole("button", { name: "代码样本 standard Primary Loading 普通" });
    expect(disabled).toBeDisabled();
    expect(loading).toBeDisabled();
    expect(loading).toHaveAttribute("aria-busy", "true");
  });

  it("marks unread styles and resets to a supported size when switching families", async () => {
    render(<ButtonFoundations />);
    const user = userEvent.setup();
    await selectLabOption(user, "按钮尺寸", "lg");
    expect(within(screen.getByRole("article", { name: "Primary Hover" })).getByText(/样式待读取/)).toBeVisible();
    await selectLabOption(user, "按钮类别", "工具按钮");
    expect(screen.getByRole("combobox", { name: "按钮尺寸" })).toHaveTextContent("sm");
    await selectLabOption(user, "按钮尺寸", "xs");
    expect(screen.getAllByText("现有 Button 无 xs 尺寸，待映射").length).toBeGreaterThan(0);
    await selectLabOption(user, "按钮类别", "普通按钮");
    expect(screen.getByRole("combobox", { name: "按钮尺寸" })).toHaveTextContent("md");
    await selectLabOption(user, "按钮内容", "仅图标");
    expect(screen.queryByRole("region", { name: "Link color" })).not.toBeInTheDocument();
  });
});
