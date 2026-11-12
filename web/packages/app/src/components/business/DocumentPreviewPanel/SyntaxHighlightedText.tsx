import documentPreviewPanelStyles from "./DocumentPreviewPanel.module.css";
import { useEffect, useState } from "react";
import DOMPurify from "dompurify";
import { createHighlighterCore, type HighlighterCore } from "@shikijs/core";
import { createJavaScriptRegexEngine } from "@shikijs/engine-javascript";
import type { SyntaxLanguage } from "./syntaxLanguages";
import { syntaxLanguages } from "./syntaxLanguages";

let highlighterPromise: Promise<HighlighterCore> | null = null;
const languageLoads = new Map<SyntaxLanguage, Promise<void>>();

function highlighter() {
  highlighterPromise ??= createHighlighterCore({
    engine: createJavaScriptRegexEngine(),
    langs: [],
    themes: [import("@shikijs/themes/github-light"), import("@shikijs/themes/github-dark")],
  });
  return highlighterPromise;
}

async function highlight(text: string, language: SyntaxLanguage) {
  const instance = await highlighter();
  if (!instance.getLoadedLanguages().includes(language)) {
    let load = languageLoads.get(language);
    if (!load) {
      load = syntaxLanguages[language]().then((module) => instance.loadLanguage(module.default));
      languageLoads.set(language, load);
    }
    await load;
  }
  return instance.codeToHtml(text, {
    defaultColor: "light",
    lang: language,
    themes: { dark: "github-dark", light: "github-light" },
  });
}

export function SyntaxHighlightedText({
  language,
  scale,
  text,
  t,
}: {
  language: SyntaxLanguage;
  scale: number;
  text: string;
  t: (key: string) => string;
}) {
  const [html, setHTML] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setHTML("");
    setFailed(false);
    void highlight(text, language)
      .then((result) => {
        if (!cancelled) {
          setHTML(DOMPurify.sanitize(result));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setFailed(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [language, text]);

  if (failed) {
    return (
      <pre className={documentPreviewPanelStyles["document-preview-text"]} style={{ fontSize: `${scale}em` }}>
        {text}
      </pre>
    );
  }
  if (!html) {
    return <div className={documentPreviewPanelStyles["document-preview-status"]}>{t("attachmentPreviewLoading")}</div>;
  }
  return (
    <div
      className={documentPreviewPanelStyles["document-preview-shiki"]}
      data-language={language}
      style={{ fontSize: `${scale}em` }}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
