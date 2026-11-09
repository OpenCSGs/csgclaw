import { useMemo, useState } from "react";
import { LoaderCircle, RefreshCw } from "lucide-react";
import { Button, Select } from "@/components/ui";
import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { ResourceFeaturedIcon, SkillFeaturedIcon } from "@/components/business/HubDetailPane";
import { acknowledgeDiscoveredSkills, rescanDiscoveredSkills } from "@/api/skills";
import {
  matchesSkillQuery,
  skillLibrarySource,
  skillLibrarySources,
  skillSourceFilters,
  skillVersionLabel,
} from "@/models/skillLibrary";
import type { SkillSummary } from "@/models/skillhub";
import type { TranslateFn } from "@/models/conversations";
import styles from "./InstalledSkillLibrary.module.css";

export function InstalledSkillLibrary({
  skills,
  query,
  onOpen,
  onRefresh,
  busy = false,
  loading = false,
  t,
}: {
  skills: SkillSummary[];
  query: string;
  onOpen: (skill: SkillSummary) => void;
  onRefresh: () => Promise<unknown>;
  busy?: boolean;
  loading?: boolean;
  t: TranslateFn;
}) {
  const [sourceFilter, setSourceFilter] = useState("all");
  const [scanning, setScanning] = useState(false);
  const [error, setError] = useState(false);
  const [scanCount, setScanCount] = useState<number | null>(null);
  const sources = useMemo(() => skillSourceFilters(skills), [skills]);
  const activeFilter = sources.some((source) => source.source === sourceFilter) ? sourceFilter : "all";
  const visible = useMemo(
    () =>
      skills
        .filter(
          (skill) =>
            (activeFilter === "all" || skillLibrarySources(skill).includes(activeFilter)) &&
            matchesSkillQuery(skill, query),
        )
        .sort((a, b) => a.name.localeCompare(b.name)),
    [skills, activeFilter, query],
  );
  const options = [
    { value: "all", label: t("iaSkillsAllSources") },
    ...sources.map(({ source, count }) => ({ value: source, label: `${sourceLabel(source, t)} (${count})` })),
  ];
  async function refresh() {
    const result = await onRefresh();
    if (result && typeof result === "object" && "isError" in result && result.isError)
      throw new Error("refresh failed");
  }
  async function rescan() {
    setScanning(true);
    setError(false);
    setScanCount(null);
    try {
      const items = await rescanDiscoveredSkills();
      await refresh();
      setScanCount(items.filter((skill) => skill.discovery).length);
    } catch {
      setError(true);
    } finally {
      setScanning(false);
    }
  }
  async function open(skill: SkillSummary) {
    onOpen(skill);
    if (!skill.discovery?.new) return;
    try {
      await acknowledgeDiscoveredSkills([skill.discovery.key]);
      await refresh();
    } catch {
      setError(true);
    }
  }
  return (
    <div className={styles.library}>
      <div className={styles.toolbar}>
        <Select
          triggerClassName={styles.sourceFilter}
          value={activeFilter}
          onValueChange={setSourceFilter}
          options={options}
          size="sm"
          triggerProps={{ "aria-label": t("iaSkillsFilterSource") }}
        />
        <span className={styles.count}>
          {visible.length === skills.length
            ? t("iaSkillsTotalCount", { count: skills.length })
            : t("iaSkillsVisibleCount", { count: visible.length, total: skills.length })}
        </span>
        <Button
          className={styles.rescan}
          variant="secondaryGray"
          size="sm"
          disabled={busy || scanning}
          onClick={() => void rescan()}
        >
          <RefreshCw size={14} className={busy || scanning ? "animate-spin" : undefined} aria-hidden="true" />
          {t("iaSkillsDiscoveryRescan")}
        </Button>
      </div>
      {error ? (
        <p className="form-error" role="alert">
          {t("iaSkillsDiscoveryFailed")}
        </p>
      ) : null}
      {scanCount !== null ? (
        <p className={styles.notice} role="status">
          {t("iaSkillsDiscoveryComplete", { count: scanCount })}
        </p>
      ) : null}
      {loading ? (
        <p className={styles.notice}>
          <LoaderCircle size={14} className="animate-spin" aria-hidden="true" />
          {t("resourcesLoading")}
        </p>
      ) : visible.length === 0 ? (
        <p className={styles.notice}>{t(skills.length === 0 ? "resourcesSkillsEmpty" : "iaListSearchEmpty")}</p>
      ) : (
        <ResourceList>
          {visible.map((skill) => (
            <ResourceListCard
              key={skill.name}
              title={skill.name}
              description={skill.description}
              icon={
                <ResourceFeaturedIcon>
                  <SkillFeaturedIcon />
                </ResourceFeaturedIcon>
              }
              onOpen={() => void open(skill)}
              titleBadge={
                skill.discovery?.new ? <span className={styles.newBadge}>{t("iaSkillsUpdatedBadge")}</span> : undefined
              }
              actions={
                <Button
                  className={styles.source}
                  variant="ghost"
                  size="sm"
                  aria-label={t("iaSkillsViewVersionSources", { name: skill.name })}
                  title={
                    skill.discovery
                      ? t("iaSkillsActiveVersion", {
                          version: skillVersionLabel(skill.discovery.version) || t("iaSkillsUnversioned"),
                        }) +
                        "\n" +
                        skillLibrarySources(skill).join(" · ")
                      : sourceLabel(skillLibrarySource(skill), t)
                  }
                  onClick={() => void open(skill)}
                >
                  {skillLibrarySources(skill).length > 1
                    ? `${skillVersionLabel(skill.discovery?.version) || t("iaSkillsUnversioned")} · ${t("iaSkillsSoftwareCount", { count: skillLibrarySources(skill).length })}`
                    : `${sourceLabel(skillLibrarySource(skill), t)}${skill.discovery?.version ? ` · ${skillVersionLabel(skill.discovery.version)}` : ""}`}
                </Button>
              }
            />
          ))}
        </ResourceList>
      )}
    </div>
  );
}

function sourceLabel(source: string, t: TranslateFn): string {
  const keys: Record<string, string> = {
    builtin: "resourcesSkillSystemBadge",
    market: "iaSkillsBadgeMarket",
    local: "iaSkillsLocalSource",
  };
  return keys[source] ? t(keys[source]) : source;
}
