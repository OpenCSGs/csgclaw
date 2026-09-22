import type { AppTool } from "@/api/apps";
import type { TranslateFn } from "@/models/conversations";
import styles from "./AgentAppsPanel.module.css";

export function AppToolList({ tools, t }: { tools: AppTool[]; t: TranslateFn }) {
  return (
    <details className={styles.tools}>
      <summary>{t("appAvailableTools", { count: tools.length })}</summary>
      {tools.length ? (
        <ul>
          {tools.map((tool) => (
            <li key={tool.name}>
              <strong>{tool.title || tool.name}</strong>
              {tool.description ? <p>{tool.description}</p> : null}
            </li>
          ))}
        </ul>
      ) : (
        <p className={styles.hint}>{t("appNoTools")}</p>
      )}
    </details>
  );
}
