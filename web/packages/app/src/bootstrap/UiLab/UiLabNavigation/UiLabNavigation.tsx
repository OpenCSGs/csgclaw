import { useLabTranslation } from "../LabLocale";
import { useEffect, useRef, useState } from "react";
import { Button, LinkButton, TextInput } from "@/components/ui";
import { labCatalogId, labGroups } from "./navigation";
import type { LabSectionId } from "./navigation";
import navigationStyles from "../ApplicationNavigation.module.css";
import styles from "./UiLabNavigation.module.css";

export function UiLabNavigation({ active }: { active: LabSectionId }) {
  const { text: labText, locale } = useLabTranslation();

  const sidebarRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const sidebar = sidebarRef.current;
    const link = sidebar?.querySelector('nav [aria-current="page"]');
    if (!sidebar || !link || !link.getClientRects().length) return;
    const frame = sidebar.getBoundingClientRect();
    const item = link.getBoundingClientRect();
    if (item.top < frame.top) sidebar.scrollTop += item.top - frame.top;
    if (item.bottom > frame.bottom) sidebar.scrollTop += item.bottom - frame.bottom;
  }, [active]);

  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState(false);
  const groups = labGroups.map((group) => ({
    ...group,
    items: group.items.filter((item) =>
      `${group.label} ${item.label} ${labText(group.label)} ${labText(item.label)}`
        .toLowerCase()
        .includes(query.trim().toLowerCase()),
    ),
  }));
  return (
    <aside className={styles.sidebar} ref={sidebarRef}>
      <a
        className={styles.brand}
        href={`#${labCatalogId}`}
        aria-current={active === labCatalogId ? "page" : undefined}
        onClick={() => setExpanded(false)}
      >
        <span className={styles.mark} aria-hidden="true">
          {labText("C")}
        </span>
        <div>
          <strong>{labText("CSGClaw")}</strong>
          <span>{labText("UI Lab · 组件工作台")}</span>
        </div>
      </a>
      <Button
        className={styles.toggle}
        size="sm"
        aria-expanded={expanded}
        aria-controls="ui-lab-directory"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? labText("收起目录") : labText("展开目录")}
      </Button>
      <nav
        id="ui-lab-directory"
        className={styles.directory}
        data-expanded={expanded}
        aria-label={labText("UI Lab 组件导航")}
      >
        <TextInput
          className={styles.search}
          size="sm"
          type="search"
          aria-label={labText("搜索组件目录")}
          placeholder={labText("搜索组件…")}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        {groups.map(
          (group) =>
            group.items.length > 0 && (
              <section key={labText(group.label)} className={styles.group} aria-label={labText(group.label)}>
                <h2>{labText(group.label)}</h2>
                <ul>
                  {group.items.map((item) => (
                    <li key={item.id}>
                      <LinkButton
                        className={navigationStyles.item}
                        variant="tertiaryGray"
                        href={`#${item.id}`}
                        aria-current={active === item.id ? "page" : undefined}
                        onClick={() => setExpanded(false)}
                      >
                        {locale === "zh" && "zh" in item ? item.zh : labText(item.label)}
                      </LinkButton>
                    </li>
                  ))}
                </ul>
              </section>
            ),
        )}
        {groups.every((group) => group.items.length === 0) && (
          <p role="status" className={styles.empty}>
            {labText("未找到匹配组件，请调整搜索词。")}
          </p>
        )}
      </nav>
    </aside>
  );
}
