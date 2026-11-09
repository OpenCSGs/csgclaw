import type { AppTool } from "@/api/apps";
import type { TranslateFn } from "@/models/conversations";
import styles from "./AgentAppsPanel.module.css";

export function AppToolList({
  tools,
  t,
  fillAvailableHeight = false,
  defaultOpen = true,
}: {
  tools: AppTool[];
  t: TranslateFn;
  fillAvailableHeight?: boolean;
  defaultOpen?: boolean;
}) {
  const label = t("appAvailableTools", { count: tools.length });
  const toolItems = tools.map((tool) => (
    <li key={tool.name}>
      <strong>{tool.title || tool.name}</strong>
      {tool.description ? <p>{tool.description}</p> : null}
    </li>
  ));

  if (fillAvailableHeight) {
    return (
      <section className={`${styles.tools} ${styles.toolsFillAvailableHeight}`}>
        <h3 className={styles.toolsHeading}>{label}</h3>
        {tools.length ? (
          <div className={styles.toolsScrollViewport}>
            <ul>{toolItems}</ul>
          </div>
        ) : (
          <p className={styles.hint}>{t("appNoTools")}</p>
        )}
      </section>
    );
  }

  return (
    <details className={styles.tools} open={defaultOpen}>
      <summary>{label}</summary>
      {tools.length ? <ul>{toolItems}</ul> : <p className={styles.hint}>{t("appNoTools")}</p>}
    </details>
  );
}
