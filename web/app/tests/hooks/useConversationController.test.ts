import { useState } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import {
  normalizeSlashShorthandForPayload,
  slashSkillCommandText,
  slashSkillInputText,
  slashSkillQueryForDraft,
  useConversationController,
} from "@/hooks/workspace/useConversationController";
import type { IMData, LocaleCode, TranslateFn } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";

const t: TranslateFn = (key) =>
  ({
    sendFailed: "Failed to send the message. Please retry.",
  })[key] ?? key;

const testData: IMData = {
  current_user_id: "u-admin",
  rooms: [
    {
      id: "room-1",
      is_direct: true,
      members: ["u-admin", "u-manager"],
      messages: [],
      title: "manager",
    },
  ],
  users: [
    {
      accent_hex: "#8b1d2c",
      avatar: "AD",
      handle: "admin",
      id: "u-admin",
      name: "Admin",
      role: "admin",
    },
    {
      accent_hex: "#0f5b66",
      avatar: "MG",
      handle: "manager",
      id: "u-manager",
      name: "manager",
      role: "worker",
    },
  ],
};

function useConversationControllerTestHarness() {
  const [data, setData] = useState<IMData | null>(testData);
  return useConversationController({
    activeConversationId: "room-1",
    activePane: { type: WorkspacePaneTypes.conversation, id: "room-1" },
    agents: [],
    authBusyProvider: "",
    authStatuses: {},
    data,
    locale: "en" as LocaleCode,
    managerProfile: null,
    managerProfileIncomplete: false,
    messageActionBusy: "",
    messageActionError: {},
    navigatePane: () => {},
    onMessageAction: () => {},
    onProviderLogin: async () => {},
    onUpgradeStatusChange: () => {},
    rooms: data?.rooms ?? [],
    selectComputer: () => {},
    selectConversation: () => {},
    setActiveConversationId: () => {},
    setBootstrapData: (value) => {
      setData((current) => (typeof value === "function" ? value(current) : value));
    },
    setShowToolCalls: () => {},
    showToolCalls: false,
    t,
    theme: "light",
  });
}

describe("useConversationController slash skill helpers", () => {
  it("keeps the skill picker open only while editing the command name", () => {
    expect(slashSkillQueryForDraft("/")).toBe("");
    expect(slashSkillQueryForDraft("  /sk")).toBe("sk");
    expect(slashSkillQueryForDraft("/skill-creator ")).toBeNull();
    expect(slashSkillQueryForDraft("/skill-creator make a skill")).toBeNull();
    expect(slashSkillQueryForDraft("hello /skill-creator")).toBeNull();
  });

  it("renders selected skills as canonical slash-command XML", () => {
    expect(slashSkillCommandText("skill-creator")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command>',
    );
    expect(slashSkillCommandText('a&b"c<d>')).toBe(
      '<slash-command name="use-skill" arg="a&amp;b&quot;c&lt;d&gt;"></slash-command>',
    );
  });

  it("renders skill input as /slug command text", () => {
    expect(slashSkillInputText("skill-creator")).toBe("/skill-creator ");
    expect(slashSkillInputText(" manager-worker-dispatch ")).toBe("/manager-worker-dispatch ");
  });

  it("normalizes slash-command shorthand into canonical XML before send", () => {
    expect(normalizeSlashShorthandForPayload("/skill-creator")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command>',
    );
    expect(normalizeSlashShorthandForPayload("/skill-creator build a review")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command> build a review',
    );
    expect(normalizeSlashShorthandForPayload("  /skill-creator   build a review  ")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command> build a review',
    );
  });

  it("does not normalize invalid shorthand payloads", () => {
    expect(normalizeSlashShorthandForPayload("/skill/creator")).toBe("/skill/creator");
    expect(normalizeSlashShorthandForPayload("/")).toBe("/");
    expect(normalizeSlashShorthandForPayload("hello /skill-creator")).toBe("hello /skill-creator");
    expect(normalizeSlashShorthandForPayload("//skill-creator")).toBe("//skill-creator");
  });
});

describe("useConversationController send errors", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the complete API error body when message send fails", async () => {
    const fullError =
      'Error processing message: LLM call failed after retries: API request failed:\n' +
      "Status: 500\n" +
      'Body: {"error":{"message":"Post \\"https://chatgpt.com/backend-api/codex/responses\\": EOF","type":"server_error","code":"internal_server_error","param":null}}';
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(fullError, { status: 500 }));
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      class {
        onmessage: ((event: MessageEvent) => void) | null = null;
        onerror: ((event: Event) => void) | null = null;
        close() {}
      },
    );

    const { result } = renderHook(() => useConversationControllerTestHarness());
    const editor = document.createElement("div");
    editor.textContent = "hello";

    await act(async () => {
      (result.current.conversationViewProps.editorRef as { current: HTMLDivElement | null }).current = editor;
      result.current.conversationViewProps.onSyncComposer();
    });
    await waitFor(() => expect(result.current.conversationViewProps.draftText).toBe("hello"));

    await act(async () => {
      await result.current.conversationViewProps.onSendMessage();
    });

    await waitFor(() => expect(result.current.conversationViewProps.composerError).toBe(fullError));
  });
});
