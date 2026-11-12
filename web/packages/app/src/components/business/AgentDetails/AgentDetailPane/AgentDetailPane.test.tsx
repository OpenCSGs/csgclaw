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
import buttonStyles from "csgclaw-ui/testing/Button/Button.module.css";
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
            kind: "dsh",
            name: "codex",
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
  it("distinguishes an unknown model directory from a confirmed missing model", () => {
    const { rerender } = render(<Harness modelDirectory={null} />);
    const model = screen.getByRole("combobox", { name: "profileModel" });
    expect(model).toHaveTextContent("gpt-4.1-mini");
    expect(model).toHaveAccessibleDescription("profileModelAvailabilityUnknown");
    expect(model).toBeDisabled();

    rerender(<Harness modelDirectory={[]} />);
    expect(model).toHaveTextContent("gpt-4.1-mini");
    expect(model).toHaveAccessibleDescription("profileModelOutsideDirectory");

    rerender(<Harness modelDirectory={["gpt-4.1-mini"]} modelSaveError="Model no longer routable" />);
    expect(model).toHaveAccessibleDescription("Model no longer routable");
    expect(model).toHaveAttribute("aria-invalid", "true");
  });

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
  it("updates idle memory automatically without a refresh action", async () => {
    const user = userEvent.setup();
    let reads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(
        async () =>
          new Response(
            JSON.stringify({
              available: true,
              enabled: true,
              ready: true,
              name: "MEMORY.md",
              revision: ++reads,
              status: "idle",
              initialization: "complete",
              content: "",
              entries: reads > 1 ? [{ id: "new", kind: "fact", text: "New background memory" }] : [],
            }),
            { headers: { "content-type": "application/json" } },
          ),
      ),
    );
    render(<Harness item={{ ...agent, memory_supported: true }} />);
    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const document = await screen.findByRole("region", { name: "agentMemoryDocumentLabel" });
    expect(await screen.findByText("New background memory", {}, { timeout: 4000 })).toBeVisible();
    expect(screen.getByRole("region", { name: "agentMemoryDocumentLabel" })).toBe(document);
    expect(screen.queryByRole("button", { name: "agentMemoryRefresh" })).not.toBeInTheDocument();
  });

  it("automatically refreshes without replacing the document or starting history learning", async () => {
    const user = userEvent.setup();
    const memory = {
      available: true,
      enabled: true,
      ready: true,
      name: "MEMORY.md",
      revision: 4,
      initialization: "complete",
      status: "idle",
      content: "",
      entries: [],
    };
    let finishRefresh: ((response: Response) => void) | undefined;
    const response = () => new Response(JSON.stringify(memory), { headers: { "content-type": "application/json" } });
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(response())
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            finishRefresh = resolve;
          }),
      );
    vi.stubGlobal("fetch", fetchMock);
    render(<Harness item={{ ...agent, memory_supported: true }} />);
    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const document = await screen.findByRole("region", { name: "agentMemoryDocumentLabel" });
    expect(screen.queryByRole("button", { name: "agentMemoryRefresh" })).not.toBeInTheDocument();
    await act(async () => window.dispatchEvent(new Event("focus")));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("region", { name: "agentMemoryDocumentLabel" })).toBe(document);
    expect(within(document).getByRole("heading", { name: "agentMemoryPreference 0" })).toBeVisible();
    expect(screen.queryByText("agentMemoryLoading")).not.toBeInTheDocument();
    await act(async () => finishRefresh?.(response()));
    expect(screen.getByRole("region", { name: "agentMemoryDocumentLabel" })).toBe(document);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    for (const [, options] of fetchMock.mock.calls) expect(options?.method).not.toBe("PUT");
  });

  it("confirms reset before one atomic request and places editing first", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn<typeof fetch>(
      async (_url, options) =>
        new Response(
          JSON.stringify({
            available: true,
            enabled: true,
            ready: true,
            name: "MEMORY.md",
            revision: options?.method === "PUT" ? 5 : 4,
            status: options?.method === "PUT" ? "pending" : "idle",
            initialization: options?.method === "PUT" ? "pending" : "complete",
            content: "",
            entries: [],
          }),
          { headers: { "content-type": "application/json" } },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<Harness item={{ ...agent, memory_supported: true }} />);
    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const reset = await screen.findByRole("button", { name: "agentMemoryReset" });
    expect(reset).toHaveClass(buttonStyles["btn-secondary-gray"]);
    expect(screen.getByRole("button", { name: "agentMemoryClear" })).toHaveClass(buttonStyles["btn-secondary-gray"]);
    const edit = screen.getByRole("button", { name: "agentMemoryEdit" });
    expect(edit.compareDocumentPosition(reset) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await user.click(reset);
    let dialog = screen.getByRole("dialog", { name: "agentMemoryResetTitle" });
    expect(within(dialog).getByText("agentMemoryResetConfirm")).toBeVisible();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await user.click(within(dialog).getByRole("button", { name: "cancel" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await user.click(reset);
    dialog = screen.getByRole("dialog", { name: "agentMemoryResetTitle" });
    await user.click(within(dialog).getByRole("button", { name: "agentMemoryReset" }));
    const learning = await screen.findByText("agentMemoryLearning");
    expect(learning.closest(".agent-memory-document-section")).toBeInTheDocument();
    expect(learning.nextElementSibling).toBe(
      screen.getByRole("region", { name: "agentMemoryDocumentLabel" }).parentElement,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenLastCalledWith(
      "api/v1/agents/agent-1/memory",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ revision: 4, reset: true }) }),
    );
    expect(fetchMock.mock.calls.filter(([, options]) => options?.method === "PUT")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "save" })).not.toBeInTheDocument();
  });

  it.each([false, true])(
    "confirms clearing, saves immediately, and keeps failures in the dialog (%s)",
    async (failure) => {
      const user = userEvent.setup();
      let cleared = false;
      const fetchMock = vi.fn<typeof fetch>(async (_url, options) => {
        if (options?.method === "PUT") {
          if (failure) throw new Error("clear request failed");
          cleared = true;
        }
        return new Response(
          JSON.stringify({
            available: true,
            enabled: true,
            ready: true,
            name: "MEMORY.md",
            revision: cleared ? 5 : 4,
            status: "idle",
            initialization: "complete",
            content: cleared ? "" : "Remember this",
            entries: cleared ? [] : [{ id: "keep", kind: "fact", text: "Remember this" }],
          }),
          { headers: { "content-type": "application/json" } },
        );
      });
      vi.stubGlobal("fetch", fetchMock);
      render(<Harness item={{ ...agent, memory_supported: true }} />);
      await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
      await user.click(await screen.findByRole("button", { name: "agentMemoryClear" }));
      const dialog = screen.getByRole("dialog", { name: "agentMemoryClearTitle" });
      expect(within(dialog).getByText("agentMemoryClearConfirm")).toBeVisible();
      expect(fetchMock).toHaveBeenCalledTimes(1);
      await user.click(within(dialog).getByRole("button", { name: "agentMemoryClear" }));
      if (failure) {
        expect(await within(dialog).findByText("clear request failed")).toBeVisible();
        expect(dialog).toBeInTheDocument();
      } else {
        await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
        expect(screen.queryByText("Remember this")).not.toBeInTheDocument();
      }
      expect(fetchMock).toHaveBeenLastCalledWith(
        "api/v1/agents/agent-1/memory",
        expect.objectContaining({ method: "PUT", body: JSON.stringify({ revision: 4, entries: [] }) }),
      );
      expect(screen.queryByRole("button", { name: "save" })).not.toBeInTheDocument();
    },
  );

  it("keeps readable memory when an automatic refresh fails", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi
        .fn<typeof fetch>()
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              available: true,
              enabled: true,
              ready: true,
              name: "MEMORY.md",
              revision: 4,
              initialization: "complete",
              status: "idle",
              content: "Keep this memory",
              entries: [{ id: "remembered", kind: "preference", text: "Keep this memory" }],
            }),
            { headers: { "content-type": "application/json" } },
          ),
        )
        .mockRejectedValueOnce(new Error("network unavailable")),
    );
    render(<Harness item={{ ...agent, memory_supported: true }} />);
    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const document = await screen.findByRole("region", { name: "agentMemoryDocumentLabel" });
    await act(async () => window.dispatchEvent(new Event("focus")));
    expect(screen.queryByText("network unavailable")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "agentMemoryDocumentLabel" })).toBe(document);
    expect(within(document).getByText("Keep this memory")).toBeVisible();
  });

  it("does not let an older refresh overwrite a Markdown save", async () => {
    const user = userEvent.setup();
    const original = {
      available: true,
      enabled: true,
      ready: true,
      name: "MEMORY.md",
      revision: 4,
      initialization: "complete",
      status: "idle",
      content: "",
      entries: [],
    };
    const updated = {
      ...original,
      revision: 5,
      content: "Saved memory",
      entries: [{ id: "saved", kind: "fact", text: "Saved memory" }],
    };
    const response = (value: unknown) =>
      new Response(JSON.stringify(value), { headers: { "content-type": "application/json" } });
    let finishRefresh: ((response: Response) => void) | undefined;
    let gets = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (_input, options) => {
        if (options?.method === "PUT") return response(updated);
        if (++gets === 1) return response(original);
        return new Promise<Response>((resolve) => {
          finishRefresh = resolve;
        });
      }),
    );
    render(<Harness item={{ ...agent, memory_supported: true }} />);
    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const edit = await screen.findByRole("button", { name: "agentMemoryEdit" });
    await act(async () => window.dispatchEvent(new Event("focus")));
    await waitFor(() => expect(gets).toBe(2));
    await user.click(edit);
    await user.clear(screen.getByRole("textbox", { name: "agentMemoryMarkdownLabel" }));
    await user.type(screen.getByRole("textbox", { name: "agentMemoryMarkdownLabel" }), "Saved memory");
    expect(screen.getByRole("textbox", { name: "agentMemoryMarkdownLabel" })).toHaveValue("Saved memory");
    await user.click(screen.getByRole("button", { name: "save" }));
    await screen.findByText("Saved memory");
    await act(async () => finishRefresh?.(response(original)));
    expect(screen.getByText("Saved memory")).toBeVisible();
    expect(screen.getByRole("button", { name: "agentMemoryEdit" })).toBeEnabled();
  });

  it("shows the Agent memory tab, readable document, and enable toggle", async () => {
    const user = userEvent.setup();
    const onMemoryChange = vi.fn();
    const fetchMock = vi.fn<typeof fetch>(async (_input, init) => {
      const enabled = init?.method === "PUT" ? false : true;
      return new Response(
        JSON.stringify({
          enabled,
          ready: true,
          name: "MEMORY.md",
          available: true,
          revision: 7,
          entries: [{ id: "m1", kind: "preference", text: "Remember **this**.\n\n<script>unsafe()</script>" }],
          content: "# Durable memory\n\nRemember **this**.\n\n<script>unsafe()</script>\n",
        }),
        { headers: { "content-type": "application/json" }, status: 200 },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<Harness item={{ ...agent, memory_supported: true }} onMemoryChange={onMemoryChange} />);

    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const document = await screen.findByRole("region", { name: "agentMemoryDocumentLabel" });
    expect(within(document).getByRole("heading", { name: "agentMemoryPreference 1" })).toBeInTheDocument();
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
    expect(screen.queryByRole("button", { name: "agentMemoryRefresh" })).not.toBeInTheDocument();
    expect(screen.queryByText("agentMemoryToggleHint")).not.toBeInTheDocument();

    const toggle = screen.getByRole("switch", { name: "agentMemoryEnabled" });
    expect(toggle).toBeChecked();
    expect(toggle.closest(".agent-memory-setting-actions")).toBeInTheDocument();
    expect(toggle.closest(".agent-memory-section-heading")).toBeInTheDocument();
    await user.click(toggle);

    await waitFor(() => expect(toggle).not.toBeChecked());
    expect(onMemoryChange).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenLastCalledWith(
      "api/v1/agents/agent-1/memory",
      expect.objectContaining({ body: JSON.stringify({ revision: 7, enabled: false }), method: "PUT" }),
    );
  });

  it("hides the memory tab when the runtime does not expose the capability", () => {
    render(<Harness item={{ ...agent, memory_supported: false }} />);

    expect(screen.queryByRole("button", { name: "agentMemoryTab" })).not.toBeInTheDocument();
  });

  it("shows all three categories without separate add or classification actions", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(
        async () =>
          new Response(
            JSON.stringify({
              enabled: true,
              ready: false,
              name: "MEMORY.md",
              available: true,
              revision: 0,
              entries: [],
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
    const document = await screen.findByRole("region", { name: "agentMemoryDocumentLabel" });
    for (const label of ["agentMemoryPreference", "agentMemoryConvention", "agentMemoryFact"]) {
      expect(within(document).getByRole("heading", { name: `${label} 0` })).toBeInTheDocument();
      expect(within(document).queryByRole("button", { name: `agentMemoryAdd · ${label}` })).not.toBeInTheDocument();
    }
  });

  it.each([
    { entries: [] },
    { entries: [{ id: "fact-only", kind: "fact", text: "Existing **fact**.\nKeep the next line." }] },
  ])("prefills all three Markdown headings and retains existing memory (%j)", async ({ entries }) => {
    const user = userEvent.setup();
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(
          JSON.stringify({
            available: true,
            enabled: true,
            ready: entries.length > 0,
            name: "MEMORY.md",
            revision: 4,
            content: "# Agent memory\n\n## Confirmed facts\n",
            entries,
          }),
          { headers: { "content-type": "application/json" } },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<Harness item={{ ...agent, memory_supported: true }} />);
    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    await user.click(await screen.findByRole("button", { name: "agentMemoryEdit" }));
    const textarea = screen.getByRole("textbox", { name: "agentMemoryMarkdownLabel" }) as HTMLTextAreaElement;
    for (const category of ["agentMemoryPreference", "agentMemoryConvention", "agentMemoryFact"])
      expect(textarea.value).toContain(`## ${category}\n`);
    if (entries.length) expect(textarea.value).toContain("- Existing **fact**.\n  Keep the next line.");
    expect(screen.queryAllByText(/^## agentMemory/).filter((element) => element.tagName !== "TEXTAREA")).toHaveLength(
      0,
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: "cancel" }));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("shows the latest persisted error and accepts a complete Markdown document", async () => {
    const user = userEvent.setup();
    const calls: Array<Record<string, unknown>> = [];
    let document = {
      available: true,
      enabled: true,
      ready: false,
      revision: 4,
      entries: [],
      name: "MEMORY.md",
      content: "",
      status: "failed",
      initialization: "failed",
      last_error: { message: "HTTP 429: rate limit exceeded", operation: "initialization", at: "2026-11-11T03:00:00Z" },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (_url, init) => {
        if (init?.method === "PUT") {
          const payload = JSON.parse(String(init.body));
          calls.push(payload);
          document = { ...document, content: payload.content, revision: 5, status: "idle" };
        }
        return new Response(JSON.stringify(document), { headers: { "content-type": "application/json" } });
      }),
    );
    render(<Harness item={{ ...agent, memory_supported: true }} />);
    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    expect(await screen.findByText("HTTP 429: rate limit exceeded")).toBeVisible();
    expect(screen.queryByRole("button", { name: "agentMemoryEditEntries" })).not.toBeInTheDocument();
    const description = screen.getByText("agentMemoryDocumentDescription");
    await user.click(screen.getByRole("button", { name: "agentMemoryEdit" }));
    expect(screen.queryByText("agentMemoryMarkdownHint")).not.toBeInTheDocument();
    expect(screen.queryByText("agentMemoryMarkdownFallback")).not.toBeInTheDocument();
    expect(screen.getByText("agentMemoryDocumentDescription")).toBe(description);
    for (const label of ["agentMemoryPreference", "agentMemoryConvention", "agentMemoryFact"])
      expect(
        (screen.getByRole("textbox", { name: "agentMemoryMarkdownLabel" }) as HTMLTextAreaElement).value,
      ).toContain(`## ${label}\n`);
    expect(screen.queryByRole("combobox", { name: /agentMemoryCategory/ })).not.toBeInTheDocument();
    const markdown =
      "# Agent memory\n\n## 偏好\n- 中文回复\n\n## 工作约定\n- 验证后合并\n\n## 已确认事实\n- 项目叫 example。";
    await user.clear(screen.getByRole("textbox", { name: "agentMemoryMarkdownLabel" }));
    await user.paste(markdown);
    await user.click(screen.getByRole("button", { name: "save" }));
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]).toEqual({ revision: 4, content: markdown });
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
    { runtimeKind: "codex", sandbox: false, supported: true },
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

it("omits the retired sandbox switch even when old native metadata contains it", () => {
  render(
    <Harness
      draft={{ ...savedDraft, sandbox_enabled: true }}
      item={{ ...agent, runtime: { ...agent.runtime, sandbox_enabled: true } }}
    />,
  );
  expect(screen.getByRole("textbox", { name: "profileRuntimeKind" })).toBeDisabled();
  expect(screen.queryByRole("checkbox", { name: "profileSandboxEnabled" })).not.toBeInTheDocument();
});
