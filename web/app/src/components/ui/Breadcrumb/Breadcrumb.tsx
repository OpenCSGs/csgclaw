import styles from "./Breadcrumb.module.css";
import homeIcon from "./assets/home-line.svg";
import slashDivider from "./assets/slash-divider.svg";
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRoot,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";
import { classNames } from "@/shared/lib/classNames";

export type BreadcrumbItem = {
  current?: boolean;
  href?: string;
  label: string;
  onClick?: () => void;
};

export type BreadcrumbProps = {
  "aria-label"?: string;
  className?: string;
  home?: BreadcrumbItem;
  items: BreadcrumbItem[];
  maxVisibleItems?: number;
  overflowLabel?: string;
  dividerSrc?: string;
  dividerSize?: number;
  homeIconSrc?: string;
};

function Item({ item, home = false, icon = homeIcon }: { home?: boolean; item: BreadcrumbItem; icon?: string }) {
  const className = classNames(styles.item, home && styles.homeItem, item.current && styles.current);
  const content = home ? <img src={icon} width="20" height="20" alt="" /> : item.label;

  if (item.current) {
    return (
      <span className={className} aria-current="page" title={home ? item.label : undefined}>
        {content}
        {home ? <span className={styles.srOnly}>{item.label}</span> : null}
      </span>
    );
  }

  if (item.href) {
    return (
      <a
        className={className}
        href={item.href}
        aria-label={home ? item.label : undefined}
        title={home ? item.label : undefined}
      >
        {content}
      </a>
    );
  }

  return (
    <button
      className={className}
      type="button"
      onClick={item.onClick}
      aria-label={home ? item.label : undefined}
      title={home ? item.label : undefined}
      disabled={!item.onClick}
    >
      {content}
    </button>
  );
}

export function Breadcrumb({
  "aria-label": ariaLabel = "Breadcrumb",
  className,
  home,
  items,
  maxVisibleItems = 4,
  dividerSrc = slashDivider,
  dividerSize = 20,
  homeIconSrc = homeIcon,
  overflowLabel = "Show earlier path segments",
}: BreadcrumbProps) {
  const shouldCollapse = items.length > maxVisibleItems && maxVisibleItems >= 3;
  const edgeCount = shouldCollapse ? Math.max(1, Math.floor((maxVisibleItems - 2) / 2)) : items.length;
  const hiddenItems = shouldCollapse ? items.slice(edgeCount, -(maxVisibleItems - edgeCount - 1)) : [];
  const tailCount = maxVisibleItems - edgeCount - 1;
  const pathEntries = shouldCollapse
    ? [
        ...items.slice(0, edgeCount).map((item, index) => ({ kind: "item" as const, item, key: `head-${index}` })),
        { kind: "overflow" as const, key: "overflow" },
        ...items.slice(-tailCount).map((item, index) => ({ kind: "item" as const, item, key: `tail-${index}` })),
      ]
    : items.map((item, index) => ({ kind: "item" as const, item, key: `item-${index}` }));
  const renderedEntries = [...(home ? [{ kind: "home" as const, item: home, key: "home" }] : []), ...pathEntries];

  return (
    <nav className={classNames(styles.breadcrumb, className)} aria-label={ariaLabel}>
      <ol className={styles.list}>
        {renderedEntries.map((entry, index) => (
          <li className={styles.segment} key={entry.key}>
            {index > 0 ? (
              <img className={styles.divider} src={dividerSrc} width={dividerSize} height={dividerSize} alt="" />
            ) : null}
            {entry.kind === "overflow" ? (
              <DropdownMenuRoot>
                <DropdownMenuTrigger className={styles.overflowTrigger} type="button" aria-label={overflowLabel}>
                  …
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" size="sm">
                  {hiddenItems.map((item, hiddenIndex) => (
                    <DropdownMenuItem
                      key={`${item.label}-${hiddenIndex}`}
                      onSelect={item.onClick}
                      disabled={!item.onClick}
                    >
                      {item.label}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenuRoot>
            ) : (
              <Item item={entry.item} home={entry.kind === "home"} icon={homeIconSrc} />
            )}
          </li>
        ))}
      </ol>
    </nav>
  );
}
