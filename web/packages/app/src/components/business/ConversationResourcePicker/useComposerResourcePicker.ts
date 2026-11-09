import { useEffect, useRef, useState, type RefObject } from "react";
import type { TaskResourceKind } from "@/components/business/ConversationPane/ComposerAddMenu";
import { placeCaretAtEnd } from "@/models/composer";
import type { ResourceSnapshot } from "@/models/conversationResources";

export type ComposerResourceProps = {
  resourceAgentID?: string;
  onPickResources?: (resources: ResourceSnapshot[]) => void;
};

export function useComposerResourcePicker({
  resourceAgentID,
  onPickResources,
  editorRef,
  selected,
  disabled,
}: ComposerResourceProps & {
  editorRef: RefObject<HTMLDivElement | HTMLTextAreaElement | null>;
  selected: readonly ResourceSnapshot[];
  disabled: boolean;
}) {
  const [selection, setSelection] = useState<{ agentID: string; kind: TaskResourceKind } | null>(null);
  const focusFrame = useRef<number | undefined>(undefined);
  useEffect(() => {
    setSelection(null);
    return () => {
      if (focusFrame.current !== undefined) cancelAnimationFrame(focusFrame.current);
    };
  }, [resourceAgentID, disabled]);

  function close(restoreFocus = true) {
    setSelection(null);
    if (!restoreFocus) return;
    focusFrame.current = requestAnimationFrame(() => {
      const editor = editorRef.current;
      if (!editor) return;
      editor.focus();
      if (editor instanceof HTMLTextAreaElement) {
        editor.setSelectionRange(editor.value.length, editor.value.length);
      } else {
        placeCaretAtEnd(editor);
      }
    });
  }

  const active = Boolean(!disabled && onPickResources && selection && selection.agentID === resourceAgentID);
  return {
    active,
    open:
      resourceAgentID && onPickResources && !disabled
        ? (kind: TaskResourceKind) => setSelection({ agentID: resourceAgentID, kind })
        : undefined,
    pickerProps:
      active && selection
        ? {
            agentID: selection.agentID,
            kind: selection.kind,
            existing: selected,
            onClose: close,
            onPick: (resource: ResourceSnapshot) => {
              onPickResources?.([resource]);
              close();
            },
          }
        : null,
  };
}
