import type { PropsWithChildren } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { AgentResourcePicker } from "@/components/business/AgentDetails/AgentResourcePicker";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import type { JSONRecord } from "@/models/agents";
import { createTranslator } from "@/shared/i18n";
import { createQueryWrapper } from "../helpers/queryClient";

function knowledgeConfig(id: string, name: string) {
  return {
    display_name: name,
    url: `https://example.com/${id}/mcp`,
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

const enabledConfig = knowledgeConfig("101", "Enabled knowledge");
const availableConfig = knowledgeConfig("102", "Available knowledge");
const newConfig = knowledgeConfig("103", "New knowledge");
const remoteItems = [
  { id: 101, content_id: "wiki-101", name: "Enabled knowledge", availability: "available" },
  { id: 102, content_id: "wiki-102", name: "Available knowledge", availability: "available" },
  { id: 103, content_id: "wiki-103", name: "New knowledge", availability: "available" },
];

function deferredResponse() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function setup(overrides: { global?: () => Promise<Response>; worker?: () => Promise<Response> } = {}) {
  let globalServers: JSONRecord = { enabled: enabledConfig, available: availableConfig };
  const responses = {
    global: async () => Response.json({ mcpServers: globalServers }),
    worker: async () => Response.json({ servers: { enabled: { ...enabledConfig, enabled: true } } }),
    ...overrides,
  };
  const fetch = vi.fn(async (input: string, init?: RequestInit) => {
    const path = new URL(String(input), "http://localhost").pathname;
    if (path === "/api/v1/agents/agent-1/mcp-servers") return responses.worker();
    if (path === "/api/v1/mcp-servers" && init?.method !== "POST") return responses.global();
    if (path === "/api/v1/knowledge-bases/remote") {
      return Response.json({ items: remoteItems, page: 1, per: 50, total: remoteItems.length });
    }
    if (path === "/api/v1/knowledge-bases/remote/103/mcp-config") {
      return Response.json({ name: "New knowledge", config: newConfig });
    }
    if (path === "/api/v1/mcp-servers" && init?.method === "POST") {
      globalServers = { ...globalServers, mcp_new: newConfig };
      return Response.json({ mcpServers: globalServers });
    }
    throw new Error(`Unexpected request: ${path}`);
  });
  vi.stubGlobal("fetch", fetch);
  const { client, wrapper: QueryWrapper } = createQueryWrapper();
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <MemoryRouter>
        <QueryWrapper>{children}</QueryWrapper>
      </MemoryRouter>
    );
  }
  const t = createTranslator("en");
  const onAdd = vi.fn().mockResolvedValue(true);
  const onClose = vi.fn();
  render(
    <AgentResourcePicker agentID="agent-1" kind="knowledge" authenticated onAdd={onAdd} onClose={onClose} t={t} />,
    { wrapper: Wrapper },
  );
  return { client, fetch, responses, onAdd, onClose, t, user: userEvent.setup() };
}

afterEach(() => vi.unstubAllGlobals());

describe("Agent knowledge picker state", () => {
  it("waits for worker state before showing global or remote knowledge rows", async () => {
    const worker = deferredResponse();
    const { client, t } = setup({ worker: () => worker.promise });
    await waitFor(() => {
      expect(client.getQueryData(workspaceQueryKeys.mcpServers())).toBeDefined();
      expect(client.getQueryData(workspaceQueryKeys.knowledgeBases(""))).toBeDefined();
    });
    expect(screen.getByRole("status")).toHaveTextContent(t("profileKnowledgeLoading"));
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByText("New knowledge")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: t("profileKnowledgeConnectEnable") })).not.toBeInTheDocument();
    await act(async () => {
      worker.resolve(Response.json({ servers: { enabled: { ...enabledConfig, enabled: true } } }));
    });
    const enabled = await screen.findByRole("checkbox", { name: "Enabled knowledge" });
    expect(enabled).toBeChecked();
    expect(enabled).toBeDisabled();
    expect(screen.getAllByRole("checkbox", { name: "Enabled knowledge" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: t("profileKnowledgeConnectEnable") })).toHaveLength(1);
  });

  it.each(["global", "worker"] as const)("keeps actions unavailable when initial %s state fails", async (failed) => {
    const { fetch, onAdd, t, user } = setup({
      [failed]: async () =>
        Response.json({ error: { code: "knowledge_test_failure", message: "State unavailable" } }, { status: 503 }),
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("State unavailable");
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: t("profileKnowledgeConnectEnable") })).not.toBeInTheDocument();
    const enable = screen.getByRole("button", { name: t("agentKnowledgeAdd") });
    expect(enable).toBeDisabled();
    await user.click(enable);
    expect(onAdd).not.toHaveBeenCalled();
    expect(
      fetch.mock.calls.some(([path, init]) => String(path).includes("/mcp-config") || init?.method === "POST"),
    ).toBe(false);
  });

  it("preserves checked state during a background refresh and applies the returned state", async () => {
    const { client, responses, t } = setup();
    expect(await screen.findByRole("checkbox", { name: "Enabled knowledge" })).toBeChecked();
    expect(await screen.findByRole("button", { name: t("profileKnowledgeConnectEnable") })).toBeEnabled();
    const worker = deferredResponse();
    responses.worker = () => worker.promise;
    let refresh!: Promise<void>;
    act(() => {
      refresh = client.invalidateQueries({ queryKey: workspaceQueryKeys.agentMCPServers("agent-1") });
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: t("profileKnowledgeConnectEnable") })).toBeDisabled(),
    );
    expect(screen.getByRole("checkbox", { name: "Enabled knowledge" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Enabled knowledge" })).toBeDisabled();
    expect(screen.queryByText(t("profileKnowledgeLoading"))).not.toBeInTheDocument();
    await act(async () => {
      worker.resolve(Response.json({ servers: { enabled: { ...enabledConfig, enabled: false } } }));
      await refresh;
    });
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "Enabled knowledge" })).not.toBeChecked());
    expect(screen.getByRole("checkbox", { name: "Enabled knowledge" })).toBeEnabled();
  });

  it.each(["global", "remote"] as const)(
    "uses the %s action with enabled, available, and unconnected rows together",
    async (action) => {
      const { fetch, onAdd, onClose, t, user } = setup();
      const enabled = await screen.findByRole("checkbox", { name: "Enabled knowledge" });
      expect(enabled).toBeChecked();
      expect(enabled).toBeDisabled();
      expect(screen.getByText(`Enabled knowledge · ${t("profileResourceEnabled")}`)).toBeVisible();
      const available = screen.getByRole("checkbox", { name: "Available knowledge" });
      expect(available).not.toBeChecked();
      expect(available).toBeEnabled();
      const remote = screen.getByRole("checkbox", { name: "New knowledge" });
      expect(remote).not.toBeChecked();
      expect(remote).toBeDisabled();
      expect(screen.getAllByRole("checkbox")).toHaveLength(3);
      expect(screen.getByRole("button", { name: "Manage global knowledge bases" })).toBeVisible();
      const connect = screen.getByRole("button", { name: t("profileKnowledgeConnectEnable") });
      expect(connect).toBeEnabled();
      const enable = screen.getByRole("button", { name: t("agentKnowledgeAdd") });
      expect(enable).toBeDisabled();
      if (action === "global") {
        await user.click(available);
        expect(enable).toBeEnabled();
        await user.click(enable);
        await waitFor(() => expect(onAdd).toHaveBeenCalledExactlyOnceWith(["available"]));
        expect(fetch.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
      } else {
        await user.click(connect);
        await waitFor(() => expect(onAdd).toHaveBeenCalledExactlyOnceWith(["mcp_new"]));
        const creates = fetch.mock.calls.filter(([, init]) => init?.method === "POST");
        expect(creates).toHaveLength(1);
        expect(JSON.parse(String(creates[0][1]?.body))).toEqual({ name: "New knowledge", config: newConfig });
      }
      expect(onClose).toHaveBeenCalledOnce();
    },
  );
});
