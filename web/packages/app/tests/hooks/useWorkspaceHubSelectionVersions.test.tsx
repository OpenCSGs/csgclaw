import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchHubTemplate, fetchHubWorkspace, fetchHubWorkspaceFile, updateHubWorkspaceFile } from "@/api/hub";
import { useWorkspaceHubSelection } from "@/hooks/workspace/useWorkspaceHubSelection";
import { useWorkspaceUiStore } from "@/hooks/workspace/workspaceUiStore";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { paneFromLocation } from "@/models/routing";
import type { HubTemplate, HubWorkspaceListing } from "@/models/hubWorkspace";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/hub", async () => ({
  ...(await vi.importActual<typeof import("@/api/hub")>("@/api/hub")),
  fetchHubTemplate: vi.fn(),
  fetchHubWorkspace: vi.fn(),
  fetchHubWorkspaceFile: vi.fn(),
  updateHubWorkspaceFile: vi.fn(),
}));
const t = (key: string) => key;
const auth = openCSGAuthGuardStub(false);
const template: HubTemplate & { id: string } = {
  id: "local.Reviewer",
  name: "Reviewer",
  uid: "identity",
  revision: 2,
  source: { kind: "local" },
};

function setup() {
  useWorkspaceUiStore.setState({
    selectedHubResourceType: "template",
    selectedHubTemplateId: template.id,
    selectedHubWorkspacePath: "",
    selectedHubSkillName: "",
    selectedHubSkillPath: "",
    selectedMCPServerName: "",
    selectedKnowledgeBaseID: "",
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(workspaceQueryKeys.hubTemplate(template.id), template);
  client.setQueryData(workspaceQueryKeys.hubTemplates(), [template]);
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(
    () =>
      useWorkspaceHubSelection({
        activePane: paneFromLocation("/templates/local.Reviewer"),
        templates: [template],
        loaded: true,
        openCSGAuthGuard: auth,
        t,
      }),
    { wrapper },
  );
  return { ...hook, client };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

it("reloads instructions, skill files and connector content when the active revision changes", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  let revision = 2;
  vi.mocked(fetchHubTemplate).mockImplementation(async () => ({ ...template, revision }));
  vi.mocked(fetchHubWorkspace).mockImplementation(async (_id, path = "") => ({
    kind: "dir",
    path,
    entries:
      path === ""
        ? ["instructions", "skills", "mcps"].map((name) => ({ name, path: name, type: "dir" as const }))
        : path === "skills"
          ? [{ name: `skill-v${revision}`, path: `skills/skill-v${revision}`, type: "dir" as const }]
          : path.startsWith("skills/")
            ? [{ name: "SKILL.md", path: `${path}/SKILL.md`, type: "file" as const }]
            : path === "mcps"
              ? [{ name: "mcp.json", path: "mcps/mcp.json", type: "file" as const }]
              : [],
  }));
  vi.mocked(fetchHubWorkspaceFile).mockImplementation(async (_id, path) => ({
    path,
    content: `V${revision}: ${path}`,
    size: 10,
  }));
  const { result, client, unmount } = setup();
  const pane = () => result.current.detailPaneProps;
  await waitFor(() => expect(pane().workspaceFiles?.["instructions/AGENTS.md"]?.content).toContain("V2"));
  await act(async () => {
    await pane().onToggleWorkspaceDir?.("skills");
    await pane().onToggleWorkspaceDir?.("mcps");
  });
  await act(async () => {
    await pane().onToggleWorkspaceDir?.("skills/skill-v2");
  });
  await waitFor(() => expect(pane().workspaceFiles?.["skills/skill-v2/SKILL.md"]?.content).toContain("V2"));
  await waitFor(() => expect(pane().workspaceFiles?.["mcps/mcp.json"]?.content).toContain("V2"));
  // Match the cache update made by a successful restore; the template ID stays the same.
  revision = 1;
  act(() => {
    client.setQueryData(workspaceQueryKeys.hubTemplate(template.id), { ...template, revision });
  });
  await waitFor(() => expect(pane().selectedTemplate?.revision).toBe(1));
  expect(pane().workspaceFiles?.["skills/skill-v2/SKILL.md"]).toBeUndefined();
  await waitFor(() => expect(pane().workspaceFiles?.["instructions/AGENTS.md"]?.content).toContain("V1"));
  await act(async () => {
    await pane().onToggleWorkspaceDir?.("skills");
    await pane().onToggleWorkspaceDir?.("mcps");
  });
  expect(pane().workspaceEntries.some((entry) => entry.path.includes("skill-v2"))).toBe(false);
  await act(async () => {
    await pane().onToggleWorkspaceDir?.("skills/skill-v1");
  });
  await waitFor(() => expect(pane().workspaceFiles?.["skills/skill-v1/SKILL.md"]?.content).toContain("V1"));
  await waitFor(() => expect(pane().workspaceFiles?.["mcps/mcp.json"]?.content).toContain("V1"));
  vi.mocked(updateHubWorkspaceFile).mockImplementation(async (_id, path, content) => ({
    path,
    content,
    size: content.length,
  }));
  await act(async () => {
    await pane().onUpdateTemplateInstructions?.(pane().workspaceFiles?.["instructions/AGENTS.md"]?.content || "");
  });
  expect(updateHubWorkspaceFile).toHaveBeenCalledWith(
    template.id,
    "instructions/AGENTS.md",
    "V1: instructions/AGENTS.md",
  );
  unmount();
  client.clear();
});

it("discards a directory response that arrives after a version switch", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  let revision = 2;
  let resolveOld!: (value: HubWorkspaceListing) => void;
  vi.mocked(fetchHubTemplate).mockImplementation(async () => ({ ...template, revision }));
  vi.mocked(fetchHubWorkspace).mockImplementation(async (_id, path = "") => {
    if (path === "skills" && revision === 2)
      return new Promise<HubWorkspaceListing>((resolve) => {
        resolveOld = resolve;
      });
    return { kind: "dir", path, entries: path === "" ? [{ name: "skills", path: "skills", type: "dir" }] : [] };
  });
  vi.mocked(fetchHubWorkspaceFile).mockImplementation(async (_id, path) => ({
    path,
    content: `V${revision}`,
    size: 2,
  }));
  const { result, client, unmount } = setup();
  await waitFor(() =>
    expect(result.current.detailPaneProps.workspaceFiles?.["instructions/AGENTS.md"]?.content).toBe("V2"),
  );
  let pending: unknown;
  act(() => {
    pending = result.current.detailPaneProps.onToggleWorkspaceDir("skills");
  });
  await waitFor(() => expect(resolveOld).toBeDefined());
  revision = 1;
  act(() => {
    client.setQueryData(workspaceQueryKeys.hubTemplate(template.id), { ...template, revision });
  });
  await waitFor(() =>
    expect(result.current.detailPaneProps.workspaceFiles?.["instructions/AGENTS.md"]?.content).toBe("V1"),
  );
  await act(async () => {
    await result.current.detailPaneProps.onToggleWorkspaceDir("skills");
  });
  await act(async () => {
    resolveOld({
      kind: "dir",
      path: "skills",
      entries: [{ name: "old-skill", path: "skills/old-skill", type: "dir" }],
    });
    await pending;
  });
  expect(result.current.detailPaneProps.workspaceEntries.some((entry) => entry.path === "skills/old-skill")).toBe(
    false,
  );
  expect(result.current.detailPaneProps.loadingWorkspaceDirs.size).toBe(0);
  unmount();
  client.clear();
});
