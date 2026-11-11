import { Pin, PinOff } from "lucide-react";
import type { ReactNode } from "react";
import { Button, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./EntityPinRow.module.css";

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
  const Icon = pinned ? PinOff : Pin;
  return (
    <Tag className={styles.row} data-pinned={pinned}>
      {children}
      <Tooltip content={action}>
        <Button
          variant={pinned ? "tertiaryColor" : "tertiaryGray"}
          className={styles.pin}
          iconOnly
          size="sm"
          aria-label={action}
          aria-description={name}
          aria-pressed={pinned}
          title={`${action} · ${name}`}
          onClick={onToggle}
        >
          <Icon size={14} aria-hidden="true" />
        </Button>
      </Tooltip>
    </Tag>
  );
}
