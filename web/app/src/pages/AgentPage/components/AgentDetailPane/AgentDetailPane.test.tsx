// @vitest-environment jsdom

import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useState } from "react";
import type { Ref } from "react";
import { describe, expect, it, vi } from "vitest";
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
