import { createQueryWrapper } from "../helpers/queryClient";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { useState } from "react";
import { act, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fetchAgentSkillSummaries } from "@/api/agents";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, IMData, IMUser, TranslateFn } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";
import { FloatingChat } from "@/pages/WorkspacePage/components/FloatingChat";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/agents", async () => {
  const actual = await vi.importActual<typeof import("@/api/agents")>("@/api/agents");
  return {
    ...actual,
    fetchAgentSkillSummaries: vi.fn(),
  };
});

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: () => () => {},
}));

const t: TranslateFn = (key) => key;

const users: IMUser[] = [
  { id: "u-admin", name: "admin", role: "admin", avatar: "AD", accent_hex: "#dc2626" },
  {
    id: "u-skill-worker",
    name: "skill-worker",
    role: "worker",
    avatar: "SW",
    accent_hex: "#2563eb",
  },
];

const directConversation: IMConversation = {
  id: "room-skill",
  is_direct: true,
  members: ["u-admin", "u-skill-worker"],
  messages: [],
  title: "skill-worker",
};

type SkillHarnessFixture = {
  agent: AgentLike;
  conversation: IMConversation;
  users: IMUser[];
};

const defaultFixture: SkillHarnessFixture = {
  agent: {
    id: "u-skill-worker",
    name: "skill-worker",
    role: "worker",
    avatar: "SW",
    runtime_kind: "codex",
    status: "running",
  },
  conversation: directConversation,
  users,
};

const floatingFixture: SkillHarnessFixture = {
  agent: {
    id: "u-floating-skill-worker",
    name: "floating-skill-worker",
    role: "worker",
    avatar: "FS",
    runtime_kind: "codex",
    status: "running",
  },
  conversation: {
    id: "room-floating-skill",
    is_direct: true,
    members: ["u-admin", "u-floating-skill-worker"],
    messages: [],
    title: "floating-skill-worker",
  },
  users: [
    users[0],
    {
      id: "u-floating-skill-worker",
      name: "floating-skill-worker",
      role: "worker",
      avatar: "FS",
      accent_hex: "#2563eb",
    },
  ],
};

function useConversationControllerHarness(fixture: SkillHarnessFixture = defaultFixture) {
  const [data] = useState<IMData>({
    current_user_id: "u-admin",
    rooms: [fixture.conversation],
    users: fixture.users,
  });

  return useConversationController({
    activeConversationId: fixture.conversation.id,
    activePane: { type: WorkspacePaneTypes.conversation, id: fixture.conversation.id },
    agents: [fixture.agent],
    authBusyProvider: "",
    authStatuses: {},
    data,
    locale: "en",
    managerProfile: null,
    managerProfileIncomplete: false,
    hasObservedWorkLease: () => false,
    messageActionBusy: "",
    messageActionFeedback: { key: "", message: "" },
    navigatePane: vi.fn(),
    onMessageAction: vi.fn(),
    onProviderLogin: vi.fn(),
    openCSGAuthGuard: openCSGAuthGuardStub(),
    rooms: [fixture.conversation],
    selectComputer: vi.fn(),
    selectConversation: vi.fn(),
    setActiveConversationId: vi.fn(),
    setBootstrapData: vi.fn(),
    setShowToolCalls: vi.fn(),
    showToolCalls: false,
    t,
    theme: "light",
    workingParticipantsForRoom: () => [],
  });
}

describe("useConversationController skill loading", () => {
  beforeEach(() => {
    vi.mocked(fetchAgentSkillSummaries).mockReset();
  });

  it("loads slash skills from the dedicated skills API", async () => {
    vi.mocked(fetchAgentSkillSummaries).mockResolvedValue([
      { name: "reviewer", description: "Review pull requests", enabled: true },
      { name: "disabled", description: "Hidden skill", enabled: false },
    ]);
    const { result } = renderHook(() => useConversationControllerHarness(), { wrapper: createQueryWrapper().wrapper });
    const editor = document.createElement("div");
    editor.textContent = "/";
    document.body.append(editor);
    const range = document.createRange();
    range.setStart(editor.firstChild || editor, 1);
    range.collapse(true);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);

    await waitFor(() => expect(fetchAgentSkillSummaries).toHaveBeenCalled());
    await act(async () => {
      (result.current.conversationViewProps.editorRef as { current: HTMLDivElement | null }).current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });

    await waitFor(() =>
      expect(result.current.conversationViewProps.slashCandidates).toContainEqual({
        name: "reviewer",
        description: "Review pull requests",
        type: "skill",
      }),
    );

    expect(result.current.conversationViewProps.slashCandidates.some((item) => item.name === "disabled")).toBe(false);
    act(() => {
      result.current.conversationViewProps.onApplySlashCandidate("reviewer");
    });

    expect(editor.querySelector('[data-composer-slash-token="true"]')).toHaveTextContent("/reviewer");
    expect(result.current.conversationViewProps.draftSegments).toEqual([
      { type: "slash", text: "/reviewer" },
      { type: "text", text: " " },
    ]);
    editor.remove();
  });

  it("refreshes added skills in floating chat and navigates them with Ctrl+N and Ctrl+P", async () => {
    const user = userEvent.setup();
    vi.mocked(fetchAgentSkillSummaries)
      .mockResolvedValueOnce([])
      .mockResolvedValue([
        { name: "alpha", description: "Alpha skill", enabled: true },
        { name: "beta", description: "Beta skill", enabled: true },
      ]);
    const query = createQueryWrapper();
    function Harness() {
      const controller = useConversationControllerHarness(floatingFixture);
      return (
        <FloatingChat
          avatarFallback="FS"
          chatProps={{
            ...controller.conversationViewProps,
            conversation: floatingFixture.conversation,
            onPreviewUser: () => {},
          }}
          locale="en"
          open={true}
          t={t}
          title="floating-skill-worker"
          onOpenChange={() => {}}
        />
      );
    }

    render(<Harness />, { wrapper: query.wrapper });
    await waitFor(() => expect(fetchAgentSkillSummaries).toHaveBeenCalledTimes(1));
    await act(async () => {
      await query.client.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(floatingFixture.agent.id!) });
    });

    const editor = screen.getByLabelText("floatingChatInputPlaceholder");
    await user.click(editor);
    await user.type(editor, "/");

    await waitFor(() => expect(fetchAgentSkillSummaries).toHaveBeenCalledTimes(2));
    const picker = screen.getByRole("listbox");
    const alpha = within(picker).getByRole("option", { name: /alpha/i });
    const beta = within(picker).getByRole("option", { name: /beta/i });
    expect(alpha).toHaveAttribute("aria-selected", "false");
    expect(beta).toHaveAttribute("aria-selected", "false");

    await user.keyboard("{Control>}n{/Control}");
    expect(alpha).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{Control>}n{/Control}");
    expect(beta).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{Control>}p{/Control}");
    expect(alpha).toHaveAttribute("aria-selected", "true");
  });

  it("opens the slash picker at the caret before existing draft text", async () => {
    const user = userEvent.setup();
    vi.mocked(fetchAgentSkillSummaries).mockResolvedValue([]);

    function Harness() {
      const controller = useConversationControllerHarness(floatingFixture);
      return (
        <FloatingChat
          avatarFallback="FS"
          chatProps={{
            ...controller.conversationViewProps,
            conversation: floatingFixture.conversation,
            onPreviewUser: () => {},
          }}
          locale="en"
          open={true}
          t={t}
          title="floating-skill-worker"
          onOpenChange={() => {}}
        />
      );
    }

    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    const editor = screen.getByLabelText("floatingChatInputPlaceholder");
    await user.type(editor, "existing prompt");

    const existingText = editor.firstChild;
    expect(existingText?.nodeType).toBe(Node.TEXT_NODE);
    editor.focus();
    const range = document.createRange();
    range.setStart(existingText || editor, 0);
    range.collapse(true);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    await user.keyboard("/");

    const picker = await screen.findByRole("listbox");
    const newConversation = within(picker).getByRole("option", { name: /new/i });
    expect(newConversation).toBeInTheDocument();
    expect(editor).toHaveTextContent("/existing prompt");

    await user.click(newConversation);
    expect(editor.querySelector('[data-composer-slash-token="true"]')).toHaveTextContent("/new");
    expect(editor).toHaveTextContent("/new existing prompt");
  });
});
