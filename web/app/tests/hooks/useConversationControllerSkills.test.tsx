import { useState } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchAgentSkills, fetchAgentSkillsFile } from "@/api/agents";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, IMData, IMUser, TranslateFn } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";

vi.mock("@/api/agents", async () => {
  const actual = await vi.importActual<typeof import("@/api/agents")>("@/api/agents");
  return {
    ...actual,
    fetchAgentSkills: vi.fn(),
    fetchAgentSkillsFile: vi.fn(),
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

function useConversationControllerHarness() {
  const [data] = useState<IMData>({
    current_user_id: "u-admin",
    rooms: [directConversation],
    users,
  });
  const agents: AgentLike[] = [
    {
      id: "u-skill-worker",
      name: "skill-worker",
      role: "worker",
      avatar: "SW",
      runtime_kind: "codex",
      status: "running",
    },
  ];

  return useConversationController({
    activeConversationId: directConversation.id,
    activePane: { type: WorkspacePaneTypes.conversation, id: directConversation.id },
    agents,
    authBusyProvider: "",
    authStatuses: {},
    data,
    locale: "en",
    managerProfile: null,
    managerProfileIncomplete: false,
    messageActionBusy: "",
    messageActionFeedback: { key: "", message: "" },
    navigatePane: vi.fn(),
    onMessageAction: vi.fn(),
    onProviderLogin: vi.fn(),
    rooms: [directConversation],
    selectComputer: vi.fn(),
    selectConversation: vi.fn(),
    setActiveConversationId: vi.fn(),
    setBootstrapData: vi.fn(),
    setShowToolCalls: vi.fn(),
    showToolCalls: false,
    t,
    theme: "light",
  });
}

describe("useConversationController skill loading", () => {
  beforeEach(() => {
    vi.mocked(fetchAgentSkills).mockReset();
    vi.mocked(fetchAgentSkillsFile).mockReset();
  });

  it("loads slash skills from the dedicated skills API", async () => {
    vi.mocked(fetchAgentSkills).mockResolvedValue({
      entries: [
        { path: "reviewer", name: "reviewer", type: "dir" },
        { path: "reviewer/SKILL.md", name: "SKILL.md", type: "file" },
      ],
      kind: "dir",
      path: "",
    });
    vi.mocked(fetchAgentSkillsFile).mockResolvedValue({
      path: "reviewer/SKILL.md",
      content: '---\ndescription: "Review pull requests"\n---\n# reviewer',
    });

    const { result } = renderHook(() => useConversationControllerHarness());
    const editor = document.createElement("div");
    editor.textContent = "/";

    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledWith("u-skill-worker"));
    await waitFor(() => expect(fetchAgentSkillsFile).toHaveBeenCalledWith("u-skill-worker", "reviewer/SKILL.md"));

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
  });
});
