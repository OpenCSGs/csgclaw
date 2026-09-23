import DOMPurify from "dompurify";
import { Marked, marked } from "marked";
import { parseCitationDocument, type CitationSource } from "@/models/citations";
import { decorateMentionMarkup, escapeHTML } from "./mentions";

marked.setOptions({
  breaks: true,
  gfm: true,
});

export function renderMarkdown(content: unknown): string {
  return renderMarkdownWithCitations(content).html;
}

export type RenderedCitation = CitationSource & { number: number };

export type RenderedMarkdown = {
  html: string;
  cited: RenderedCitation[];
  byID: Map<string, RenderedCitation>;
};

export function renderMarkdownWithCitations(
  content: unknown,
  citationLabel: (title: string) => string = (title) => `查看引用：${title}`,
): RenderedMarkdown {
  const citationDocument = parseCitationDocument(String(content ?? ""));
  const cited: RenderedCitation[] = [];
  const byID = new Map<string, RenderedCitation>();
  const parser = citationDocument.sources.size ? new Marked({ breaks: true, gfm: true }) : null;
  parser?.use({
    extensions: [
      {
        name: "csgclawCitation",
        level: "inline",
        start(source) {
          const numbered = source.indexOf("([");
          return numbered < 0 ? undefined : numbered;
        },
        tokenizer(source) {
          const numbered = /^\(\[([^\]\n]{1,300})\]\[([1-9]\d{0,5})\]\)/.exec(source);
          if (!numbered) return;
          return { type: "csgclawCitation", raw: numbered[0], id: numbered[2], title: numbered[1] };
        },
        renderer(token) {
          const id = String(token.id ?? "");
          const source = citationDocument.sources.get(id);
          if (!source) return escapeHTML(token.raw);
          let reference = byID.get(id);
          if (!reference) {
            reference = {
              ...source,
              title: String(token.title ?? source.title).trim(),
              number: Number(id),
            };
            cited.push(reference);
            byID.set(id, reference);
          }
          return `<button type="button" class="message-citation-link" data-citation-id="${escapeHTML(id)}" aria-label="${escapeHTML(citationLabel(reference.title))}">${escapeHTML(reference.title)}</button>`;
        },
      },
    ],
  });
  const raw = parser
    ? (parser.parse(decorateMentionMarkup(citationDocument.body)) as string)
    : (marked.parse(decorateMentionMarkup(citationDocument.body)) as string);
  const sanitized = DOMPurify.sanitize(raw, {
    ADD_ATTR: ["target", "rel", "class", "data-user-id", "data-citation-id", "aria-label"],
    USE_PROFILES: { html: true },
  });
  const template = document.createElement("template");
  template.innerHTML = sanitized;
  template.content.querySelectorAll("a[href]").forEach((link) => {
    link.setAttribute("target", "_blank");
    link.setAttribute("rel", "noopener noreferrer");
  });
  return { html: template.innerHTML, cited, byID };
}
