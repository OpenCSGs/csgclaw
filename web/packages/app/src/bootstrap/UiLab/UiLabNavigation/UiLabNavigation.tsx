import { useLabTranslation } from "../LabLocale";
import { useState } from "react";
import { Button, TextInput } from "@/components/ui";
import { labGroups } from "./navigation";
import type { LabSectionId } from "./navigation";
import styles from "./UiLabNavigation.module.css";

export function UiLabNavigation({ active }: { active: LabSectionId }) {
  const { text: labText, locale } = useLabTranslation();

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
    <aside className={styles.sidebar}>
      <div className={styles.brand}>
        <span className={styles.mark} aria-hidden="true">
          {labText("C")}
        </span>
        <div>
          <strong>{labText("CSGClaw")}</strong>
          <span>{labText("UI Lab · 组件工作台")}</span>
        </div>
      </div>
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
                      <a
                        href={`#${item.id}`}
                        aria-current={active === item.id ? "page" : undefined}
                        onClick={() => setExpanded(false)}
                      >
                        {locale === "zh" && "zh" in item ? item.zh : labText(item.label)}
                      </a>
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
