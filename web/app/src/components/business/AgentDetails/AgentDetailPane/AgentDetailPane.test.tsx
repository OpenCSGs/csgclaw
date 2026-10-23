import { createQueryWrapper } from "../../../../../tests/helpers/queryClient";
// @vitest-environment jsdom

import { act, render as baseRender, screen, waitFor, within, type RenderOptions } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useState } from "react";
import type { ReactElement, ReactNode, Ref } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentDraft, AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";
import buttonStyles from "@/components/ui/Button/Button.module.css";
import { AgentDetailPane } from "./AgentDetailPane";
import type { AgentDetailPaneHandle, AgentDetailPaneProps } from "./AgentDetailPane";

function render(ui: ReactElement, options?: RenderOptions) {
  const ProvidedWrapper = options?.wrapper;
  function Wrapper({ children }: { children: ReactNode }) {
    return <MemoryRouter>{ProvidedWrapper ? <ProvidedWrapper>{children}</ProvidedWrapper> : children}</MemoryRouter>;
  }
  return baseRender(ui, { ...options, wrapper: Wrapper });
}

const t: TranslateFn = (key) => key;

const agent: AgentLike = {
  id: "agent-1",
  name: "Saved agent",
  description: "Saved description",
  role: "assistant",
  runtime_kind: "codex",
  runtime: {
    kind: "codex",
    name: "codex",
    state: "running",
  },
  agent_profile: {
    model_id: "gpt-4.1-mini",
    model_provider_id: "csghub_lite",
    provider: "csghub_lite",
    profile_complete: true,
    reasoning_effort: "medium",
  },
};

const savedDraft: AgentDraft = {
  agent_id: "agent-1",
  api_key: "",
  api_key_preview: "",
  api_key_set: false,
  avatar: "",
  base_url: "",
  description: "Saved description",
  enable_fast_mode: false,
  envRows: [],
  from_template: "",
  headersText: "{}",
  image: "",
  instructions: "",
  mcpServers: {},
  model_id: "gpt-4.1-mini",
  model_provider_id: "csghub_lite",
  name: "Saved agent",
  provider: "csghub_lite",
  reasoning_effort: "medium",
  requestOptionsText: "{}",
  role: "assistant",
  runtime_kind: "codex",
  runtime_options: {},
  sandbox_enabled: false,
};

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

type HarnessProps = Partial<AgentDetailPaneProps>;

function Harness({
  detailPaneRef,
  onMetadataSave = vi.fn(),
  ...props
}: HarnessProps & { detailPaneRef?: Ref<AgentDetailPaneHandle> }) {
  const [draft, setDraft] = useState(savedDraft);
  return (
    <AgentDetailPane
      ref={detailPaneRef}
      item={agent}
      t={t}
      draft={draft}
      savedDraft={savedDraft}
      onDraftChange={setDraft}
      onMetadataSave={onMetadataSave}
      onDelete={vi.fn()}
      onInvite={vi.fn()}
      onOpenDM={vi.fn()}
      onRecreate={vi.fn()}
      onStart={vi.fn()}
      onStop={vi.fn()}
      {...props}
    />
  );
}

describe("AgentDetailPane metadata editing", () => {
  it("shows a dismissible standard error when skill descriptions cannot be read", async () => {
    const user = userEvent.setup();
    render(<Harness workspaceSupported skillsError="Some skill descriptions could not be read." />);
    await user.click(screen.getByRole("button", { name: /agentProfileSkillsTab/ }));
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Some skill descriptions could not be read.");
    expect(alert.parentElement).toHaveClass("form-error");
    await user.click(within(alert.parentElement!).getByRole("button", { name: /^close$/ }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it.each(["agent-1", "agent-manager"])("confirms recreation from the pending badge for %s", async (id) => {
    const user = userEvent.setup();
    const onRecreate = vi.fn();
    const item = {
      ...agent,
      id,
      name: id === "agent-manager" ? "manager" : agent.name,
      agent_profile: { ...agent.agent_profile, env_restart_required: true },
    };
    render(<Harness item={item} onRecreate={onRecreate} />);
    const badge = screen.getByRole("button", { name: "profileRestartRequired" });
    await user.click(badge);
    expect(screen.getByRole("dialog")).toHaveTextContent("agentRecreateConfirmMessage");
    expect(onRecreate).not.toHaveBeenCalled();
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "cancel" }));
    expect(onRecreate).not.toHaveBeenCalled();
    await user.click(badge);
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "agentRecreate" }));
    expect(onRecreate).toHaveBeenCalledExactlyOnceWith(item);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("disables the recreation badge while an agent action is running", () => {
    render(
      <Harness
        busyKey="agent-1:recreate"
        item={{ ...agent, agent_profile: { ...agent.agent_profile, env_restart_required: true } }}
      />,
    );
    expect(screen.getByRole("button", { name: "profileRestartRequired" })).toBeDisabled();
  });

  it("offers a primary new-task action for the current agent", async () => {
    const onNewTask = vi.fn();
    const onOpenDM = vi.fn();
    render(<Harness onNewTask={onNewTask} onOpenDM={onOpenDM} />);
    const button = screen.getByRole("button", { name: "agentStartTask" });
    expect(button).toHaveClass(buttonStyles["btn-primary"]);
    expect(button).toHaveAttribute("title", "agentStartTaskHint");
    expect(screen.queryByRole("button", { name: "openDM" })).not.toBeInTheDocument();
    await userEvent.setup().click(button);
    expect(onNewTask).toHaveBeenCalledWith(agent);
    expect(onOpenDM).not.toHaveBeenCalled();
  });

  it("writes an inferred model provider back to the draft so the change can be saved", async () => {
    const onDraftChange = vi.fn();
    render(
      <Harness
        draft={{ ...savedDraft, model_provider_id: "", provider: "" }}
        modelProviders={normalizeModelProviderCatalog({
          providers: [
            {
              id: "csghub-lite",
              display_name: "CSGHub Lite",
              status: "connected",
              models: ["gpt-4.1-mini"],
            },
          ],
        })}
        onDraftChange={onDraftChange}
      />,
    );

    await waitFor(() =>
      expect(onDraftChange).toHaveBeenCalledWith(
        expect.objectContaining({
          model_id: "gpt-4.1-mini",
          model_provider_id: "csghub-lite",
          provider: "csghub_lite",
        }),
      ),
    );
  });

  it("refreshes skills when entering the tab and returning from connectors", async () => {
    const user = userEvent.setup();
    const onRefreshSkills = vi.fn().mockResolvedValue(undefined);
    render(<Harness workspaceSupported onRefreshSkills={onRefreshSkills} />);
    expect(onRefreshSkills).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /agentProfileSkillsTab/ }));
    await waitFor(() => expect(onRefreshSkills).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole("button", { name: "agentAppsTab" }));
    await user.click(screen.getByRole("button", { name: /agentProfileSkillsTab/ }));
    await waitFor(() => expect(onRefreshSkills).toHaveBeenCalledTimes(2));
  });

  it("preserves the skill list while applying an enablement change", async () => {
    const skills = [{ name: "reviewer", description: "Review", enabled: true }];
    const view = render(<Harness workspaceSupported skills={skills} />);
    await userEvent.click(screen.getByRole("button", { name: /agentProfileSkillsTab/ }));
    const entry = screen.getByRole("button", { name: /reviewer/ });
    view.rerender(<Harness workspaceSupported skills={skills} resourceBusy="skill:reviewer" />);
    expect(screen.getByRole("button", { name: /reviewer/ })).toBe(entry);
    expect(screen.getByRole("switch", { name: "reviewer" })).toBeDisabled();
    view.rerender(<Harness workspaceSupported skills={[{ ...skills[0], enabled: false }]} />);
    expect(screen.getByRole("button", { name: /reviewer/ })).toBe(entry);
    expect(screen.getByRole("switch", { name: "reviewer" })).not.toBeChecked();
  });

  it("places a dismiss button on the right side of a notice", async () => {
    const user = userEvent.setup();
    const onDismissNotice = vi.fn();
    render(<Harness notice="Agent deleted" noticeTone="success" onDismissNotice={onDismissNotice} />);

    const notice = screen.getByText("Agent deleted").closest('[role="status"]');
    const closeButton = screen.getByRole("button", { name: "close" });
    expect(notice).toContainElement(closeButton);

    await user.click(closeButton);
    expect(onDismissNotice).toHaveBeenCalledTimes(1);
  });

  it("shows a degraded gateway runtime as unavailable instead of online", () => {
    render(
      <Harness
        item={{
          ...agent,
          status: "running",
          runtime: {
            kind: "openclaw_sandbox",
            name: "openclaw",
            state: "running",
            availability: {
              state: "degraded",
              reason: "control_plane_unavailable",
              expires_at: "2099-01-01T00:00:00Z",
            },
          },
        }}
      />,
    );

    expect(screen.getAllByText("offline")).toHaveLength(1);
    expect(screen.getAllByText("agentRuntimeDockerUnavailable")).toHaveLength(1);
    expect(screen.queryByText("online")).not.toBeInTheDocument();
  });

  it("reverts name edits on Escape without saving on blur", async () => {
    const user = userEvent.setup();
    const onMetadataSave = vi.fn();
    const onOuterKeyDown = vi.fn();
    render(
      <div onKeyDown={onOuterKeyDown}>
        <Harness onMetadataSave={onMetadataSave} />
      </div>,
    );

    await user.click(screen.getByRole("button", { name: "editAgentName" }));
    const input = screen.getByDisplayValue("Saved agent");
    await user.clear(input);
    await user.type(input, "Draft agent");
    onOuterKeyDown.mockClear();
    await user.keyboard("{Escape}");

    expect(screen.getByRole("button", { name: "editAgentName" })).toHaveTextContent("Saved agent");
    expect(onOuterKeyDown).not.toHaveBeenCalled();
    await waitFor(() => expect(onMetadataSave).not.toHaveBeenCalled());
  });

  it("reverts description edits on Escape without saving on blur", async () => {
    const user = userEvent.setup();
    const onMetadataSave = vi.fn();
    const onOuterKeyDown = vi.fn();
    render(
      <div onKeyDown={onOuterKeyDown}>
        <Harness onMetadataSave={onMetadataSave} />
      </div>,
    );

    await user.click(screen.getByRole("button", { name: "editDescription" }));
    const textarea = screen.getByDisplayValue("Saved description");
    await user.clear(textarea);
    await user.type(textarea, "Draft description");
    onOuterKeyDown.mockClear();
    await user.keyboard("{Escape}");

    expect(screen.getByRole("button", { name: "editDescription" })).toHaveTextContent("Saved description");
    expect(onOuterKeyDown).not.toHaveBeenCalled();
    await waitFor(() => expect(onMetadataSave).not.toHaveBeenCalled());
  });

  it("commits active name edits through the imperative close hook", async () => {
    const user = userEvent.setup();
    const detailPaneRef = createRef<AgentDetailPaneHandle>();
    const onMetadataSave = vi.fn();
    render(<Harness detailPaneRef={detailPaneRef} onMetadataSave={onMetadataSave} />);

    await user.click(screen.getByRole("button", { name: "editAgentName" }));
    const input = screen.getByDisplayValue("Saved agent");
    await user.clear(input);
    await user.type(input, "Backdrop saved agent");

    expect(detailPaneRef.current?.commitActiveMetadataEdit()).toEqual(["name"]);
    expect(onMetadataSave).toHaveBeenCalledWith({ name: "Backdrop saved agent" });
  });

  it("cancels active name edits through the imperative escape hook", async () => {
    const user = userEvent.setup();
    const detailPaneRef = createRef<AgentDetailPaneHandle>();
    const onMetadataSave = vi.fn();
    render(<Harness detailPaneRef={detailPaneRef} onMetadataSave={onMetadataSave} />);

    await user.click(screen.getByRole("button", { name: "editAgentName" }));
    const input = screen.getByDisplayValue("Saved agent");
    await user.clear(input);
    await user.type(input, "Esc canceled agent");

    let canceledFields: ReturnType<AgentDetailPaneHandle["cancelActiveMetadataEdit"]> = [];
    act(() => {
      canceledFields = detailPaneRef.current?.cancelActiveMetadataEdit() ?? [];
    });

    expect(canceledFields).toEqual(["name"]);
    expect(screen.getByRole("button", { name: "editAgentName" })).toHaveTextContent("Saved agent");
    await waitFor(() => expect(onMetadataSave).not.toHaveBeenCalled());
  });
});

describe("AgentDetailPane model loading error", () => {
  it("keeps the model controls aligned and exposes retryable technical details", async () => {
    const user = userEvent.setup();
    const onRetryModels = vi.fn();
    const technicalError = "request models: proxyconnect tcp: connection refused";

    render(<Harness modelBusy={false} modelError={new Error(technicalError)} onRetryModels={onRetryModels} />);

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("modelLoadFailed");
    expect(alert).toHaveTextContent("profileModelLoadErrorHelp");
    expect(alert).toHaveTextContent("profileModelCurrentSelectionRetained");

    await user.click(screen.getByText("profileModelErrorDetails"));
    expect(screen.getByText(technicalError)).toBeVisible();

    await user.click(screen.getByRole("button", { name: "retry" }));
    expect(onRetryModels).toHaveBeenCalledTimes(1);
  });
});

describe("AgentDetailPane memory", () => {
  it("shows the runtime memory tab, readable document, and enable toggle", async () => {
    const user = userEvent.setup();
    const onMemoryChange = vi.fn();
    const fetchMock = vi.fn<typeof fetch>(async (_input, init) => {
      const enabled = init?.method === "PUT" ? false : true;
      return new Response(
        JSON.stringify({
          enabled,
          ready: true,
          name: "memory_summary.md",
          location: "$CODEX_HOME/memories/memory_summary.md",
          content: "# Durable memory\n\nRemember **this**.\n\n<script>unsafe()</script>\n",
        }),
        { headers: { "content-type": "application/json" }, status: 200 },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<Harness item={{ ...agent, memory_supported: true }} onMemoryChange={onMemoryChange} />);

    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const document = await screen.findByRole("region", { name: "agentMemoryDocumentLabel" });
    expect(within(document).getByRole("heading", { name: "Durable memory" })).toBeInTheDocument();
    expect(within(document).getByText("this").tagName).toBe("STRONG");
    expect(document.querySelector("script")).not.toBeInTheDocument();
    expect(document.closest(".agent-memory-document-shell")).toHaveClass("agent-section-form");
    const copy = screen.getByRole("button", { name: "copyToClipboard" });
    const clipboardWrite = vi.spyOn(navigator.clipboard, "writeText");
    expect(copy).toHaveClass("agent-memory-copy-button");
    await user.click(copy);
    expect(clipboardWrite).toHaveBeenCalledWith(
      "# Durable memory\n\nRemember **this**.\n\n<script>unsafe()</script>\n",
    );
    expect(screen.getByRole("button", { name: "copiedToClipboard" })).toBeInTheDocument();
    const refresh = screen.getByRole("button", { name: "agentMemoryRefresh" });
    expect(refresh).toHaveClass("agent-skill-add-button");
    expect(refresh.closest(".agent-memory-section-heading")).toBeInTheDocument();
    expect(screen.getByText("$CODEX_HOME/memories/memory_summary.md").tagName).toBe("CODE");

    const toggle = screen.getByRole("checkbox", { name: "agentMemoryEnabled" });
    expect(toggle).toBeChecked();
    expect(toggle.closest(".agent-memory-setting-actions")).toBeInTheDocument();
    expect(toggle.closest(".agent-memory-section-heading")).toBeInTheDocument();
    await user.click(toggle);

    await waitFor(() => expect(toggle).not.toBeChecked());
    expect(onMemoryChange).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenLastCalledWith(
      "api/v1/agents/agent-1/memory",
      expect.objectContaining({ body: JSON.stringify({ enabled: false }), method: "PUT" }),
    );
  });

  it("hides the memory tab when the runtime does not expose the capability", () => {
    render(<Harness item={{ ...agent, memory_supported: false }} />);

    expect(screen.queryByRole("button", { name: "agentMemoryTab" })).not.toBeInTheDocument();
  });

  it("uses the compact tab empty state before memory_summary.md is generated", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(
        async () =>
          new Response(
            JSON.stringify({
              enabled: true,
              ready: false,
              name: "memory_summary.md",
              location: "$CODEX_HOME/memories/memory_summary.md",
              content: "",
            }),
            {
              headers: { "content-type": "application/json" },
              status: 200,
            },
          ),
      ),
    );

    render(<Harness item={{ ...agent, memory_supported: true }} />);

    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const emptyTitle = await screen.findByText("agentMemoryEmptyTitle");
    expect(emptyTitle.closest(".agent-memory-summary-empty")).toHaveClass("agent-skills-summary-empty");
    expect(screen.queryByRole("region", { name: "agentMemoryDocumentLabel" })).not.toBeInTheDocument();
  });
});

describe("AgentDetailPane MCP snapshots", () => {
  it("opens knowledge deep links and keeps knowledge separate from tool connectors", async () => {
    const user = userEvent.setup();
    const knowledge = {
      name: "engineering",
      config: {
        url: "https://example.test/wiki",
        _meta: {
          "com.opencsg/mcp": {
            type: "llm_wiki",
            resource_id: "kb-1",
            content_id: "wiki-1",
            auth_type: "csghub_access_token",
          },
        },
      },
    };
    const tool = { name: "filesystem", config: { command: "node" } };
    const onSetResourceEnabled = vi.fn().mockResolvedValue(undefined);
    const { rerender } = render(
      <Harness
        requestedProfileTab="knowledge"
        mcpServers={[tool]}
        knowledgeServers={[knowledge]}
        onSetResourceEnabled={onSetResourceEnabled}
      />,
    );
    expect(screen.getByRole("switch", { name: "engineering" })).toBeVisible();
    expect(screen.queryByText("filesystem")).not.toBeInTheDocument();
    await user.click(screen.getByRole("switch", { name: "engineering" }));
    expect(onSetResourceEnabled).toHaveBeenCalledWith("mcp", "engineering", false);
    rerender(<Harness requestedProfileTab="connectors" mcpServers={[tool]} knowledgeServers={[knowledge]} />);
    expect(screen.getByRole("switch", { name: "filesystem" })).toBeVisible();
    expect(screen.queryByText("engineering")).not.toBeInTheDocument();
    rerender(<Harness requestedProfileTab="connectors" mcpServers={[knowledge]} knowledgeServers={[]} />);
    expect(screen.getByRole("switch", { name: "engineering" })).toBeVisible();
  });
  it.each([
    { runtimeKind: "dsh", sandbox: false, supported: true },
    { runtimeKind: "openclaw_sandbox", sandbox: true, supported: false },
  ])("shows the Feishu connector for $runtimeKind", async ({ runtimeKind, sandbox, supported }) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () => new Response(JSON.stringify({ items: [] }), { headers: { "content-type": "application/json" } }),
      ),
    );
    render(
      <Harness
        draft={{ ...savedDraft, runtime_kind: runtimeKind, sandbox_enabled: sandbox }}
        item={{
          ...agent,
          runtime_kind: runtimeKind,
          runtime: { kind: runtimeKind, name: runtimeKind, sandbox_enabled: sandbox, state: "running" },
        }}
        requestedProfileTab="apps"
      />,
    );
    const toggle = await screen.findByRole("switch", { name: "feishuCLIWorkerSwitch" });
    if (supported) expect(toggle).toBeEnabled();
    else {
      expect(toggle).toBeDisabled();
      expect(screen.getByText("feishuCLIUnsupported")).toBeVisible();
    }
  });

  it("distinguishes manually added MCP servers from app-managed services", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) =>
        Response.json({
          items: String(url).includes("/agents/")
            ? [
                {
                  installation_id: "app-1",
                  agent_id: "agent-1",
                  app_id: "gitlab",
                  name: "Work GitLab",
                  enabled: true,
                  disconnected: false,
                  status: "connected",
                  config: {},
                  credentials_set: {},
                  tools: [],
                  created_at: "",
                  updated_at: "",
                },
              ]
            : [],
        }),
      ),
    );
    const user = userEvent.setup();
    render(<Harness workspaceSupported />);
    await user.click(screen.getByRole("button", { name: "agentAppsTab" }));
    expect(await screen.findByText("connectorsTools")).toBeVisible();
    expect(screen.getByText("appManualMCPEmpty")).toBeVisible();
    expect(screen.queryByText("agentMCPEmpty")).not.toBeInTheDocument();
  });
  it("offers a manual update when the agent snapshot differs from the global MCP", async () => {
    const user = userEvent.setup();
    const onUpdateMCPServer = vi.fn().mockResolvedValue(true);
    const mcpServer = {
      name: "kb_investment",
      description: "Saved knowledge base snapshot",
      config: {
        url: "https://old.example.test/mcp",
        _meta: {
          "com.opencsg/mcp": {
            auth_type: "csghub_access_token",
            content_id: "kb_investment",
            resource_id: "143",
            type: "llm_wiki",
          },
        },
      },
    };
    render(
      <Harness
        workspaceSupported
        mcpServers={[mcpServer]}
        mcpUpdateAvailableNames={new Set([mcpServer.name])}
        onUpdateMCPServer={onUpdateMCPServer}
      />,
    );

    await user.click(screen.getByRole("button", { name: /profileKnowledgeTitle/ }));
    expect(screen.getByText("agentKnowledgeMCPUpdateAvailable")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "agentMCPUpdateConfig" }));
    expect(onUpdateMCPServer).toHaveBeenCalledWith(mcpServer);
  });

  it("keeps a stale knowledge base MCP removable without offering update or retry", async () => {
    const user = userEvent.setup();
    const onDeleteMCPServer = vi.fn().mockResolvedValue(true);
    const onUpdateMCPServer = vi.fn();
    const mcpServer = {
      name: "kb_deleted",
      description: "Deleted knowledge base snapshot",
      config: {
        url: "https://old.example.test/mcp",
        _meta: {
          "com.opencsg/mcp": {
            auth_type: "csghub_access_token",
            content_id: "kb_deleted",
            resource_id: "143",
            type: "llm_wiki",
          },
        },
      },
    };
    render(
      <Harness
        workspaceSupported
        mcpServers={[mcpServer]}
        mcpSourceUnavailableNames={new Set([mcpServer.name])}
        onUpdateMCPServer={onUpdateMCPServer}
        onDeleteMCPServer={onDeleteMCPServer}
      />,
      { wrapper: createQueryWrapper().wrapper },
    );

    await user.click(screen.getByRole("button", { name: /profileKnowledgeTitle/ }));
    expect(screen.getByText("agentKnowledgeMCPSourceDeleted")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "agentMCPUpdateConfig" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: new RegExp(mcpServer.description) }));
    const deleteButton = within(screen.getByRole("dialog")).getByRole("button", { name: "profileRemoveResource" });
    expect(deleteButton).toBeEnabled();
    expect(deleteButton).toHaveClass(buttonStyles["btn-danger"]);
  });

  it("keeps source-check failures silent while the saved MCP remains usable", async () => {
    const user = userEvent.setup();
    const mcpServer = {
      name: "kb_unchecked",
      description: "Saved knowledge base snapshot",
      config: {
        url: "https://saved.example.test/mcp",
        _meta: {
          "com.opencsg/mcp": {
            auth_type: "csghub_access_token",
            content_id: "kb_unchecked",
            resource_id: "144",
            type: "llm_wiki",
          },
        },
      },
    };
    render(<Harness workspaceSupported mcpServers={[mcpServer]} />, { wrapper: createQueryWrapper().wrapper });

    await user.click(screen.getByRole("button", { name: /profileKnowledgeTitle/ }));
    expect(screen.queryByText("agentKnowledgeMCPSourceDeleted")).not.toBeInTheDocument();
    expect(screen.queryByText("agentKnowledgeMCPUpdateAvailable")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: new RegExp(mcpServer.description) }));
    expect(within(screen.getByRole("dialog")).getByRole("button", { name: "profileRemoveResource" })).toBeEnabled();
  });
});

describe("Profile 资源交互", () => {
  it.each(["codex", "dsh"])("%s 的开关与详情入口独立", async (runtimeKind) => {
    const user = userEvent.setup();
    const onSetResourceEnabled = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ entries: [], path: "reviewer/SKILL.md", content: "# Reviewer" }), {
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    render(
      <Harness
        item={{
          ...agent,
          runtime_kind: runtimeKind,
          runtime: { ...agent.runtime, kind: runtimeKind, name: runtimeKind },
        }}
        workspaceSupported
        skills={[{ name: "reviewer", description: "Review", enabled: false }]}
        onSetResourceEnabled={onSetResourceEnabled}
      />,
      { wrapper: createQueryWrapper().wrapper },
    );
    await user.click(screen.getByRole("button", { name: /agentProfileSkillsTab/ }));
    await user.click(screen.getByRole("switch", { name: "reviewer" }));
    expect(onSetResourceEnabled).toHaveBeenCalledWith("skill", "reviewer", true);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /reviewer/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "reviewer" })).toBeVisible();
    expect(within(dialog).getByRole("switch", { name: "reviewer" })).toBeEnabled();
  });
});

it.each(["skill", "mcp"])("%s 更新期间保留其他资源状态并标记当前条目", async (kind) => {
  const onSetResourceEnabled = vi.fn();
  render(
    <Harness
      workspaceSupported
      resourceBusy={`${kind}:alpha`}
      skills={[
        { name: "alpha", enabled: true },
        { name: "beta", enabled: true },
        { name: "gamma", enabled: false },
      ]}
      mcpServers={[
        { name: "alpha", config: { enabled: true } },
        { name: "beta", config: { enabled: true } },
        { name: "gamma", config: { enabled: false } },
      ]}
      onSetResourceEnabled={onSetResourceEnabled}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
  await userEvent.click(
    screen.getByRole("button", { name: kind === "skill" ? /agentProfileSkillsTab/ : /agentAppsTab/ }),
  );
  for (const name of ["alpha", "beta", "gamma"]) {
    const control = screen.getByRole("switch", { name });
    expect(control).toBeDisabled();
    expect(control).toHaveAttribute("aria-checked", String(name !== "gamma"));
    expect(control).toHaveAttribute("aria-busy", String(name === "alpha"));
  }
  if (kind === "mcp") expect(screen.getByText("resourceUpdating")).toBeVisible();
  expect(onSetResourceEnabled).not.toHaveBeenCalled();
});

it("hides failed CLI providers even when stale models remain", async () => {
  const user = userEvent.setup();
  render(
    <Harness
      modelProviders={normalizeModelProviderCatalog({
        providers: [
          { id: "codex", display_name: "Codex", status: "failed", models: ["stale-codex"] },
          { id: "claude_code", display_name: "Claude Code", status: "failed", models: ["stale-claude"] },
          { id: "custom", display_name: "Available API", status: "connected", models: ["available-model"] },
        ],
      })}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
  await user.click(screen.getByRole("combobox", { name: "profileModelProvider" }));
  expect(screen.queryByRole("option", { name: /Codex/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /Claude Code/ })).not.toBeInTheDocument();
  expect(screen.getByRole("option", { name: /Available API/ })).not.toHaveAttribute("aria-disabled", "true");
});

it("recommends and selects the OpenCSG auto model while preserving its model ID", async () => {
  const user = userEvent.setup();
  render(
    <Harness
      modelProviders={normalizeModelProviderCatalog({
        providers: [
          { id: "opencsg", display_name: "OpenCSG", status: "connected", models: ["qwen3", "auto"] },
          { id: "csghub-lite", display_name: "CSGHub Lite", status: "connected", models: ["gpt-4.1-mini"] },
        ],
      })}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );

  await user.click(screen.getByRole("combobox", { name: "profileModelProvider" }));
  expect(screen.queryByRole("option", { name: "profileProviderSelect" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: /OpenCSG/ }));

  const modelSelect = screen.getByRole("combobox", { name: "profileModel" });
  expect(modelSelect).toHaveTextContent("modelProviderAutoName");
  expect(modelSelect).not.toHaveTextContent("modelProviderAutoBadge");

  await user.click(modelSelect);
  const autoOption = screen.getByRole("option", { name: /modelProviderAutoName/ });
  expect(autoOption).toHaveTextContent("modelProviderAutoBadge");
  expect(autoOption).toHaveTextContent("modelProviderAutoDescription");
  expect(screen.queryByRole("option", { name: "profileSelectModel" })).not.toBeInTheDocument();
});
