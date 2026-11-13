import { useWorkspaceSkillsQuery } from "@/hooks/workspace/workspaceQueries";
import { InstalledSkillLibrary } from "./InstalledSkillLibrary";
import { useEffect, useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { useSkillUpdatesQuery } from "@/hooks/workspace/useSkillUpdates";
import { InlineError, Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import { hasSkillName, remoteSkillInstallName } from "@/models/skillhub";
import { ResourceFilterTabs, ResourceSearchField } from "@/components/business/HubDetailPane";
import { SkillUploadDialog } from "@/components/business/SkillUploadDialog";
import { SkillDetailDialog } from "./SkillDetailDialog";
import { SkillMarketPane } from "./SkillMarketPane";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
import type { SkillDetailTarget } from "./SkillDetailDialog";
import type { SkillSummary } from "@/models/skillhub";

type SkillTabID = "installed" | "market";

export function SkillsSection() {
  const controller = useWorkspaceControllerContext();
  const hub = controller.hubViewProps?.hub;
  const t = controller.t;
  const [tab, setTab] = useState<SkillTabID>("installed");
  const [query, setQuery] = useState("");
  const [detail, setDetail] = useState<SkillDetailTarget | null>(null);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [checked, setChecked] = useState(false);
  const updatesQuery = useSkillUpdatesQuery();
  const updates = updatesQuery.data?.items ?? [];
  const available = updates.filter((item) => item.available);
  const checkFailed = updatesQuery.isError || updates.some((item) => item.error);

  const skillsQuery = useWorkspaceSkillsQuery({ refetchInterval: 30000 });
  const skills = useMemo(() => skillsQuery.data ?? hub?.skills ?? [], [skillsQuery.data, hub?.skills]);
  const remoteSkills = hub?.remoteSkills ?? [];
  const authed = isAuthenticated(controller.sidebarProps?.authStatus);
  const count = tab === "installed" ? skills.length : authed ? hub?.remoteSkillsTotal : null;
  const setRemoteEnabled = hub?.setRemoteSkillsEnabled;

  useEffect(() => {
    if (!setRemoteEnabled) {
      return;
    }
    setRemoteEnabled(tab === "market" && authed);
  }, [authed, setRemoteEnabled, tab]);

  async function installFromMarket(item: SkillSummary, replace: boolean) {
    const installed = await hub?.installRemoteSkill?.(item, { replace });
    if (installed) {
      setDetail(null);
    }
  }

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
          {count != null ? <span className={styles.countBadge}>{count}</span> : null}
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
            <InlineError as="p" role="alert">
              {hub.skillsError}
            </InlineError>
          ) : null}
          <InstalledSkillLibrary
            skills={skills}
            query={query}
            onOpen={(skill) => setDetail({ skill, variant: "installed" })}
            onRefresh={() => skillsQuery.refetch()}
            busy={skillsQuery.isFetching}
            loading={!skillsQuery.isFetched}
            t={t}
            updates={updates}
            onScanRemote={() => {
              setChecked(true);
              void updatesQuery.refetch();
            }}
            scanningRemote={updatesQuery.isFetching}
            updateAction={
              available.length ? (
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => void hub?.requestSkillUpdates(available.map((item) => item.name))}
                >
                  {t("skillUpdatesReviewAll", { count: available.length })}
                </Button>
              ) : null
            }
            updateNotice={
              <>
                {checked && !updatesQuery.isFetching && !available.length && !checkFailed ? (
                  <span role="status">{t("skillUpdatesUpToDate")}</span>
                ) : null}
                {checkFailed ? <InlineError role="alert">{t("skillUpdatesCheckFailed")}</InlineError> : null}
              </>
            }
          />
        </>
      ) : (
        <SkillMarketPane
          authed={authed}
          hub={hub}
          installedSkills={skills}
          onLogin={() => controller.sidebarProps?.onRequireOpenCSGAuthentication?.()}
          onInstall={(item, replace) => void installFromMarket(item, replace)}
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
        update={detail?.variant === "installed" ? updates.find((item) => item.name === detail.skill.name) : undefined}
        onRequestUpdate={(name) => {
          setDetail(null);
          void hub?.requestSkillUpdates([name]);
        }}
        target={
          detail?.variant === "installed"
            ? { ...detail, skill: skills.find((skill) => skill.name === detail.skill.name) ?? detail.skill }
            : detail
        }
        t={t}
        market={
          detail?.variant === "market"
            ? {
                installed: hasSkillName(skills, remoteSkillInstallName(detail.skill)),
                busy: hub?.remoteInstallBusy === (detail.skill.remotePath || detail.skill.name),
                onAction: () =>
                  void installFromMarket(detail.skill, hasSkillName(skills, remoteSkillInstallName(detail.skill))),
              }
            : undefined
        }
        onOpenChange={(open) => (open ? undefined : setDetail(null))}
      />
    </SettingsRow>
  );
}
