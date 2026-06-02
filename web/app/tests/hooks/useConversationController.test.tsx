import { renderHook, act } from "@testing-library/react";
import { vi } from "vitest";
import { useConversationController } from "@/hooks/workspace/useConversationController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { IMConversation, IMData, IMUser, TranslateFn } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: () => () => {},
}));

const t: TranslateFn = (key) => key;

const users: IMUser[] = [
  { id: "u-admin", name: "admin", handle: "admin", role: "admin", avatar: "AD", accent_hex: "#dc2626" },
  {
    id: "u-demo",
    name: "demo",
    handle: "demo",
    role: "worker",
    avatar: "avatar/cartoon-3.png",
    accent_hex: "#4f46e5",
  },
];

const directConversation: IMConversation = {
  id: "room-1",
  is_direct: true,
  members: ["u-admin", "u-demo"],
  messages: [],
  title: "demo",
};

function renderConversationController() {
  const agents: AgentLike[] = [
    {
      id: "u-demo",
      name: "demo",
      handle: "demo",
      role: "worker",
      avatar: "GI",
      runtime_kind: "picoclaw_sandbox",
      status: "running",
    },
  ];
  const data: IMData = {
    current_user_id: "u-admin",
    rooms: [directConversation],
    users,
  };

  return renderHook(() =>
    useConversationController({
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
      messageActionError: { key: "", message: "" },
      navigatePane: vi.fn(),
      onMessageAction: vi.fn(),
      onProviderLogin: vi.fn(),
      onUpgradeStatusChange: vi.fn(),
      rooms: [directConversation],
      selectComputer: vi.fn(),
      selectConversation: vi.fn(),
      setActiveConversationId: vi.fn(),
      setBootstrapData: vi.fn(),
      setShowToolCalls: vi.fn(),
      showToolCalls: false,
      t,
      theme: "light",
    }),
  );
}

describe("useConversationController", () => {
  it("opens create-room modal from a direct message", () => {
    const { result } = renderConversationController();

    expect(result.current.conversationViewProps.inviteActionLabel).toBe("createRoomFromDM");

    act(() => {
      result.current.conversationViewProps.onInviteAction();
    });

    expect(result.current.createRoomModalProps).toMatchObject({
      roomMemberIDs: ["u-admin", "u-demo"],
      lockedRoomMemberIDs: ["u-admin", "u-demo"],
    });
    expect(result.current.inviteMembersModalProps).toBeNull();
  });
});
