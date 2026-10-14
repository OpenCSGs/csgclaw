import { skillOriginLabel as originLabel } from "./resourceOrigins";
import { useEffect, useMemo, useState } from "react";
import { LoaderCircle, Plus } from "lucide-react";
import { Button } from "@/components/ui";
import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import { hasSkillName, remoteSkillInstallName } from "@/models/skillhub";
import {
  ResourceFeaturedIcon,
  ResourceFilterTabs,
  ResourceSearchField,
  SkillFeaturedIcon,
} from "@/components/business/HubDetailPane";
import { SkillUploadDialog } from "@/components/business/SkillUploadDialog";
import { SkillDetailDialog } from "./SkillDetailDialog";
import { SkillMarketPane } from "./SkillMarketPane";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
import type { SkillDetailTarget } from "./SkillDetailDialog";
import type { SkillSummary } from "@/models/skillhub";
import type { TranslateFn } from "@/models/conversations";

type SkillTabID = "installed" | "market";

export function SkillsSection() {
  const controller = useWorkspaceControllerContext();
  const hub = controller.hubViewProps?.hub;
  const t = controller.t;
  const [tab, setTab] = useState<SkillTabID>("installed");
  const [query, setQuery] = useState("");
  const [detail, setDetail] = useState<SkillDetailTarget | null>(null);
  const [uploadOpen, setUploadOpen] = useState(false);

  const skills = useMemo(() => hub?.skills ?? [], [hub?.skills]);
  const remoteSkills = hub?.remoteSkills ?? [];
  const authed = isAuthenticated(controller.sidebarProps?.authStatus);
  const setRemoteEnabled = hub?.setRemoteSkillsEnabled;

  useEffect(() => {
    if (!setRemoteEnabled) {
      return;
    }
    setRemoteEnabled(tab === "market" && authed);
  }, [authed, setRemoteEnabled, tab]);

  async function installAndReveal(item: SkillSummary, replace: boolean) {
    const installed = await hub?.installRemoteSkill?.(item, { replace });
    if (installed) {
      setDetail(null);
      setTab("installed");
    }
  }

  const installedVisible = useMemo(() => skills.filter((skill) => matchesQuery(skill, query)), [query, skills]);

  const tabs = useMemo(
    () => [
      { id: "installed" as const, label: t("iaSkillsTabInstalled") },
      { id: "market" as const, label: t("iaSkillsTabMarket") },
    ],
    [t],
  );

  return (
    <SettingsRow
      title={
        <span>
          {t("iaSettingsCardSkills")}
          <span className={styles.countBadge}>{skills.length}</span>
        </span>
      }
      description={t("iaSettingsCardSkillsHint")}
      actions={
        <Button variant="primary" size="md" onClick={() => setUploadOpen(true)}>
          <Plus size={14} strokeWidth={2.2} aria-hidden="true" />
          {t("resourcesSkillUpload")}
        </Button>
      }
    >
      <ResourceFilterTabs label={t("resourcesSkillsLabel")} tabs={tabs} value={tab} onChange={setTab} />

      {tab === "installed" ? (
        <ResourceSearchField value={query} placeholder={t("iaSkillsInstalledSearchPlaceholder")} onChange={setQuery} />
      ) : (
        <ResourceSearchField
          value={hub?.remoteSkillsSearch || ""}
          placeholder={t("iaSkillsMarketSearchPlaceholder")}
          onChange={(value) => hub?.setRemoteSkillsSearch?.(value)}
        />
      )}

      {tab === "installed" ? (
        <>
          {hub?.skillsError ? (
            <p className="form-error" role="alert">
              {hub.skillsError}
            </p>
          ) : null}
          {/* Use resource-specific loading state; hub.loaded tracks templates. */}
          {!hub?.detailPaneProps?.loaded ? (
            <p className={styles.listNotice}>
              <LoaderCircle className="animate-spin" size={14} strokeWidth={2} aria-hidden="true" />
              {t("resourcesLoading")}
            </p>
          ) : installedVisible.length === 0 ? (
            <p className={styles.listNotice}>
              {skills.length === 0 ? t("resourcesSkillsEmpty") : t("iaListSearchEmpty")}
            </p>
          ) : (
            <ResourceList>
              {installedVisible.map((skill) => (
                <ResourceListCard
                  key={skill.name}
                  title={skill.name}
                  description={skill.description}
                  icon={
                    <ResourceFeaturedIcon>
                      <SkillFeaturedIcon />
                    </ResourceFeaturedIcon>
                  }
                  active={false}
                  onOpen={() => setDetail({ skill, variant: "installed" })}
                  actions={<OriginPill skill={skill} t={t} />}
                />
              ))}
            </ResourceList>
          )}
        </>
      ) : (
        <SkillMarketPane
          authed={authed}
          hub={hub}
          installedSkills={skills}
          onLogin={() => controller.sidebarProps?.onRequireOpenCSGAuthentication?.()}
          onInstall={(item, replace) => void installAndReveal(item, replace)}
          onOpenSkill={(skill) => setDetail({ skill, variant: "market" })}
          remoteSkills={remoteSkills}
          t={t}
        />
      )}

      <SkillUploadDialog
        open={uploadOpen}
        zipOnly
        busy={Boolean(hub?.uploadBusy)}
        error={hub?.uploadError || ""}
        installedSkills={skills}
        remoteInstallBusy={hub?.remoteInstallBusy || ""}
        remoteInstallError={hub?.remoteInstallError || ""}
        remoteSkills={remoteSkills}
        remoteSkillsError={hub?.remoteSkillsError || ""}
        remoteSkillsHasMore={Boolean(hub?.remoteSkillsHasMore)}
        remoteSkillsLoading={Boolean(hub?.remoteSkillsLoading)}
        remoteSkillsLoadingMore={Boolean(hub?.remoteSkillsLoadingMore)}
        remoteSkillsSearch={hub?.remoteSkillsSearch || ""}
        t={t}
        onInstallRemoteSkill={hub?.installRemoteSkill}
        onLoadMoreRemoteSkills={hub?.loadMoreRemoteSkills}
        onOpenChange={setUploadOpen}
        onRefreshRemoteSkills={hub?.refetchRemoteSkills}
        onRemoteSkillsSearchChange={hub?.setRemoteSkillsSearch}
        onRemoteVisibleChange={hub?.setRemoteSkillsEnabled}
        onSubmit={async (file: File) => {
          const uploaded = (await hub?.uploadSkill?.(file)) ?? null;
          if (uploaded) {
            setTab("installed");
          }
          return uploaded;
        }}
      />

      <SkillDetailDialog
        target={detail}
        t={t}
        market={
          detail?.variant === "market"
            ? {
                installed: hasSkillName(skills, remoteSkillInstallName(detail.skill)),
                busy: hub?.remoteInstallBusy === (detail.skill.remotePath || detail.skill.name),
                onAction: () =>
                  void installAndReveal(detail.skill, hasSkillName(skills, remoteSkillInstallName(detail.skill))),
              }
            : undefined
        }
        onOpenChange={(open) => (open ? undefined : setDetail(null))}
      />
    </SettingsRow>
  );
}

function OriginPill({ skill, t }: { skill: SkillSummary; t: TranslateFn }) {
  return (
    <span className={styles.tag}>
      <span className={styles.tagDot} aria-hidden="true" />
      {originLabel(skill, t)}
    </span>
  );
}

function matchesQuery(skill: { name: string; description?: string }, query: string): boolean {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle) {
    return true;
  }
  return [skill.name, skill.description].some((value) =>
    String(value ?? "")
      .toLocaleLowerCase()
      .includes(needle),
  );
}
