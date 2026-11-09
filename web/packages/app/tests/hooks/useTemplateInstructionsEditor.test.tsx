import { act, renderHook } from "@testing-library/react";
import { useTemplateInstructionsEditor } from "@/components/business/HubDetailPane/useTemplateInstructionsEditor";
import { replaceManagedAgentInstructions } from "@/models/templateInstructions";

const content = replaceManagedAgentInstructions("# Preface\n", "Original instructions");
function setup() {
  const onSave = vi.fn<(document: string) => boolean | Promise<boolean>>().mockResolvedValue(true);
  const props = { templateId: "local.first", content: content as string | null, blocked: false, onSave };
  return { onSave, props, ...renderHook(useTemplateInstructionsEditor, { initialProps: props }) };
}

it("only enables saving for a meaningful change and disables again on revert", () => {
  const { result } = setup();
  expect(result.current.canSave).toBe(false);
  act(() => result.current.change("Original instructions \n"));
  expect(result.current.value).toBe("Original instructions \n");
  expect(result.current.canSave).toBe(false);
  act(() => result.current.change("Changed instructions"));
  expect(result.current.canSave).toBe(true);
  act(() => result.current.change("Original instructions"));
  expect(result.current.canSave).toBe(false);
});

it("retains edits across modes and treats advanced whitespace edits as changes", () => {
  const { result } = setup();
  act(() => result.current.change("Two words\n\nNext paragraph"));
  act(() => result.current.setMode("advanced"));
  expect(result.current.value).toContain("Two words\n\nNext paragraph");
  act(() => result.current.setMode("default"));
  expect(result.current.value).toBe("Two words\n\nNext paragraph");
  act(() => result.current.change("Original instructions"));
  expect(result.current.canSave).toBe(false);
  act(() => result.current.setMode("advanced"));
  act(() => result.current.change(content + "\n"));
  expect(result.current.canSave).toBe(true);
  act(() => result.current.change(content));
  expect(result.current.canSave).toBe(false);
});

it("does not materialize a managed block when changes are reverted after switching modes", () => {
  const { result, rerender, props } = setup();
  rerender({ ...props, content: "# Unmanaged document\n" });
  act(() => result.current.change("New custom content \n\n"));
  act(() => result.current.setMode("advanced"));
  expect(result.current.value).toContain("New custom content");
  act(() => result.current.setMode("default"));
  expect(result.current.value).toBe("New custom content \n\n");
  act(() => result.current.change(""));
  expect(result.current.canSave).toBe(false);
  act(() => result.current.setMode("advanced"));
  expect(result.current.value).toBe("# Unmanaged document\n");
});

it.each([false, true])("blocks unloaded and loading/readonly states (%s)", (blocked) => {
  const { result, rerender, props } = setup();
  rerender({ ...props, content: null, blocked });
  expect(result.current.editable).toBe(false);
  act(() => result.current.change("Must not be edited"));
  expect(result.current.value).toBe("Original instructions");
  rerender({ ...props, blocked: true });
  expect(result.current.canSave).toBe(false);
});

it("prevents duplicate saves, locks editing and resets dirty only on success", async () => {
  const { result, onSave } = setup();
  let finish!: (success: boolean) => void;
  onSave.mockImplementation(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  act(() => result.current.change("Saved instructions"));
  let saving!: Promise<void>;
  act(() => {
    saving = result.current.save();
    void result.current.save();
  });
  expect(onSave).toHaveBeenCalledTimes(1);
  expect(result.current.saving).toBe(true);
  expect(result.current.canSave).toBe(false);
  act(() => result.current.change("Cannot edit while saving"));
  expect(result.current.value).toBe("Saved instructions");
  await act(async () => {
    finish(true);
    await saving;
  });
  expect(result.current.saved).toBe(true);
  expect(result.current.dirty).toBe(false);
  expect(result.current.canSave).toBe(false);
});

it.each(["false", "throw"])("preserves failed drafts and supports retry (%s)", async (failure) => {
  const { result, onSave } = setup();
  if (failure === "throw") onSave.mockRejectedValueOnce(new Error("Network failure"));
  else onSave.mockResolvedValueOnce(false);
  act(() => result.current.change("Keep this draft"));
  await act(async () => {
    await result.current.save();
  });
  expect(result.current.failed).toBe(true);
  expect(result.current.value).toBe("Keep this draft");
  expect(result.current.canSave).toBe(true);
  await act(async () => {
    await result.current.save();
  });
  expect(result.current.dirty).toBe(false);
  expect(result.current.failed).toBe(false);
});

it("keeps drafts isolated when switching templates during a save", async () => {
  const { result, rerender, props, onSave } = setup();
  let finish!: (success: boolean) => void;
  onSave.mockImplementationOnce(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  act(() => result.current.change("First template draft"));
  let saving!: Promise<void>;
  act(() => {
    saving = result.current.save();
  });
  rerender({ ...props, templateId: "local.second", content: replaceManagedAgentInstructions("", "Second template") });
  act(() => result.current.change("Second template draft"));
  await act(async () => {
    finish(true);
    await saving;
  });
  expect(result.current.value).toBe("Second template draft");
  expect(result.current.canSave).toBe(true);
  rerender(props);
  expect(result.current.value).toBe("First template draft");
  expect(result.current.dirty).toBe(false);
});

it("preserves dirty content on remote refresh and requires explicit reload before overwriting", () => {
  const { result, rerender, props } = setup();
  act(() => result.current.change("Local draft"));
  const remote = replaceManagedAgentInstructions(content, "Remote update");
  rerender({ ...props, content: remote });
  expect(result.current.value).toBe("Local draft");
  expect(result.current.conflict).toBe(true);
  expect(result.current.canSave).toBe(false);
  act(() => result.current.reload());
  expect(result.current.value).toBe("Remote update");
  expect(result.current.dirty).toBe(false);
  expect(result.current.conflict).toBe(false);
});

it("accepts server refreshes during saving and synchronizes clean documents", async () => {
  const { result, rerender, props, onSave } = setup();
  let finish!: (success: boolean) => void;
  onSave.mockImplementationOnce(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  act(() => result.current.change("Saved update"));
  let saving!: Promise<void>;
  act(() => {
    saving = result.current.save();
  });
  const savedContent = onSave.mock.calls[0][0];
  rerender({ ...props, content: savedContent });
  await act(async () => {
    finish(true);
    await saving;
  });
  expect(result.current.conflict).toBe(false);
  expect(result.current.canSave).toBe(false);
  rerender({ ...props, content });
  expect(result.current.value).toBe("Original instructions");
});
