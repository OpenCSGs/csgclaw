// @vitest-environment jsdom

import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useState } from "react";
import type { Ref } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentDraft, AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { AgentDetailPane } from "./AgentDetailPane";
import type { AgentDetailPaneHandle, AgentDetailPaneProps } from "./AgentDetailPane";

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
          content: "# Durable memory\n\nRemember this.\n",
        }),
        { headers: { "content-type": "application/json" }, status: 200 },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<Harness item={{ ...agent, memory_supported: true }} onMemoryChange={onMemoryChange} />);

    await user.click(screen.getByRole("button", { name: "agentMemoryTab" }));
    const document = await screen.findByRole("textbox", { name: "agentMemoryDocumentLabel" });
    expect(document).toHaveValue("# Durable memory\n\nRemember this.\n");
    expect(document.closest(".agent-memory-document-shell")).toHaveClass("agent-section-form");
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
    expect(screen.queryByRole("textbox", { name: "agentMemoryDocumentLabel" })).not.toBeInTheDocument();
  });
});
