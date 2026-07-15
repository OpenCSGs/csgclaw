import { classNames } from "@/shared/lib/classNames";
import styles from "./WorkspaceSidebar.module.css";
import { Tooltip } from "@/components/ui";
import type { ReactNode } from "react";
import type { WorkspaceContextSectionId } from "./types";

export type PrimaryNavigationItem = {
  active: boolean;
  badge?: number;
  groupId: WorkspaceContextSectionId;
  icon: ReactNode;
  id: string;
  label: string;
  onSelect: () => void;
};

export type PrimaryNavigationSection = {
  id: string;
  items: PrimaryNavigationItem[];
  label: string;
};

type WorkspacePrimaryNavigationProps = {
  collapsed: boolean;
  onActivate: (item: PrimaryNavigationItem) => void;
  sections: PrimaryNavigationSection[];
};

export function WorkspacePrimaryNavigation({ collapsed, onActivate, sections }: WorkspacePrimaryNavigationProps) {
  const collapsedItems = sections.flatMap((section) => section.items);

  return (
    <nav className={classNames(styles.primaryNav, collapsed && styles.primaryNavCollapsed)} aria-label="Workspace">
      {collapsed ? (
        <div className={styles.primaryCollapsedItems}>
          {collapsedItems.map((item) => (
            <PrimaryNavigationButton key={item.id} item={item} collapsed onClick={() => onActivate(item)} />
          ))}
        </div>
      ) : (
        sections.map((section) => (
          <div key={section.id} className={styles.primaryNavSection}>
            <div className={styles.primaryGroupLabel}>{section.label}</div>
            <div className={styles.primarySectionItems}>
              {section.items.map((item) => (
                <PrimaryNavigationButton key={item.id} item={item} onClick={() => onActivate(item)} />
              ))}
            </div>
          </div>
        ))
      )}
    </nav>
  );
}

function PrimaryNavigationButton({
  collapsed = false,
  item,
  onClick,
}: {
  collapsed?: boolean;
  item: PrimaryNavigationItem;
  onClick: () => void;
}) {
  const button = (
    <button
      type="button"
      className={classNames(styles.primaryNavRow, collapsed && styles.iconOnly, item.active && styles.active)}
      aria-label={item.label}
      onClick={onClick}
    >
      <span className={styles.primaryNavIcon} aria-hidden="true">
        {item.icon}
      </span>
      {!collapsed ? <span className={classNames(styles.primaryNavLabel, "truncate")}>{item.label}</span> : null}
      {typeof item.badge === "number" ? (
        <span className={styles.primaryNavBadge}>{formatBadge(item.badge)}</span>
      ) : null}
    </button>
  );

  return collapsed ? <Tooltip content={item.label}>{button}</Tooltip> : button;
}

function formatBadge(value: number) {
  return value > 99 ? "99+" : String(value);
}
