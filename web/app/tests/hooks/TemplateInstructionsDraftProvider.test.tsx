import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TemplateInstructionsDraftProvider } from "@/components/business/HubDetailPane/TemplateInstructionsDraftProvider";
import { useTemplateInstructionsEditor } from "@/components/business/HubDetailPane/useTemplateInstructionsEditor";
import { replaceManagedAgentInstructions } from "@/models/templateInstructions";
import { act } from "react";

const source = replaceManagedAgentInstructions("# Preface\n", "Original");
function Editor({
  id,
  content = source,
  save = async () => true,
}: {
  id: string;
  content?: string | null;
  save?: (value: string) => Promise<boolean>;
}) {
  const editor = useTemplateInstructionsEditor({ templateId: id, content, blocked: false, onSave: save });
  return (
    <>
      <textarea
        aria-label="Instructions"
        value={editor.value}
        onChange={(event) => editor.change(event.target.value)}
      />
      <button onClick={() => editor.setMode(editor.mode === "default" ? "advanced" : "default")}>Switch mode</button>
      <button disabled={!editor.canSave} onClick={() => void editor.save()}>
        Save
      </button>
      <button onClick={editor.reload}>Discard</button>
      <output>
        {editor.conflict ? "Conflict" : editor.failed ? "Failed" : editor.dirty ? "Unsaved" : "Unchanged"}
      </output>
    </>
  );
}
function beforeUnload() {
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

it("preserves per-template drafts and unload protection after the detail pane unmounts", async () => {
  const user = userEvent.setup();
  const view = render(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.first" />
    </TemplateInstructionsDraftProvider>,
  );
  expect(beforeUnload()).toBe(false);
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "First draft" } });
  await user.click(screen.getByRole("button", { name: "Switch mode" }));
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <p>My agents</p>
    </TemplateInstructionsDraftProvider>,
  );
  expect(beforeUnload()).toBe(true);
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.second" />
    </TemplateInstructionsDraftProvider>,
  );
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Second draft" } });
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.first" content={null} />
    </TemplateInstructionsDraftProvider>,
  );
  expect(screen.getByRole("textbox")).toHaveValue(replaceManagedAgentInstructions(source, "First draft"));
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.first" />
    </TemplateInstructionsDraftProvider>,
  );
  expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "Switch mode" }));
  expect(screen.getByRole("textbox")).toHaveValue("First draft");
  await user.click(screen.getByRole("button", { name: "Discard" }));
  expect(beforeUnload()).toBe(true); // The other template still has pending work.
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.second" />
    </TemplateInstructionsDraftProvider>,
  );
  expect(screen.getByRole("textbox")).toHaveValue("Second draft");
  await user.click(screen.getByRole("button", { name: "Discard" }));
  expect(beforeUnload()).toBe(false);
});

it("preserves a returning draft on source conflicts until explicitly discarded", async () => {
  const user = userEvent.setup();
  const view = render(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.first" />
    </TemplateInstructionsDraftProvider>,
  );
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "My draft" } });
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <p>Another route</p>
    </TemplateInstructionsDraftProvider>,
  );
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.first" content={replaceManagedAgentInstructions(source, "Remote update")} />
    </TemplateInstructionsDraftProvider>,
  );
  expect(screen.getByRole("textbox")).toHaveValue("My draft");
  expect(screen.getByRole("status")).toHaveTextContent("Conflict");
  expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Discard" }));
  expect(screen.getByRole("textbox")).toHaveValue("Remote update");
  expect(beforeUnload()).toBe(false);
});

it.each([true, false])("settles a save while the editor is absent (success: %s)", async (success) => {
  const user = userEvent.setup();
  let finish!: (value: boolean) => void;
  const save = vi.fn<(value: string) => Promise<boolean>>(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  const view = render(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.first" save={save} />
    </TemplateInstructionsDraftProvider>,
  );
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Pending save" } });
  await user.click(screen.getByRole("button", { name: "Save" }));
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <p>Another route</p>
    </TemplateInstructionsDraftProvider>,
  );
  expect(beforeUnload()).toBe(true);
  await act(async () => {
    finish(success);
  });
  const currentSource = success ? save.mock.calls[0][0] : source;
  view.rerender(
    <TemplateInstructionsDraftProvider>
      <Editor id="local.first" content={currentSource} save={save} />
    </TemplateInstructionsDraftProvider>,
  );
  expect(screen.getByRole("textbox")).toHaveValue("Pending save");
  if (success) expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  else {
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    expect(screen.getByRole("status")).toHaveTextContent("Failed");
  }
  expect(beforeUnload()).toBe(!success);
  expect(save).toHaveBeenCalledTimes(1);
});
