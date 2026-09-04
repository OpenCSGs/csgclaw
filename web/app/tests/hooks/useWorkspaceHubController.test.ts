import { createElement, type ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import { deleteSkillRequest } from "@/api/skills";
import {
  nextWorkspaceSkillAfterDelete,
  useWorkspaceHubController,
  visibleWorkspaceHubTemplates,
  workspaceHubMutationError,
} from "@/hooks/workspace/useWorkspaceHubController";
import { useWorkspaceHubSelection } from "@/hooks/workspace/useWorkspaceHubSelection";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { SkillSummary } from "@/models/skillhub";

vi.mock("@/api/skills", async () => {
  const actual = await vi.importActual<typeof import("@/api/skills")>("@/api/skills");
  return {
    ...actual,
    deleteSkillRequest: vi.fn(),
  };
});

vi.mock("@/hooks/workspace/useWorkspaceHubSelection", () => ({
  useWorkspaceHubSelection: vi.fn(),
}));

describe("workspace hub controller errors", () => {
  it("only exposes mutation errors on their owning resource page", () => {
    expect(workspaceHubMutationError("template", "", "Publishing failed", "")).toBe("Publishing failed");
    expect(workspaceHubMutationError("skill", "", "Publishing failed", "Skill delete failed")).toBe(
      "Skill delete failed",
    );
    expect(workspaceHubMutationError("mcp", "Delete failed", "Publishing failed", "Skill delete failed")).toBe("");
  });

  it("hides a deleting template before the refreshed list arrives", () => {
    const templates: HubTemplate[] = [
      { id: "local.alpha", role: "worker", source: { kind: "local" } },
      { id: "local.beta", role: "worker", source: { kind: "local" } },
    ];

    expect(visibleWorkspaceHubTemplates(templates, "local.alpha").map((item) => item.id)).toEqual(["local.beta"]);
  });
});

describe("nextWorkspaceSkillAfterDelete", () => {
  const skills: SkillSummary[] = [
    { name: "alpha", description: "Alpha" },
    { name: "beta", description: "Beta" },
    { name: "gamma", description: "Gamma" },
  ];

  it("selects the following skill when one is available", () => {
    expect(nextWorkspaceSkillAfterDelete(skills, "beta")?.name).toBe("gamma");
  });

  it("selects the previous skill after deleting the last item", () => {
    expect(nextWorkspaceSkillAfterDelete(skills, "gamma")?.name).toBe("beta");
  });

  it("returns no destination after deleting the only skill", () => {
    expect(nextWorkspaceSkillAfterDelete(skills.slice(0, 1), "alpha")).toBeNull();
  });
});

describe("useWorkspaceHubController skill deletion", () => {
  it("selects and reports the adjacent skill immediately after deletion", async () => {
    const skills: SkillSummary[] = [
      { name: "alpha", description: "Alpha" },
      { name: "weather-assistant", description: "Weather" },
    ];
    const setSelectedHubSkillName = vi.fn();
    const setSelectedHubSkillPath = vi.fn();
    const onSkillDeleted = vi.fn();
    vi.mocked(deleteSkillRequest).mockResolvedValue(undefined);
    vi.mocked(useWorkspaceHubSelection).mockReturnValue({
      detailPaneProps: { error: "" },
      error: "",
      selectedHubResourceType: "skill",
      setSelectedHubResourceType: vi.fn(),
      setSelectedHubSkillName,
      setSelectedHubSkillPath,
      setSelectedHubTemplateId: vi.fn(),
      skills,
    } as unknown as ReturnType<typeof useWorkspaceHubSelection>);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const { result } = renderHook(
      () =>
        useWorkspaceHubController({
          activePane: { type: "hub", id: "weather-assistant", resourceType: "skill" },
          hubLoaded: true,
          hubTemplates: [],
          hubTemplatesQuery: {} as UseQueryResult<HubTemplate[]>,
          onSkillDeleted,
          refreshWorkspaceHubTemplates: vi.fn(async () => []),
          t: (key) => key,
        }),
      { wrapper },
    );

    await act(async () => {
      await expect(result.current.hub.deleteSkill(skills[1])).resolves.toBe(true);
    });

    expect(setSelectedHubSkillName).toHaveBeenCalledWith("alpha");
    expect(setSelectedHubSkillPath).toHaveBeenCalledWith("alpha/SKILL.md");
    expect(onSkillDeleted).toHaveBeenCalledWith(skills[0]);
  });
});
