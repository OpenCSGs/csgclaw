import { classNames } from "@/shared/lib/classNames";
import styles from "./WorkspaceSidebar.module.css";
import { useCallback, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { CSSProperties, ReactNode } from "react";
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
  const tooltipId = useId();
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const [tooltipStyle, setTooltipStyle] = useState<CSSProperties | null>(null);

  const showTooltip = useCallback(() => {
    if (!collapsed || !buttonRef.current) {
      return;
    }
    const rect = buttonRef.current.getBoundingClientRect();
    setTooltipStyle({
      left: rect.right + 10,
      top: rect.top + rect.height / 2,
    });
  }, [collapsed]);

  const hideTooltip = useCallback(() => {
    setTooltipStyle(null);
  }, []);

  return (
    <button
      ref={buttonRef}
      type="button"
      className={classNames(styles.primaryNavRow, collapsed && styles.iconOnly, item.active && styles.active)}
      aria-label={item.label}
      aria-describedby={tooltipStyle ? tooltipId : undefined}
      onClick={onClick}
      onBlur={hideTooltip}
      onFocus={showTooltip}
      onMouseEnter={showTooltip}
      onMouseLeave={hideTooltip}
    >
      <span className={styles.primaryNavIcon} aria-hidden="true">
        {item.icon}
      </span>
      {!collapsed ? <span className={classNames(styles.primaryNavLabel, "truncate")}>{item.label}</span> : null}
      {typeof item.badge === "number" ? (
        <span className={styles.primaryNavBadge}>{formatBadge(item.badge)}</span>
      ) : null}
      {collapsed && tooltipStyle
        ? createPortal(
            <span id={tooltipId} className={styles.primaryNavTooltip} role="tooltip" style={tooltipStyle}>
              {item.label}
            </span>,
            document.body,
          )
        : null}
    </button>
  );
}

function formatBadge(value: number) {
  return value > 99 ? "99+" : String(value);
}
