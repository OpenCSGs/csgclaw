import { createElement, type ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import { deleteHubTemplateRequest, publishHubTemplateToCommunityRequest } from "@/api/hub";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { deleteSkillRequest, installRemoteSkillRequest } from "@/api/skills";
import {
  nextWorkspaceSkillAfterDelete,
  useWorkspaceHubController,
  visibleWorkspaceHubTemplates,
  workspaceHubMutationError,
} from "@/hooks/workspace/useWorkspaceHubController";
import { useWorkspaceHubSelection } from "@/hooks/workspace/useWorkspaceHubSelection";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { SkillSummary } from "@/models/skillhub";
import { createTranslator } from "@/shared/i18n";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/hub", async () => ({
  ...(await vi.importActual<typeof import("@/api/hub")>("@/api/hub")),
  deleteHubTemplateRequest: vi.fn(),
  publishHubTemplateToCommunityRequest: vi.fn(),
}));

vi.mock("@/api/skills", async () => {
  const actual = await vi.importActual<typeof import("@/api/skills")>("@/api/skills");
  return {
    ...actual,
    deleteSkillRequest: vi.fn(),
    installRemoteSkillRequest: vi.fn(),
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
          openCSGAuthGuard: openCSGAuthGuardStub(),
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

  it("opens login instead of installing a remote OpenCSG skill while unauthenticated", async () => {
    const openCSGAuthGuard = openCSGAuthGuardStub(false);
    openCSGAuthGuard.requireAuthentication = vi.fn(() => false);
    vi.mocked(useWorkspaceHubSelection).mockReturnValue({
      detailPaneProps: { error: "" },
      error: "",
      selectedHubResourceType: "skill",
      setSelectedHubResourceType: vi.fn(),
      setSelectedHubSkillName: vi.fn(),
      setSelectedHubSkillPath: vi.fn(),
      setSelectedHubTemplateId: vi.fn(),
      skills: [],
    } as unknown as ReturnType<typeof useWorkspaceHubSelection>);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const { result } = renderHook(
      () =>
        useWorkspaceHubController({
          activePane: { type: "hub", id: "remote/weather", resourceType: "skill" },
          hubLoaded: true,
          hubTemplates: [],
          hubTemplatesQuery: {} as UseQueryResult<HubTemplate[]>,
          openCSGAuthGuard,
          refreshWorkspaceHubTemplates: vi.fn(async () => []),
          t: (key) => key,
        }),
      { wrapper },
    );

    await act(async () => {
      await expect(
        result.current.hub.installRemoteSkill({ name: "weather", remotePath: "remote/weather" }),
      ).resolves.toBeNull();
    });

    expect(openCSGAuthGuard.requireAuthentication).toHaveBeenCalledOnce();
    expect(installRemoteSkillRequest).not.toHaveBeenCalled();
  });
});

describe("useWorkspaceHubController template deletion", () => {
  it.each([false, true])(
    "leaves detail before deletion and removes cached item (refresh failure: %s)",
    async (refreshFails) => {
      const template: HubTemplate = { id: "local.codex-211", source: { kind: "local" } };
      const remaining: HubTemplate = { id: "local.other", source: { kind: "local" } };
      const setSelectedHubTemplateId = vi.fn();
      const onTemplateDeleteStarted = vi.fn();
      const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
      const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
      queryClient.setQueryData(workspaceQueryKeys.hubTemplates(), [template, remaining]);
      queryClient.setQueryData(workspaceQueryKeys.hubTemplate(template.id), template);
      vi.mocked(useWorkspaceHubSelection).mockReturnValue({
        detailPaneProps: { error: "" },
        error: "",
        selectedHubResourceType: "template",
        setSelectedHubTemplateId,
        skills: [],
      } as unknown as ReturnType<typeof useWorkspaceHubSelection>);
      vi.mocked(deleteHubTemplateRequest).mockImplementation(async () => {
        expect(setSelectedHubTemplateId).toHaveBeenCalledWith("");
        expect(onTemplateDeleteStarted).toHaveBeenCalledWith(template.id);
      });
      const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children);
      const { result } = renderHook(
        () =>
          useWorkspaceHubController({
            activePane: { type: "hub", resourceType: "template", id: "local.codex-211" },
            hubLoaded: true,
            hubTemplates: [template, remaining],
            hubTemplatesQuery: {} as UseQueryResult<HubTemplate[]>,
            onTemplateDeleteStarted,
            openCSGAuthGuard: openCSGAuthGuardStub(),
            refreshWorkspaceHubTemplates: async () => {
              if (refreshFails) throw new Error("offline");
              return [remaining];
            },
            t: createTranslator("zh"),
          }),
        { wrapper },
      );
      await act(async () => {
        await expect(result.current.hub.deleteHubTemplate(template)).resolves.toBe(true);
      });
      expect(confirm).toHaveBeenCalledWith(
        `删除模板 ${template.id}?\n\n只删除本地模板，AgenticHub 上的模板不会删除。如需彻底删除，请前往 AgenticHub 下架。`,
      );
      expect(queryClient.getQueryData(workspaceQueryKeys.hubTemplates())).toEqual([remaining]);
      expect(queryClient.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toBeUndefined();
      confirm.mockRestore();
      queryClient.clear();
    },
  );
});

it("clears publishing errors when selecting a different template", async () => {
  const queryClient = new QueryClient();
  const selection = {
    detailPaneProps: { error: "" },
    error: "",
    selectedHubResourceType: "template",
    selectedHubTemplateId: "local.first",
    skills: [],
  } as unknown as ReturnType<typeof useWorkspaceHubSelection>;
  vi.mocked(useWorkspaceHubSelection).mockReturnValue(selection);
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  const { result, rerender } = renderHook(
    () =>
      useWorkspaceHubController({
        activePane: { type: "hub", resourceType: "template", id: "local.first" },
        hubLoaded: true,
        hubTemplates: [],
        hubTemplatesQuery: {} as UseQueryResult<HubTemplate[]>,
        openCSGAuthGuard: openCSGAuthGuardStub(),
        refreshWorkspaceHubTemplates: async () => [],
        t: (key) => key,
      }),
    { wrapper },
  );
  act(() => result.current.hub.setPublishError("Previous template failed"));
  expect(result.current.hub.detailPaneProps.publishError).toBe("Previous template failed");
  vi.mocked(useWorkspaceHubSelection).mockReturnValue({ ...selection, selectedHubTemplateId: "local.second" });
  rerender();
  expect(result.current.hub.detailPaneProps.publishError).toBe("");
  expect(result.current.hub.error).toBe("");
  act(() => result.current.hub.setPublishError("New attempt failed"));
  act(() => result.current.hub.detailPaneProps.onResetPublishError());
  expect(result.current.hub.detailPaneProps.publishError).toBe("");
  queryClient.clear();
});

it.each(["switch", "reopen"] as const)("ignores a late publishing failure after %s", async (action) => {
  const queryClient = new QueryClient();
  const selection = {
    detailPaneProps: { error: "" },
    error: "",
    selectedHubResourceType: "template",
    selectedHubTemplateId: "local.first",
    skills: [],
  } as unknown as ReturnType<typeof useWorkspaceHubSelection>;
  vi.mocked(useWorkspaceHubSelection).mockReturnValue(selection);
  let rejectPublish!: (error: Error) => void;
  vi.mocked(publishHubTemplateToCommunityRequest).mockReturnValue(
    new Promise((_resolve, reject) => {
      rejectPublish = reject;
    }),
  );
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  const { result, rerender } = renderHook(
    () =>
      useWorkspaceHubController({
        activePane: { type: "hub", resourceType: "template", id: "local.first" },
        hubLoaded: true,
        hubTemplates: [],
        hubTemplatesQuery: {} as UseQueryResult<HubTemplate[]>,
        openCSGAuthGuard: openCSGAuthGuardStub(),
        refreshWorkspaceHubTemplates: async () => [],
        t: (key) => key,
      }),
    { wrapper },
  );
  let request!: ReturnType<typeof result.current.hub.publishHubTemplate>;
  act(() => {
    request = result.current.hub.publishHubTemplate({ id: "local.first", source: { kind: "local" } });
  });
  if (action === "switch") {
    vi.mocked(useWorkspaceHubSelection).mockReturnValue({ ...selection, selectedHubTemplateId: "local.second" });
    rerender();
  } else {
    act(() => result.current.hub.detailPaneProps.onResetPublishError());
  }
  await act(async () => {
    rejectPublish(new Error("Old request failed"));
    await request;
  });
  expect(result.current.hub.detailPaneProps.publishError).toBe("");
  expect(result.current.hub.error).toBe("");
  queryClient.clear();
});

it.each(
  (["agent", "template"] as const).flatMap((entry) =>
    (["Pending", "Fail", "missing", "refresh-failed"] as const).map((status) => ({ entry, status })),
  ),
)("preserves refreshed details and includes published templates from $entry ($status)", async ({ entry, status }) => {
  const queryClient = new QueryClient();
  const local: HubTemplate = { id: "local.first", name: "first", source: { kind: "local" } };
  const published: HubTemplate = { id: "alice/first", name: "first", source: { kind: "remote" } };
  const refreshed: HubTemplate = {
    ...published,
    description: "Full refreshed details",
    metadata: { sensitive_check: { status, failure_details: [{ path: "MEMORY.md" }] } },
  };
  vi.mocked(useWorkspaceHubSelection).mockReturnValue({
    detailPaneProps: { error: "" },
    error: "",
    selectedHubResourceType: "template",
    selectedHubTemplateId: local.id,
    skills: [],
  } as unknown as ReturnType<typeof useWorkspaceHubSelection>);
  vi.mocked(publishHubTemplateToCommunityRequest).mockResolvedValue(published);
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  const { result } = renderHook(
    () =>
      useWorkspaceHubController({
        activePane: { type: "hub", resourceType: "template", id: "local.first" },
        hubLoaded: true,
        hubTemplates: [local],
        hubTemplatesQuery: {} as UseQueryResult<HubTemplate[]>,
        openCSGAuthGuard: openCSGAuthGuardStub(),
        refreshWorkspaceHubTemplates: async () => {
          if (status === "refresh-failed") throw new Error("Listing unavailable");
          const items = status === "missing" ? [local] : [local, refreshed];
          queryClient.setQueryData(workspaceQueryKeys.hubTemplates(), items);
          return items;
        },
        t: (key) => key,
      }),
    { wrapper },
  );
  queryClient.setQueryData(workspaceQueryKeys.hubTemplates(), [local]);
  await act(async () => {
    if (entry === "agent") {
      await result.current.refreshHubTemplates(published);
    } else {
      expect(await result.current.hub.publishHubTemplate(local)).toEqual({ status: "success", template: published });
    }
  });
  expect(queryClient.getQueryData(workspaceQueryKeys.hubTemplates())).toEqual([
    local,
    status === "missing" || status === "refresh-failed" ? published : refreshed,
  ]);
  queryClient.clear();
});
