import { useState } from "react";
import { ArrowUpRight } from "lucide-react";
import { TextInput } from "@/components/ui";
import { applicationSections } from "./ApplicationFoundations/registry";
import { useLabTranslation } from "./LabLocale";
import { labGroups } from "./UiLabNavigation/navigation";
import styles from "./UiLab.module.css";

const descriptions = new Map<string, string>(
  applicationSections.map(({ key, description }) => [`${key}-foundations`, description]),
);

export function UiLabCatalog() {
  const { text: t, locale } = useLabTranslation();
  const [query, setQuery] = useState("");
  const search = query.trim().toLowerCase();
  const groups = labGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) =>
        `${group.label} ${t(group.label)} ${item.label} ${t(item.label)} ${"zh" in item ? item.zh : ""}`
          .toLowerCase()
          .includes(search),
      ),
    }))
    .filter((group) => group.items.length > 0);

  return (
    <section className={styles.catalog} id="components" aria-labelledby="ui-lab-catalog-title">
      <header className={styles.catalogHeader}>
        <h2 id="ui-lab-catalog-title">{t("组件目录")}</h2>
        <TextInput
          type="search"
          aria-label={t("搜索组件目录")}
          placeholder={t("搜索组件…")}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </header>
      {groups.map((group) => (
        <section className={styles.catalogGroup} key={group.label}>
          <h3>{t(group.label)}</h3>
          <ul className={styles.catalogGrid}>
            {group.items.map((item) => {
              const description = descriptions.get(item.id);
              return (
                <li key={item.id}>
                  <a href={`#${item.id}`} className={styles.catalogItem}>
                    <span className={styles.catalogItemTitle}>
                      <strong>{locale === "zh" && "zh" in item ? item.zh : t(item.label)}</strong>
                      <ArrowUpRight size={16} aria-hidden="true" />
                    </span>
                    {description && <span className={styles.catalogDescription}>{t(description)}</span>}
                  </a>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
      {groups.length === 0 && <p role="status">{t("未找到匹配组件，请调整搜索词。")}</p>}
    </section>
  );
}
