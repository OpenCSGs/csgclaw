import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchSkillFile, fetchSkillTree } from "@/api/skills";
import type { WorkspaceFile } from "@/models/workspace";
import type { SkillSummary } from "@/models/skillhub";
import { SkillDetailFiles } from "./SkillDetailFiles";
vi.mock("@/api/skills", () => ({ fetchSkillFile: vi.fn(), fetchSkillTree: vi.fn() }));
vi.mock("@/components/business/WorkspaceFileTree", () => ({
  WorkspaceFileTree: () => null,
  WorkspaceFilePreview: ({ file }: { file?: WorkspaceFile | null }) => <div>{file?.content}</div>,
}));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchSkillTree).mockResolvedValue({ entries: [] });
});
describe("SkillDetailFiles source changes", () => {
  it("loads the newly selected source instead of reusing cached files from an older source", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false, gcTime: 0 } } });
    const skill: SkillSummary = {
      name: "demo",
      discovery: { tool: "Codex", path: "/first/demo", key: "first", updated_at: "2026-10-30", sources: [] },
    };
    vi.mocked(fetchSkillFile)
      .mockResolvedValueOnce({ path: "demo/SKILL.md", content: "first source" })
      .mockResolvedValueOnce({ path: "demo/SKILL.md", content: "new source" });
    const view = render(
      <QueryClientProvider client={client}>
        <SkillDetailFiles skill={skill} t={(key) => key} />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("first source")).toBeVisible();
    view.rerender(
      <QueryClientProvider client={client}>
        <SkillDetailFiles
          skill={{ ...skill, discovery: { ...skill.discovery!, key: "second", path: "/second/demo" } }}
          t={(key) => key}
        />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("new source")).toBeVisible();
    expect(fetchSkillFile).toHaveBeenCalledTimes(2);
    client.clear();
  });
});
