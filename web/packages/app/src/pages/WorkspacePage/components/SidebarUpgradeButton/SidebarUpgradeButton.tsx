import { ArrowUpCircle, ChevronRight, X } from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import styles from "./SidebarUpgradeButton.module.css";

type SidebarUpgradeButtonProps = {
  collapsed: boolean;
  version: string;
  t: TranslateFn;
  onClick: () => void;
  onDismiss: () => void;
};

export function SidebarUpgradeButton({ collapsed, version, t, onClick, onDismiss }: SidebarUpgradeButtonProps) {
  const label = t("sidebarNewVersion", { version });
  const dismissLabel = t("sidebarDismissUpdateNotice");
  const tooltipSide = collapsed ? "right" : "top";

  return (
    <div className={classNames(styles.notice, collapsed && styles.railNotice)}>
      <Tooltip content={label} contentProps={{ side: tooltipSide }}>
        <Button
          variant="tertiaryColor"
          size="sm"
          iconOnly={collapsed}
          className={collapsed ? styles.railButton : styles.rowButton}
          aria-label={label}
          aria-haspopup="dialog"
          onClick={onClick}
        >
          <ArrowUpCircle size={16} aria-hidden="true" />
          {!collapsed ? (
            <>
              <span className={styles.label}>{label}</span>
              <ChevronRight size={14} aria-hidden="true" />
            </>
          ) : null}
        </Button>
      </Tooltip>
      {!collapsed ? (
        <Tooltip content={dismissLabel} contentProps={{ side: "top" }}>
          <Button variant="tertiaryGray" size="sm" iconOnly aria-label={dismissLabel} onClick={onDismiss}>
            <X size={16} aria-hidden="true" />
          </Button>
        </Tooltip>
      ) : null}
    </div>
  );
}
