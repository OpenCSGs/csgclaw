import { Fragment, useLayoutEffect, useRef, type ComponentProps, type RefObject } from "react";
import { TextArea } from "@/components/ui";
import styles from "../StartTaskPage.module.css";

type TaskPromptEditorProps = Omit<ComponentProps<typeof TextArea>, "ref" | "value" | "onScroll"> & {
  ref: RefObject<HTMLTextAreaElement | null>;
  value: string;
  fileMentions: readonly { start: number; end: number }[];
};

export function TaskPromptEditor({ ref, value, fileMentions, ...props }: TaskPromptEditorProps) {
  const mirror = useRef<HTMLDivElement>(null);
  const hasFileMentions = fileMentions.length > 0;

  useLayoutEffect(() => {
    if (mirror.current && ref.current) mirror.current.scrollTop = ref.current.scrollTop;
  }, [ref, value, hasFileMentions]);

  return (
    <div className={styles.promptEditor} data-highlighted={hasFileMentions || undefined}>
      {hasFileMentions && (
        <div aria-hidden="true" ref={mirror} className={`${styles.input} ${styles.promptMirror}`}>
          {fileMentions.map(({ start, end }, index) => (
            <Fragment key={start}>
              {value.slice(fileMentions[index - 1]?.end ?? 0, start)}
              <span className={styles.fileMention}>{value.slice(start, end)}</span>
            </Fragment>
          ))}
          {value.slice(fileMentions.at(-1)?.end)}
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
