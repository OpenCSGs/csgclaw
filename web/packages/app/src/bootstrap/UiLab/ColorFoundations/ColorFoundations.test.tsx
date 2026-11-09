import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ColorFoundations } from "./ColorFoundations";

vi.mock("@/shared/styles/tokens.css?raw", async () => {
  const { readFileSync } = await import("node:fs");
  return { default: readFileSync("src/shared/styles/tokens.css", "utf8") };
});

describe("ColorFoundations", () => {
  it("finds a Figma color by HEX and shows its code mapping", async () => {
    render(<ColorFoundations />);
    await userEvent.setup().type(screen.getByRole("searchbox"), "#3250bd");
    const primitives = screen.getByRole("heading", { name: "基础色板 · Primitives" }).closest("section")!;
    expect(within(primitives).getByText("一致：--brand-600")).toBeVisible();
    expect(within(primitives).queryByText("#F0F3FF")).not.toBeInTheDocument();
  });
  it("keeps a visible empty state when no color matches", async () => {
    render(<ColorFoundations />);
    await userEvent.setup().type(screen.getByRole("searchbox"), "not-a-color");
    expect(screen.getAllByRole("status")).toHaveLength(3);
  });
});
