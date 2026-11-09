import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui";
import { skillSourceDirectories, skillVersionLabel } from "@/models/skillLibrary";
import type { SkillSummary } from "@/models/skillhub";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import styles from "./SkillSourceDetails.module.css";

export function SkillSourceDetails({ skill, locale, t }: { skill: SkillSummary; locale: LocaleCode; t: TranslateFn }) {
  const [expanded, setExpanded] = useState(false);
  const directories = skillSourceDirectories(skill);
  if (!directories.length) return null;
  const selected = directories[0];
  const tools = selected.tools.join(locale === "zh" ? "、" : ", ");
  return (
    <section className={styles.sources} aria-label={t("iaSkillsSourceVersions")}>
      <div className={styles.summary}>
        <p>
          {directories.length === 1 && selected.tools.length > 1
            ? t("iaSkillsSharedSource", { tools })
            : t("iaSkillsSelectedSource", { tools })}
        </p>
        {directories.length > 1 ? (
          <Button variant="linkGray" size="sm" aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>
            {expanded ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
            {t("iaSkillsCompareSources", { count: directories.length })}
          </Button>
        ) : null}
      </div>
      {directories.length > 1 && expanded ? (
        <div className={styles.comparison}>
          <p className={styles.rule}>{t("iaSkillsVersionSelectionRule")}</p>
          <ul>
            {directories.map((directory) => (
              <li key={directory.path} className={directory.active ? styles.active : undefined}>
                <div className={styles.row}>
                  <strong>{directory.tools.join(locale === "zh" ? "、" : ", ")}</strong>
                  <span>{skillVersionLabel(directory.version) || t("iaSkillsUnversioned")}</span>
                  {directory.active ? <span className={styles.activeBadge}>{t("iaSkillsCurrentlyUsed")}</span> : null}
                </div>
                <code>{directory.path}</code>
                <span className={styles.updated}>
                  {t("iaSkillsDefinitionUpdated", { time: formatSourceDate(directory.updatedAt, locale) })}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  );
}

function formatSourceDate(value: string, locale: LocaleCode): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "-";
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}
