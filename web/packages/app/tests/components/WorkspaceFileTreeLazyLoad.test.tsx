import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WorkspaceFileTree } from "@/components/business/WorkspaceFileTree";

describe("WorkspaceFileTree lazy loading", () => {
  it("loads a directory when it is expanded and keeps it open when children arrive", async () => {
    const user = userEvent.setup();
    const onToggleDir = vi.fn();
    const { rerender } = render(
      <WorkspaceFileTree
        emptyText="empty"
        entries={[{ path: "skills", name: "skills", type: "dir" }]}
        loadingText="loading"
        onToggleDir={onToggleDir}
      />,
    );

    await user.click(screen.getByRole("button", { name: /skills/i }));
    expect(onToggleDir).toHaveBeenCalledWith("skills");

    rerender(
      <WorkspaceFileTree
        emptyText="empty"
        entries={[
          { path: "skills", name: "skills", type: "dir" },
          { path: "skills/SKILL.md", name: "SKILL.md", type: "file" },
        ]}
        loadingText="loading"
        onToggleDir={onToggleDir}
      />,
    );

    expect(screen.getByRole("button", { name: /SKILL\.md/i })).toBeVisible();
    expect(screen.getByRole("button", { name: /skills/i })).toHaveAttribute("aria-expanded", "true");
  });
});
