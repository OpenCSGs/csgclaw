import DOMPurify from "dompurify";
import { marked } from "marked";
import { decorateMentionMarkup } from "./mentions";

marked.setOptions({
  breaks: true,
  gfm: true,
});

export function renderMarkdown(content: unknown): string {
  const raw = marked.parse(decorateMentionMarkup(content)) as string;
  const sanitized = DOMPurify.sanitize(raw, {
    ADD_ATTR: ["target", "rel", "class", "data-user-id"],
    USE_PROFILES: { html: true },
  });
  const template = document.createElement("template");
  template.innerHTML = sanitized;
  template.content.querySelectorAll("a[href]").forEach((link) => {
    link.setAttribute("target", "_blank");
    link.setAttribute("rel", "noopener noreferrer");
  });
  return template.innerHTML;
}
