import { classNames } from "@/shared/lib/classNames";
import { useEffect, useId, useMemo, useRef } from "react";
import type { CSSProperties } from "react";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { json } from "@codemirror/lang-json";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { linter } from "@codemirror/lint";
import type { Diagnostic } from "@codemirror/lint";
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import type { TranslateFn } from "@/models/conversations";
import jsonConfigEditorStyles from "./JSONConfigEditor.module.css";

const jsonEditorTheme = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    color: "var(--hub-json-editor-text, var(--text))",
    fontFamily: "var(--font-mono)",
    fontSize: "12px",
  },
  "&.cm-focused": {
    outline: "none",
  },
  ".cm-scroller": {
    fontFamily: "var(--font-mono)",
    lineHeight: "1.6",
    minHeight: "var(--hub-json-editor-min-height, 220px)",
  },
  ".cm-content": {
    caretColor: "var(--hub-json-editor-caret)",
    padding: "14px 0",
  },
  ".cm-line": {
    padding: "0 14px",
  },
  ".cm-gutters": {
    backgroundColor: "var(--hub-json-editor-gutter-bg)",
    borderRight: "1px solid var(--hub-json-editor-gutter-border)",
    color: "var(--hub-json-editor-gutter-text)",
    paddingLeft: "4px",
  },
  ".cm-activeLine": {
    backgroundColor: "var(--hub-json-editor-active-line)",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "var(--hub-json-editor-gutter-active-bg)",
    color: "var(--hub-json-editor-gutter-active-text)",
  },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
    backgroundColor: "var(--hub-json-editor-selection)",
  },
  ".cm-lintRange-error": {
    textDecoration: "underline wavy var(--error-600)",
    textDecorationSkipInk: "none",
  },
  ".cm-tooltip": {
    border: "1px solid var(--hub-json-editor-tooltip-border)",
    borderRadius: "var(--radius-md)",
    backgroundColor: "var(--hub-json-editor-tooltip-bg)",
    color: "var(--text)",
    boxShadow: "var(--shadow-lg)",
    fontFamily: "var(--font-sans)",
    fontSize: "12px",
  },
});

const jsonHighlightStyle = HighlightStyle.define([
  { tag: tags.propertyName, color: "var(--hub-json-editor-property)" },
  { tag: tags.string, color: "var(--hub-json-editor-string)" },
  { tag: tags.number, color: "var(--hub-json-editor-number)" },
  { tag: tags.bool, color: "var(--hub-json-editor-literal)" },
  { tag: tags.null, color: "var(--hub-json-editor-literal)" },
  { tag: tags.punctuation, color: "var(--hub-json-editor-punctuation)" },
]);

function createJsonSyntaxLinter(t: TranslateFn) {
  return function jsonSyntaxLinter(view: EditorView): Diagnostic[] {
    const source = view.state.doc.toString();
    try {
      JSON.parse(source);
      return [];
    } catch (error) {
      const detail = error instanceof Error ? error.message : "";
      const positionMatch = /position\s+(\d+)/i.exec(detail);
      const message = positionMatch
        ? t("resourcesJSONSyntaxInvalidAtPosition", { position: positionMatch[1] })
        : t("resourcesJSONSyntaxInvalid");
      const parsedPosition = positionMatch ? Number(positionMatch[1]) : source.length;
      if (source.length === 0) {
        return [
          {
            from: 0,
            message,
            severity: "error",
            to: 0,
          },
        ];
      }
      const position = Number.isFinite(parsedPosition) ? Math.max(0, Math.min(source.length, parsedPosition)) : 0;
      const from = Math.max(0, Math.min(source.length, position || source.length) - 1);
      const to = Math.min(source.length, Math.max(from + 1, position + 1));
      return [
        {
          from,
          message,
          severity: "error",
          to,
        },
      ];
    }
  };
}

function jsonEditorExtensions(t: TranslateFn, lineWrapping = true): Extension[] {
  return [
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightActiveLine(),
    history(),
    json(),
    syntaxHighlighting(jsonHighlightStyle),
    linter(createJsonSyntaxLinter(t), { delay: 250 }),
    keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap]),
    EditorState.tabSize.of(2),
    ...(lineWrapping ? [EditorView.lineWrapping] : []),
    jsonEditorTheme,
  ];
}

export function JSONConfigEditor({
  fillAvailableHeight = false,
  hideLabel = false,
  invalid = false,
  label,
  lineWrapping = true,
  minRows = 12,
  onChange,
  readOnly = false,
  t,
  value,
}: {
  fillAvailableHeight?: boolean;
  hideLabel?: boolean;
  invalid?: boolean;
  label: string;
  lineWrapping?: boolean;
  minRows?: number;
  onChange: (value: string) => void;
  readOnly?: boolean;
  t: TranslateFn;
  value: string;
}) {
  const editorId = useId();
  const editorParentRef = useRef<HTMLDivElement | null>(null);
  const editorViewRef = useRef<EditorView | null>(null);
  const initialValueRef = useRef(value);
  const onChangeRef = useRef(onChange);
  const editorExtensions = useMemo(
    () => [
      ...jsonEditorExtensions(t, lineWrapping),
      EditorState.readOnly.of(readOnly),
      EditorView.editable.of(!readOnly),
    ],
    [lineWrapping, readOnly, t],
  );
  const minHeight = `${Math.max(minRows, 6) * 19.2 + 28}px`;

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    if (!editorParentRef.current) {
      return;
    }
    const view = new EditorView({
      state: EditorState.create({
        doc: initialValueRef.current,
        extensions: [
          ...editorExtensions,
          EditorView.contentAttributes.of({
            "aria-label": label,
            ...(readOnly ? { "aria-readonly": "true" } : {}),
            id: editorId,
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              onChangeRef.current(update.state.doc.toString());
            }
          }),
        ],
      }),
      parent: editorParentRef.current,
    });
    editorViewRef.current = view;
    return () => {
      editorViewRef.current = null;
      view.destroy();
    };
  }, [editorExtensions, editorId, label, readOnly]);

  useEffect(() => {
    const view = editorViewRef.current;
    if (!view) {
      return;
    }
    const current = view.state.doc.toString();
    if (current === value) {
      return;
    }
    view.dispatch({
      changes: {
        from: 0,
        insert: value,
        to: current.length,
      },
    });
  }, [value]);

  return (
    <div
      className={classNames(
        jsonConfigEditorStyles["hub-json-editor"],
        invalid ? jsonConfigEditorStyles["is-invalid"] : "",
        fillAvailableHeight ? "is-fill-available-height" : "",
      )}
      style={{ "--hub-json-editor-min-height": minHeight } as CSSProperties}
    >
      <label
        className={classNames(jsonConfigEditorStyles["hub-json-editor-label"], hideLabel ? "sr-only" : "")}
        htmlFor={editorId}
      >
        {label}
      </label>
      <div
        className={jsonConfigEditorStyles["hub-json-editor-shell"]}
        ref={editorParentRef}
        aria-invalid={invalid || undefined}
      />
    </div>
  );
}
