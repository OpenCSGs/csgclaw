import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type PropsWithChildren } from "react";
import { MemoryRouter } from "react-router-dom";
import { AgentDetailPane } from "@/components/business/AgentDetails";
import { agentToDraft, type AgentLike, type JSONRecord } from "@/models/agents";
import type { MCPServer } from "@/models/mcp";
import { createTranslator } from "@/shared/i18n";
import { AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY } from "@/shared/storage/keys";
import { createQueryWrapper } from "../helpers/queryClient";

const worker: AgentLike = { id: "worker-1", name: "Worker", role: "worker", runtime_kind: "codex" };
const t = createTranslator("en");
const knowledge: MCPServer[] = [
  { name: "engineering", config: knowledgeConfig("101", "Engineering") },
  { name: "operations", config: knowledgeConfig("102", "Operations") },
];

function knowledgeConfig(id: string, displayName: string) {
  return {
    display_name: displayName,
    url: `https://example.com/${id}/mcp`,
    enabled: true,
    _meta: {
      "com.opencsg/mcp": {
        type: "llm_wiki",
        resource_id: id,
        content_id: `wiki-${id}`,
        auth_type: "csghub_access_token",
      },
    },
  };
}

function setup({
  deleteSucceeds = true,
  deleteCompletion,
  initialServers = [knowledge[0]],
}: {
  deleteSucceeds?: boolean;
  deleteCompletion?: Promise<void>;
  initialServers?: MCPServer[];
} = {}) {
  let workerServers: JSONRecord = Object.fromEntries(initialServers.map((server) => [server.name, server.config]));
  const fetch = vi.fn(async (input: string) => {
    const path = new URL(String(input), "http://localhost").pathname;
    if (path === "/api/v1/agents/worker-1/mcp-servers") return Response.json({ servers: workerServers });
    if (path === "/api/v1/mcp-servers") {
      return Response.json({ mcpServers: Object.fromEntries(knowledge.map((server) => [server.name, server.config])) });
    }
    if (path === "/api/v1/knowledge-bases/remote") return Response.json({ items: [], total: 0 });
    throw new Error(`Unexpected request: ${path}`);
  });
  vi.stubGlobal("fetch", fetch);
  const onDelete = vi.fn();
  const onInstall = vi.fn();
  function Harness() {
    const [servers, setServers] = useState(initialServers);
    const [deleteError, setDeleteError] = useState("");
    const [deleteBusy, setDeleteBusy] = useState(false);
    return (
      <AgentDetailPane
        item={worker}
        draft={agentToDraft(worker)}
        t={t}
        workspaceSupported
        requestedProfileTab="knowledge"
        knowledgeServers={servers}
        mcpDeleteError={deleteError}
        mcpDeleteBusy={deleteBusy}
        onDraftChange={vi.fn()}
        onDelete={vi.fn()}
        onInvite={vi.fn()}
        onOpenDM={vi.fn()}
        onRecreate={vi.fn()}
        onStart={vi.fn()}
        onStop={vi.fn()}
        onDeleteMCPServer={async (server) => {
          onDelete(server);
          const name = typeof server === "string" ? server : server.name;
          setDeleteBusy(true);
          await deleteCompletion;
          setDeleteBusy(false);
          if (!deleteSucceeds) {
            setDeleteError("Removal failed");
            return false;
          }
          delete workerServers[name];
          setServers((current) => current.filter((item) => item.name !== name));
          return true;
        }}
        onInstallMCPServers={async (names) => {
          onInstall(names);
          const installed = knowledge.filter((server) => names.includes(server.name));
          workerServers = Object.fromEntries(installed.map((server) => [server.name, server.config]));
          setServers(installed);
          return true;
        }}
      />
    );
  }
  const { wrapper: QueryWrapper } = createQueryWrapper();
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <MemoryRouter>
        <QueryWrapper>{children}</QueryWrapper>
      </MemoryRouter>
    );
  }
  render(<Harness />, { wrapper: Wrapper });
  return { onDelete, onInstall, user: userEvent.setup() };
}

async function openRemoval(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /Engineering/ }));
  const details = screen.getByRole("dialog", { name: "Engineering" });
  await user.click(within(details).getByRole("button", { name: t("profileRemoveResource") }));
  return screen.getByRole("dialog", { name: t("agentDeleteMCP") });
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.removeItem(AGENT_PROFILE_ACTIVE_TAB_STORAGE_KEY);
});

describe("Knowledge enablement after removal", () => {
  it.each([{ names: ["engineering"] }, { names: ["engineering", "operations"] }])(
    "keeps the worker list visible after enabling $names following removal",
    async ({ names }) => {
      const { onDelete, onInstall, user } = setup();
      const removal = await openRemoval(user);
      await user.click(within(removal).getByRole("button", { name: t("confirm") }));
      await waitFor(() => expect(onDelete).toHaveBeenCalledExactlyOnceWith(knowledge[0]));
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Engineering/ })).not.toBeInTheDocument();
      await user.click(screen.getAllByRole("button", { name: t("agentKnowledgeAdd") })[0]);
      const picker = screen.getByRole("dialog", { name: t("agentKnowledgeAdd") });
      await user.click(await within(picker).findByRole("checkbox", { name: "Engineering" }));
      if (names.includes("operations")) {
        await user.click(within(picker).getByRole("checkbox", { name: "Operations" }));
      }
      await user.click(within(picker).getByRole("button", { name: t("agentKnowledgeAdd") }));
      await waitFor(() => expect(onInstall).toHaveBeenCalledExactlyOnceWith(names));
      expect(screen.queryByRole("dialog", { name: t("agentKnowledgeAdd") })).not.toBeInTheDocument();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(screen.getByRole("switch", { name: "Engineering" })).toBeChecked();
      await user.click(screen.getByRole("button", { name: /Engineering/ }));
      expect(screen.getByRole("dialog", { name: "Engineering" })).toBeVisible();
    },
  );

  it("keeps knowledge details available when removal fails", async () => {
    const { onDelete, onInstall, user } = setup({ deleteSucceeds: false });
    const removal = await openRemoval(user);
    await user.click(within(removal).getByRole("button", { name: t("confirm") }));
    expect(await within(removal).findByText("Removal failed")).toBeVisible();
    expect(onDelete).toHaveBeenCalledExactlyOnceWith(knowledge[0]);
    await user.click(within(removal).getByRole("button", { name: t("cancel") }));
    expect(screen.getByRole("dialog", { name: "Engineering" })).toBeVisible();
    expect(onInstall).not.toHaveBeenCalled();
  });

  it("keeps another knowledge detail open when a previous removal completes", async () => {
    let completeRemoval!: () => void;
    const deleteCompletion = new Promise<void>((resolve) => {
      completeRemoval = resolve;
    });
    const { user } = setup({ deleteCompletion, initialServers: knowledge });
    const removal = await openRemoval(user);
    await user.click(within(removal).getByRole("button", { name: t("confirm") }));
    expect(within(removal).getByRole("button", { name: t("confirm") })).toBeDisabled();
    await user.click(within(removal).getByRole("button", { name: t("cancel") }));
    const details = screen.getByRole("dialog", { name: "Engineering" });
    await user.click(within(details).getByRole("button", { name: t("close") }));
    await user.click(screen.getByRole("button", { name: /Operations/ }));
    expect(screen.getByRole("dialog", { name: "Operations" })).toBeVisible();
    await act(async () => {
      completeRemoval();
      await deleteCompletion;
    });
    expect(screen.getByRole("dialog", { name: "Operations" })).toBeVisible();
  });
});
