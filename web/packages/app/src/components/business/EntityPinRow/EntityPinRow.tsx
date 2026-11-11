import { ChevronDown, Pin } from "lucide-react";
import type { ReactNode } from "react";
import { Button, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./EntityPinRow.module.css";

export function EntityPinHeading({
  expanded,
  onToggle,
  t,
}: {
  expanded: boolean;
  onToggle: () => void;
  t: TranslateFn;
}) {
  return (
    <Button variant="tertiaryGray" size="sm" className={styles.heading} aria-expanded={expanded} onClick={onToggle}>
      {t("iaPinned")}
      <ChevronDown className={styles.chevron} size={13} aria-hidden="true" />
    </Button>
  );
}

export function EntityPinRow({
  as: Tag = "div",
  name,
  pinned,
  t,
  onToggle,
  children,
}: {
  as?: "li" | "div";
  name: string;
  pinned: boolean;
  t: TranslateFn;
  onToggle: () => void;
  children: ReactNode;
}) {
  const action = t(pinned ? "iaUnpin" : "iaPin");
  return (
    <Tag className={styles.row}>
      {children}
      <Tooltip content={action} contentProps={{ side: "top", sideOffset: 6 }}>
        <Button
          variant="tertiaryGray"
          className={styles.pin}
          iconOnly
          size="sm"
          aria-label={action}
          aria-description={name}
          onClick={onToggle}
        >
          <Pin size={14} fill={pinned ? "currentColor" : "none"} aria-hidden="true" />
        </Button>
      </Tooltip>
    </Tag>
  );
}
