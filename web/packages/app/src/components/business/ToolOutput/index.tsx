import { useId, useLayoutEffect, useRef, useState } from "react";
import styles from "./ToolOutput.module.css";

type ToolOutputProps = {
  label: string;
  text: string;
  toggleLabels?: { expand: string; collapse: string };
};

export function ToolOutput({ label, text, toggleLabels }: ToolOutputProps) {
  const id = useId();
  const region = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLPreElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [overflows, setOverflows] = useState(false);
  const expandable = Boolean(toggleLabels);

  useLayoutEffect(() => {
    const box = region.current;
    const pre = content.current;
    if (!expandable || expanded || !box || !pre) return;

    const measure = () => setOverflows(box.scrollHeight > box.clientHeight);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(box);
    observer.observe(pre);
    return () => observer.disconnect();
  }, [expandable, expanded, text]);

  return (
    <>
      <div
        id={id}
        ref={region}
        className={styles.output}
        data-expandable={expandable || undefined}
        data-expanded={expandable && expanded ? true : undefined}
        role="region"
        aria-label={label}
        tabIndex={0}
      >
        <pre ref={content}>{text}</pre>
      </div>
      {toggleLabels && (overflows || expanded) ? (
        <button
          type="button"
          className={styles.toggle}
          aria-controls={id}
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? toggleLabels.collapse : toggleLabels.expand}
        </button>
      ) : null}
    </>
  );
}
