import { act, render, screen, waitFor, within } from "@testing-library/react";
import { useState } from "react";
import userEvent from "@testing-library/user-event";
import { AgentCreateWizard } from "./AgentCreateWizard";
import { AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { fetchSkills } from "@/api/skills";
import { fetchMCPServers } from "@/api/mcp";
import { fetchAppResources } from "@/api/apps";
import { fetchRemoteKnowledgeBases } from "@/api/knowledgeBases";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { createQueryWrapper } from "../../../../../../tests/helpers/queryClient";
vi.mock("@/api/skills", () => ({ fetchSkills: vi.fn() }));
vi.mock("@/api/mcp", () => ({ fetchMCPServers: vi.fn() }));
vi.mock("@/api/apps", () => ({ fetchAppResources: vi.fn() }));
vi.mock("@/api/knowledgeBases", () => ({ fetchRemoteKnowledgeBases: vi.fn() }));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(fetchSkills).mockResolvedValue([{ name: "Research", description: "Research skill" }]);
  vi.mocked(fetchMCPServers).mockResolvedValue({ mcpServers: {} });
  vi.mocked(fetchAppResources).mockResolvedValue([]);
  vi.mocked(fetchRemoteKnowledgeBases).mockResolvedValue({ items: [], page: 1, per: 50, total: 0 });
});

function setup(valid = true, authenticated = false, queries = createQueryWrapper()) {
  const save = vi.fn();
  function Harness() {
    const [instructions, setInstructions] = useState("");
    return (
      <AgentCreateWizard
        profile={
          <label>
            Name
            <input defaultValue="Worker" />
          </label>
        }
        instructions={instructions}
        onInstructionsChange={setInstructions}
        valid={valid}
        busy={false}
        progress={null}
        authenticated={authenticated}
        onRequireAuth={() => true}
        onClose={vi.fn()}
        onSave={save}
        t={(key) => key}
      />
    );
  }
  render(<Harness />, { wrapper: queries.wrapper });
  return save;
}

describe("AgentCreateWizard", () => {
  it("keeps the draft across steps and submits selected resources only at the last step", async () => {
    const user = userEvent.setup();
    const save = setup();
    await user.type(screen.getByRole("textbox", { name: "Name" }), " renamed");
    await user.click(screen.getByRole("button", { name: "agentWizardNext" }));
    await user.type(screen.getByRole("textbox", { name: "agentInstructions" }), "Write reports");
    await user.click(screen.getByRole("button", { name: "agentWizardNext" }));
    await user.click(await screen.findByRole("checkbox", { name: /Research/ }));
    await user.click(screen.getByRole("button", { name: /agentProfileTab/ }));
    expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("Worker renamed");
    await user.click(screen.getByRole("button", { name: /agentInstructions/ }));
    expect(screen.getByRole("textbox", { name: "agentInstructions" })).toHaveValue("Write reports");
    await user.click(screen.getByRole("button", { name: /resourcesKnowledgeBasesLabel/ }));
    expect(screen.getByText("iaKnowledgeLoginRequired")).toBeVisible();
    expect(save).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "agentCreateSave" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({ skills: ["Research"], connectors: [], apps: [], knowledgeBases: [] }),
    );
  });
  it("cannot create from an optional step when the required profile is incomplete", async () => {
    const user = userEvent.setup();
    const save = setup(false);
    expect(screen.getByRole("button", { name: "agentWizardNext" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: /resourcesKnowledgeBasesLabel/ }));
    expect(screen.getByRole("button", { name: "agentCreateSave" })).toBeDisabled();
    expect(save).not.toHaveBeenCalled();
  });

  it("starts all resource requests on opening and reuses them across steps", async () => {
    let resolveSkills!: (value: Awaited<ReturnType<typeof fetchSkills>>) => void;
    vi.mocked(fetchSkills).mockReturnValue(
      new Promise((resolve) => {
        resolveSkills = resolve;
      }),
    );
    vi.mocked(fetchMCPServers).mockResolvedValue({ mcpServers: { Docs: { command: "docs" } } });
    const user = userEvent.setup();
    setup(true, true);
    expect(screen.getByRole("textbox", { name: "Name" })).toBeEnabled();
    await waitFor(() => {
      expect(fetchSkills).toHaveBeenCalledOnce();
      expect(fetchMCPServers).toHaveBeenCalledOnce();
      expect(fetchAppResources).toHaveBeenCalledOnce();
      expect(fetchRemoteKnowledgeBases).toHaveBeenCalledOnce();
    });
    await user.click(screen.getByRole("button", { name: "agentProfileSkillsTab" }));
    expect(screen.getByRole("status", { name: "agentWizardLoadingResources" })).toBeVisible();
    expect(screen.queryByText("iaListSearchEmpty")).not.toBeInTheDocument();
    await act(async () => resolveSkills([{ name: "Research", description: "Research skill" }]));
    await user.click(await screen.findByRole("checkbox", { name: /Research/ }));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "agentAppsTab" }));
    expect(screen.getByRole("checkbox", { name: /^Docs/ })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "resourcesKnowledgeBasesLabel" }));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "agentProfileSkillsTab" }));
    expect(screen.getByRole("checkbox", { name: /Research/ })).toBeChecked();
    for (const fetcher of [fetchSkills, fetchMCPServers, fetchAppResources, fetchRemoteKnowledgeBases]) {
      expect(fetcher).toHaveBeenCalledOnce();
    }
  });

  it("does not fetch knowledge bases before authentication", async () => {
    const user = userEvent.setup();
    setup();
    await user.click(screen.getByRole("button", { name: "resourcesKnowledgeBasesLabel" }));
    expect(screen.getByText("iaKnowledgeLoginRequired")).toBeVisible();
    expect(fetchRemoteKnowledgeBases).not.toHaveBeenCalled();
  });

  it("lists catalog connectors by display name and binds selected connector identities", async () => {
    vi.mocked(fetchMCPServers).mockResolvedValue({
      mcpServers: {
        bing: { display_name: "必应搜索中文", description: "搜索网页", url: "https://example.test/bing" },
        fetch: { command: "fetch" },
        disabled: { display_name: "已停用工具", command: "tool", disabled: true },
        knowledge: {
          display_name: "知识库工具",
          _meta: {
            "com.opencsg/mcp": {
              type: "llm_wiki",
              resource_id: "143",
              content_id: "kb",
              auth_type: "csghub_access_token",
            },
          },
        },
      },
    });
    const user = userEvent.setup();
    const save = setup();
    await user.click(screen.getByRole("button", { name: "agentAppsTab" }));
    const bing = await screen.findByRole("checkbox", { name: /必应搜索中文/ });
    expect(screen.getByRole("checkbox", { name: "fetch" })).toBeVisible();
    expect(screen.getByRole("checkbox", { name: /已停用工具/ })).toBeDisabled();
    expect(screen.queryByRole("checkbox", { name: /mcpServers|知识库工具/ })).not.toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "search" }), "必应");
    expect(screen.queryByRole("checkbox", { name: "fetch" })).not.toBeInTheDocument();
    await user.click(bing);
    await user.click(screen.getByRole("button", { name: "resourcesKnowledgeBasesLabel" }));
    await user.click(screen.getByRole("button", { name: "agentCreateSave" }));
    expect(save).toHaveBeenCalledWith({ skills: [], connectors: ["bing"], apps: [], knowledgeBases: [] });
  });

  it("keeps cached rows available while refreshing and supports retry after a resource failure", async () => {
    const queries = createQueryWrapper();
    queries.client.setQueryData(workspaceQueryKeys.skills(), [{ name: "Cached skill" }]);
    let rejectSkills!: (error: Error) => void;
    vi.mocked(fetchSkills).mockReturnValueOnce(
      new Promise((_, reject) => {
        rejectSkills = reject;
      }),
    );
    const user = userEvent.setup();
    setup(true, false, queries);
    await user.click(screen.getByRole("button", { name: "agentProfileSkillsTab" }));
    expect(screen.getByRole("checkbox", { name: "Cached skill" })).toBeVisible();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    await act(async () => rejectSkills(new Error("Resource unavailable")));
    expect(await screen.findByRole("alert")).toHaveTextContent("Resource unavailable");
    expect(screen.getByRole("checkbox", { name: "Cached skill" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "retry" }));
    expect(await screen.findByRole("checkbox", { name: /Research/ })).toBeVisible();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

it("keeps the nested avatar editor inside the wizard focus scope", async () => {
  const user = userEvent.setup();
  const close = vi.fn();
  function AvatarHarness() {
    const [portal, setPortal] = useState<HTMLDivElement | null>(null);
    const [avatar, setAvatar] = useState("");
    return (
      <AgentCreateWizard
        profile={
          <AgentAvatarPicker
            value={avatar}
            onChange={setAvatar}
            mode="edit"
            portalContainer={portal}
            t={(key) => key}
          />
        }
        onPortalContainer={setPortal}
        instructions=""
        onInstructionsChange={vi.fn()}
        valid
        busy={false}
        progress={null}
        authenticated={false}
        onRequireAuth={() => true}
        onClose={close}
        onSave={vi.fn()}
        t={(key) => key}
      />
    );
  }
  render(<AvatarHarness />, { wrapper: createQueryWrapper().wrapper });
  await user.click(screen.getByRole("button", { name: "editAvatar" }));
  let editor = screen.getByRole("dialog", { name: "editAvatar" });
  await user.click(within(editor).getAllByRole("radio")[0]);
  await user.click(within(editor).getByRole("button", { name: "confirm" }));
  expect(screen.queryByRole("dialog", { name: "editAvatar" })).not.toBeInTheDocument();
  expect(screen.getByRole("dialog", { name: "createAgentWizardTitle" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: /editAvatar/ }));
  editor = screen.getByRole("dialog", { name: "editAvatar" });
  within(editor).getByRole("button", { name: "confirm" }).focus();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "editAvatar" })).not.toBeInTheDocument();
  expect(close).not.toHaveBeenCalled();
});
