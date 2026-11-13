import { Fragment, useLayoutEffect, useRef, type ComponentProps, type RefObject } from "react";
import { TextArea } from "@/components/ui";
import styles from "../StartTaskPage.module.css";

type TaskPromptEditorProps = Omit<ComponentProps<typeof TextArea>, "ref" | "value" | "onScroll"> & {
  ref: RefObject<HTMLTextAreaElement | null>;
  value: string;
  mentionRanges: readonly { start: number; end: number }[];
};

export function TaskPromptEditor({ ref, value, mentionRanges, ...props }: TaskPromptEditorProps) {
  const mirror = useRef<HTMLDivElement>(null);
  const hasMentions = mentionRanges.length > 0;

  useLayoutEffect(() => {
    if (mirror.current && ref.current) mirror.current.scrollTop = ref.current.scrollTop;
  }, [ref, value, hasMentions]);

  return (
    <div className={styles.promptEditor} data-highlighted={hasMentions || undefined}>
      {hasMentions && (
        <div aria-hidden="true" ref={mirror} className={`${styles.input} ${styles.promptMirror}`}>
          {mentionRanges.map(({ start, end }, index) => (
            <Fragment key={start}>
              {value.slice(mentionRanges[index - 1]?.end ?? 0, start)}
              <span className={styles.fileMention}>{value.slice(start, end)}</span>
            </Fragment>
          ))}
          {value.slice(mentionRanges.at(-1)?.end)}
          {"\u200b"}
        </div>
      )}
      <TextArea
        {...props}
        ref={ref}
        value={value}
        className={styles.input}
        onScroll={(event) => {
          if (mirror.current) mirror.current.scrollTop = event.currentTarget.scrollTop;
        }}
      />
    </div>
  );
}
