import { Fragment, useLayoutEffect, useRef, type ComponentProps, type RefObject } from "react";
import { TextArea } from "@/components/ui";
import { builtinSlashCommandNames, suggestedSlashCommandNames } from "@/models/slashCommands";
import styles from "../StartTaskPage.module.css";

type TaskPromptEditorProps = Omit<ComponentProps<typeof TextArea>, "ref" | "value" | "onScroll"> & {
  ref: RefObject<HTMLTextAreaElement | null>;
  value: string;
  mentionRanges: readonly { start: number; end: number }[];
};

export function TaskPromptEditor({ ref, value, mentionRanges, ...props }: TaskPromptEditorProps) {
  const mirror = useRef<HTMLDivElement>(null);
  const command = /^\s*\/([^\s/]+)(?=\s|$)/u.exec(value);
  const commandName = command?.[1];
  const commandRanges =
    command && commandName && [...builtinSlashCommandNames, ...suggestedSlashCommandNames].includes(commandName)
      ? [{ start: command[0].length - commandName.length - 1, end: command[0].length, className: styles.slashCommand }]
      : [];
  const ranges = [...commandRanges, ...mentionRanges.map((range) => ({ ...range, className: styles.fileMention }))];
  const highlighted = ranges.length > 0;

  useLayoutEffect(() => {
    if (mirror.current && ref.current) mirror.current.scrollTop = ref.current.scrollTop;
  }, [ref, value, highlighted]);

  return (
    <div className={styles.promptEditor} data-highlighted={highlighted || undefined}>
      {highlighted && (
        <div aria-hidden="true" ref={mirror} className={`${styles.input} ${styles.promptMirror}`}>
          {ranges.map(({ start, end, className }, index) => (
            <Fragment key={start}>
              {value.slice(ranges[index - 1]?.end ?? 0, start)}
              <span className={className}>{value.slice(start, end)}</span>
            </Fragment>
          ))}
          {value.slice(ranges.at(-1)?.end)}
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
