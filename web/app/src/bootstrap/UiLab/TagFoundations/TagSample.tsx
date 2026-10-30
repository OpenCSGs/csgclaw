import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { Button, Checkbox } from "@/components/ui";
import type snapshot from "../figmaTags.json";
import styles from "./TagFoundations.module.css";

export type TagEntry = (typeof snapshot.entries)[number];
const assets = import.meta.glob<string>("./assets/*", { eager: true, query: "?url", import: "default" });
const metrics = {
  sm: { checkbox: 14, close: 14, count: 16, countFont: 12 },
  md: { checkbox: 16, close: 16, count: 19, countFont: 12 },
  lg: { checkbox: 18, close: 20, count: 21, countFont: 14 },
};

export function TagSample({
  entry,
  label = "Label",
  interactive = false,
  onRemove,
}: {
  entry: TagEntry;
  label?: string;
  interactive?: boolean;
  onRemove?: () => void;
}) {
  const { text: labText } = useLabTranslation();

  const [checked, setChecked] = useState(false);
  const ref = entry.reference;
  if (!ref) return <p className={styles.pending}>{labText("样式待读取")}</p>;
  const sizes = metrics[entry.size as keyof typeof metrics];
  return (
    <div
      className={styles.tag}
      data-tag-sample={entry.id}
      style={{
        width: interactive ? undefined : entry.width,
        height: entry.height,
        gap: ref.outerGap,
        paddingInlineStart: Math.max(0, Number.parseFloat(ref.left) - 1),
        paddingInlineEnd: Math.max(0, Number.parseFloat(ref.right) - 1),
        background: ref.background,
        borderColor: ref.border,
        borderRadius: ref.radius,
        fontSize: ref.fontSize,
        lineHeight: ref.lineHeight,
        color: ref.color,
      }}
    >
      <div className={styles.content} style={{ gap: ref.gap }}>
        {entry.checkbox && (
          <Checkbox
            aria-label={interactive ? labText("选择标签") : labText("参考复选框")}
            checked={interactive && checked}
            onCheckedChange={(next) => setChecked(next === true)}
            style={{
              width: sizes.checkbox,
              height: sizes.checkbox,
              minWidth: sizes.checkbox,
              minHeight: sizes.checkbox,
              borderColor: checked ? undefined : ref.border,
              background: checked ? undefined : ref.background,
            }}
          />
        )}
        {ref.iconAsset && (
          <span className={entry.icon === "Avatar" ? styles.avatar : styles.icon}>
            <img src={assets[`./assets/${ref.iconAsset}`]} alt="" />
          </span>
        )}
        <span className={styles.label} title={label}>
          {label}
        </span>
        {entry.action === "Count" && (
          <span
            className={styles.count}
            aria-label={labText("数量 5")}
            style={{
              width: sizes.count,
              height: entry.size === "lg" ? 20 : entry.size === "md" ? 18 : 16,
              fontSize: sizes.countFont,
            }}
          >
            5
          </span>
        )}
      </div>
      {ref.closeAsset && (
        <Button
          iconOnly
          variant="tertiaryGray"
          aria-label={interactive ? labText("移除标签") : labText("参考关闭按钮")}
          onClick={onRemove}
          style={{
            width: sizes.close,
            minWidth: sizes.close,
            height: sizes.close,
            minHeight: sizes.close,
            padding: 2,
            border: 0,
            borderRadius: 3,
            boxShadow: "none",
            flexShrink: 0,
          }}
        >
          <img src={assets[`./assets/${ref.closeAsset}`]} alt="" />
        </Button>
      )}
    </div>
  );
}
