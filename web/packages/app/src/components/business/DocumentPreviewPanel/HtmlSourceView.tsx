import { useEffect, useMemo, useRef } from "react";
import { standardKeymap } from "@codemirror/commands";
import { htmlLanguage } from "@codemirror/lang-html";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { Compartment, EditorState } from "@codemirror/state";
import { EditorView, keymap, lineNumbers } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import styles from "./DocumentPreviewPanel.module.css";

const sourceTheme = EditorView.theme({
  "&": { height: "100%", backgroundColor: "var(--surface)", color: "var(--text)", fontSize: "inherit" },
  "&.cm-focused": { outlineOffset: "-2px" },
  ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.6", overflow: "auto" },
  ".cm-content": { padding: "12px 0" },
  ".cm-line": { padding: "0 14px" },
  ".cm-gutters": { backgroundColor: "var(--panel)", color: "var(--muted)", borderColor: "var(--line)" },
  ".cm-lineNumbers .cm-gutterElement": { padding: "0 10px", minWidth: "40px" },
});

const sourceHighlight = syntaxHighlighting(
  HighlightStyle.define([
    { tag: [tags.tagName, tags.keyword], color: "var(--html-source-keyword)" },
    { tag: [tags.attributeName, tags.propertyName], color: "var(--html-source-attribute)" },
    { tag: [tags.attributeValue, tags.string], color: "var(--html-source-string)" },
    { tag: [tags.number, tags.bool], color: "var(--html-source-number)" },
    { tag: [tags.comment, tags.meta], color: "var(--muted)" },
  ]),
);

export function HtmlSourceView({
  label,
  scale,
  text,
  wrap,
}: {
  label: string;
  scale: number;
  text: string;
  wrap: boolean;
}) {
  const host = useRef<HTMLDivElement>(null);
  const editor = useRef<EditorView | null>(null);
  const wrapping = useMemo(() => new Compartment(), []);

  useEffect(() => {
    if (!host.current) return;
    const view = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: text,
        extensions: [
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          EditorView.contentAttributes.of({ "aria-label": label, "aria-readonly": "true", tabindex: "0" }),
          EditorState.tabSize.of(2),
          keymap.of(standardKeymap),
          lineNumbers(),
          htmlLanguage,
          sourceHighlight,
          sourceTheme,
          wrapping.of([]),
        ],
      }),
    });
    editor.current = view;
    return () => {
      editor.current = null;
      view.destroy();
    };
  }, [label, text, wrapping]);

  useEffect(() => {
    editor.current?.dispatch({ effects: wrapping.reconfigure(wrap ? EditorView.lineWrapping : []) });
  }, [label, text, wrap, wrapping]);

  return <div className={styles["document-preview-html-source"]} ref={host} style={{ fontSize: `${13 * scale}px` }} />;
}
