import { ArrowUpCircle, ChevronRight } from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./SidebarUpgradeButton.module.css";

type SidebarUpgradeButtonProps = {
  collapsed: boolean;
  version: string;
  t: TranslateFn;
  onClick: () => void;
};

export function SidebarUpgradeButton({ collapsed, version, t, onClick }: SidebarUpgradeButtonProps) {
  const label = t("sidebarNewVersion", { version });

  return (
    <Tooltip content={label} contentProps={{ side: "right" }}>
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
  );
}
