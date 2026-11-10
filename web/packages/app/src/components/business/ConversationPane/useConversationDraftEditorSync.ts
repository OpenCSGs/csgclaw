import { useLayoutEffect } from "react";
import type { RefObject } from "react";
import {
  areComposerSegmentsEqual,
  getCollapsedSelectionTextOffset,
  parseComposerSegments,
  placeCaretAtEnd,
  renderComposerSegments,
  updateComposerResourceLabels,
  segmentsToPlainText,
  type ComposerSegment,
} from "@/models/composer";

export function useConversationDraftEditorSync(
  editorRef: RefObject<HTMLDivElement | null>,
  draftSegments: ComposerSegment[],
  resourceRemoveLabel: string,
) {
  useLayoutEffect(() => {
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    updateComposerResourceLabels(editor, resourceRemoveLabel);
    const currentSegments = parseComposerSegments(editor);
    if (!areComposerSegmentsEqual(currentSegments, draftSegments)) {
      const currentText = segmentsToPlainText(currentSegments);
      const nextText = segmentsToPlainText(draftSegments);
      const selectionOffset = getCollapsedSelectionTextOffset(editor);
      renderComposerSegments(editor, draftSegments, resourceRemoveLabel);
      if (currentText === nextText && selectionOffset === currentText.length) {
        placeCaretAtEnd(editor);
      }
    }
  }, [draftSegments, editorRef, resourceRemoveLabel]);
}
