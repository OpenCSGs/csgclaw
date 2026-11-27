import type { ReactNode } from "react";
import { Sparkles } from "lucide-react";
import DOMPurify from "dompurify";
import { Marked } from "marked";
import styles from "./IntroductionCard.module.css";

const introductionMarkdown = new Marked({ breaks: true, gfm: true });

function renderIntroductionMarkdown(text: string): string {
  const html = introductionMarkdown.parse(text, { async: false });
  const content = DOMPurify.sanitize(html, {
    RETURN_DOM_FRAGMENT: true,
    ALLOWED_TAGS: [
      "a",
      "blockquote",
      "br",
      "code",
      "del",
      "em",
      "h1",
      "h2",
      "h3",
      "h4",
      "h5",
      "h6",
      "hr",
      "li",
      "ol",
      "p",
      "pre",
      "strong",
      "table",
      "tbody",
      "td",
      "th",
      "thead",
      "tr",
      "ul",
    ],
    ALLOWED_ATTR: ["href", "title", "start"],
    ALLOW_ARIA_ATTR: false,
    ALLOW_DATA_ATTR: false,
  });
  for (const link of content.querySelectorAll("a[href]")) {
    link.setAttribute("target", "_blank");
    link.setAttribute("rel", "noopener noreferrer");
  }
  const template = document.createElement("template");
  template.content.append(content);
  return template.innerHTML;
}

export function IntroductionMarkdown({ text, empty }: { text?: string | null; empty?: string }) {
  return text?.trim() ? (
    <div
      className={`${styles.introductionCopy} message-content`}
      dangerouslySetInnerHTML={{ __html: renderIntroductionMarkdown(text) }}
    />
  ) : (
    <p className={styles.introductionCopy}>{empty}</p>
  );
}

export function IntroductionCard({
  title,
  hint,
  actions,
  children,
}: {
  title: string;
  hint?: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className={styles.introduction} aria-label={title}>
      <div className={styles.introductionHeader}>
        <h2>
          <span className={styles.introductionIcon}>
            <Sparkles size={15} aria-hidden="true" />
          </span>
          {title}
        </h2>
        {actions ? <div className={styles.introductionActions}>{actions}</div> : null}
      </div>
      {hint ? <p className={styles.introductionHint}>{hint}</p> : null}
      {children}
    </section>
  );
}
