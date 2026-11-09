import { ChevronRight, Layers3, UserRound } from "lucide-react";
import type { TranslateFn } from "@/models/conversations";
import styles from "./AgentOverviewGuide.module.css";

export function AgentOverviewGuide({ templates = false, t }: { templates?: boolean; t: TranslateFn }) {
  const prefix = templates ? "iaAgentsEmptyTemplates" : "iaAgentsEmptyMine";
  const Icon = templates ? Layers3 : UserRound;
  return (
    <section className={styles.root} aria-label={t(`${prefix}Title`)}>
      <div className={styles.content}>
        <div className={styles.icon} aria-hidden="true">
          <Icon size={24} strokeWidth={1.7} />
        </div>
        <h2>{t(`${prefix}Title`)}</h2>
        <p className={styles.description}>{t(`${prefix}Body`)}</p>
        <ul className={styles.tips}>
          {[1, 2, 3].map((index) => (
            <li key={index}>
              <ChevronRight size={16} aria-hidden="true" />
              <span>{t(`${prefix}Tip${index}`)}</span>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
