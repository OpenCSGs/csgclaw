import { useRef, useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationComposer } from "@/components/business/ConversationPane/ConversationComposer";
import { ConversationThreadPanel } from "@/components/business/ConversationPane/ConversationThreadPanel";
import { useConversationDraftEditorSync } from "@/components/business/ConversationPane/useConversationDraftEditorSync";
import { MessageContent } from "@/components/business/MessageContent";
import { parseComposerSegments, type ComposerSegment } from "@/models/composer";
import { addResources, resourceMessageInput, type ResourceSnapshot } from "@/models/conversationResources";
import { createQueryWrapper } from "../helpers/queryClient";

const resources: ResourceSnapshot[] = [
  { source: "skill", id: "review", type: "skill", name: "Review" },
  { source: "app", id: "gitlab", type: "connector", name: "GitLab" },
  { source: "mcp", id: "guidelines", type: "knowledge_base", name: "工程规范" },
];
const initial = addResources([{ type: "text", text: "检查项目" }], resources, "agent");

function setup(thread: boolean, disabled = false) {
  const onSend = vi.fn();
  const onKeyDown = vi.fn();
  function Harness({ removeLabel }: { removeLabel: string }) {
    const [draft, setDraft] = useState<ComposerSegment[]>(initial);
    const editorRef = useRef<HTMLDivElement>(null);
    const t = (key: string) => (key === "remove" ? removeLabel : key);
    useConversationDraftEditorSync(editorRef, draft, removeLabel);
    return (
      <>
        {thread ? (
          <ConversationThreadPanel
            disabled={disabled}
            draftSegments={draft}
            error=""
            loading={false}
            locale="zh"
            onClose={() => {}}
            onDraftChange={setDraft}
            onPreviewUser={() => {}}
            onSend={onSend}
            showToolCalls={false}
            t={t}
            theme="light"
            usersById={new Map()}
            thread={{
              room_id: "room",
              root: { id: "root", content: "Task", sender_id: "user", created_at: "2026-10-15T00:00:00Z" },
              replies: [],
              summary: { root_id: "root", reply_count: 0 },
            }}
          />
        ) : (
          <ConversationComposer
            authBusyProvider=""
            authStatuses={{}}
            composerDisabled={disabled}
            composerError=""
            draftSegments={draft}
            draftText="检查项目"
            editorRef={editorRef}
            managerProvider=""
            mentionCandidates={[]}
            mentionIndex={0}
            mentionableUsersByName={new Map()}
            slashCandidates={[]}
            slashIndex={0}
            slashPickerLoading={false}
            slashPickerOpen={false}
            t={t}
            onApplyMention={() => {}}
            onApplySlashCandidate={() => {}}
            onComposerCompositionEnd={() => {}}
            onComposerCompositionStart={() => {}}
            onComposerKeyDown={onKeyDown}
            onProviderLogin={() => {}}
            onSendMessage={onSend}
            onSyncComposer={() => setDraft(parseComposerSegments(editorRef.current))}
          />
        )}
        <output data-testid="payload">{JSON.stringify(resourceMessageInput(draft))}</output>
      </>
    );
  }
  const view = render(<Harness removeLabel="Remove" />, { wrapper: createQueryWrapper().wrapper });
  return {
    ...view,
    onSend,
    onKeyDown,
    setRemoveLabel: (label: string) => view.rerender(<Harness removeLabel={label} />),
  };
}

describe.each([false, true])("resource tags in the composer (thread=%s)", (thread) => {
  it.each(["click", "{Enter}", " "])("removes one resource with %s and preserves the message", async (action) => {
    const user = userEvent.setup();
    const { onSend, onKeyDown } = setup(thread);
    const editor = screen.getByRole("textbox");
    const remove = within(editor).getByRole("button", { name: "Remove GitLab" });
    if (action === "click") await user.click(remove);
    else {
      remove.focus();
      await user.keyboard(action);
    }
    expect(JSON.parse(screen.getByTestId("payload").textContent!)).toEqual({
      content: "检查项目",
      resource_refs: [
        { source: "skill", id: "review" },
        { source: "mcp", id: "guidelines" },
      ],
    });
    expect(within(editor).queryByRole("button", { name: "Remove GitLab" })).not.toBeInTheDocument();
    expect(editor).toHaveFocus();
    expect(onSend).not.toHaveBeenCalled();
    expect(onKeyDown).not.toHaveBeenCalled();
  });

  it("keeps the draft and caret when removal labels change language", () => {
    const { setRemoveLabel } = setup(thread);
    const editor = screen.getByRole("textbox");
    const token = editor.querySelector('[data-resource-type="skill"]');
    const text = editor.lastChild!;
    const caret = document.createRange();
    caret.setStart(text, 1);
    caret.collapse(true);
    editor.focus();
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(caret);
    setRemoveLabel("移除");
    expect(within(editor).getByRole("button", { name: "移除 /Review" })).toBeInTheDocument();
    expect(editor.querySelector('[data-resource-type="skill"]')).toBe(token);
    expect(selection.anchorNode).toBe(text);
    expect(selection.anchorOffset).toBe(1);
    expect(JSON.parse(screen.getByTestId("payload").textContent!).resource_refs).toHaveLength(3);
  });

  it("keeps resources in a disabled composer", () => {
    setup(thread, true);
    const editor = screen.getByRole("textbox");
    fireEvent.click(editor.querySelector("[data-composer-resource-remove]")!);
    expect(JSON.parse(screen.getByTestId("payload").textContent!).resource_refs).toHaveLength(3);
  });
});

it("shows the same resource types in history without removal controls", () => {
  const { container } = render(
    <MessageContent
      content="检查项目"
      message={{ metadata: { csgclaw: { resource_selection: { version: 1, agent_id: "agent", refs: resources } } } }}
    />,
  );
  expect([...container.querySelectorAll(".resource-tag")].map((tag) => tag.getAttribute("data-resource-type"))).toEqual(
    ["skill", "connector", "knowledge_base"],
  );
  expect(screen.queryByRole("button", { name: /Remove/ })).not.toBeInTheDocument();
});
