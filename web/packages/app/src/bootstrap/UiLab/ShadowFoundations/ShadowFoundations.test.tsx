import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ShadowFoundations } from "./ShadowFoundations";

vi.mock("@/shared/styles/tokens.css?raw", async () => {
  const { readFileSync } = await import("node:fs");
  return { default: readFileSync("src/shared/styles/tokens.css", "utf8") };
});

describe("ShadowFoundations", () => {
  it("switches to real tokens and explicitly marks absent levels", async () => {
    render(<ShadowFoundations />);
    const user = userEvent.setup();
    const xs = screen.getByRole("article", { name: "shadow-xs" });
    const xl = screen.getByRole("article", { name: "shadow-xl" });
    expect(xs.querySelector("[style]")?.getAttribute("style")).toContain("#0a0d120d");
    await user.click(screen.getByRole("radio", { name: "现有代码" }));
    expect(xs.querySelector("[style]")?.getAttribute("style")).toContain("var(--shadow-xs)");
    expect(within(xl).getByText("未定义同名 token")).toBeVisible();
    await user.click(screen.getByRole("radio", { name: "Figma 参考" }));
    expect(within(xl).queryByText("未定义同名 token")).not.toBeInTheDocument();
  });
});
