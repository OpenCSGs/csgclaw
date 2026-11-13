import { RecommendationBadge } from "@/components/business/RecommendationBadge";
import { LoaderCircle, LogIn, Plus, RefreshCw } from "lucide-react";
import { InlineError, Button, Tooltip } from "@/components/ui";
import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { hasSkillName, remoteSkillInstallName } from "@/models/skillhub";
import { ResourceFeaturedIcon, ResourceSearchField, SkillFeaturedIcon } from "@/components/business/HubDetailPane";
import accountSectionStyles from "./AccountSection.module.css";
import resourceSettingsStyles from "./ResourceSettings.module.css";
import type { SkillSummary } from "@/models/skillhub";
import type { TranslateFn } from "@/models/conversations";
import { skillVersionLabel } from "@/models/skillLibrary";

type SkillsHub = NonNullable<ReturnType<typeof useWorkspaceControllerContext>["hubViewProps"]>["hub"];

export function SkillMarketPane({
  authed,
  hub,
  installedSkills,
  onInstall,
  onLogin,
  onOpenSkill,
  remoteSkills,
  rows = false,
  t,
}: {
  authed: boolean;
  hub: SkillsHub | undefined;
  rows?: boolean;
  installedSkills: readonly SkillSummary[];
  onInstall: (item: SkillSummary, replace: boolean) => void;
  onLogin: () => void;
  onOpenSkill: (skill: SkillSummary) => void;
  remoteSkills: readonly SkillSummary[];
  t: TranslateFn;
}) {
  if (!authed) {
    return (
      <div className={accountSectionStyles.loginGate}>
        <LogIn size={20} strokeWidth={1.8} aria-hidden="true" />
        <p>{t("iaSkillsMarketLoginRequired")}</p>
        <Button variant="primary" size="md" onClick={onLogin}>
          {t("modelProviderOpenCSGAuthorize")}
        </Button>
      </div>
    );
  }

  if (hub?.remoteSkillsError) {
    return (
      <div className={resourceSettingsStyles.listNotice}>
        <InlineError as="span" role="alert">
          {hub.remoteSkillsError}
        </InlineError>
        <Button size="sm" variant="secondaryGray" onClick={() => void hub?.refetchRemoteSkills?.()}>
          <RefreshCw size={14} strokeWidth={2} aria-hidden="true" />
          {t("resourcesSkillRemoteRefresh")}
        </Button>
      </div>
    );
  }

  if (hub?.remoteSkillsLoading && remoteSkills.length === 0) {
    return (
      <p className={resourceSettingsStyles.listNotice}>
        <LoaderCircle className="animate-spin" size={14} strokeWidth={2} aria-hidden="true" />
        {t("resourcesSkillRemoteSkillsLoading")}
      </p>
    );
  }

  if (remoteSkills.length === 0) {
    return <p className={resourceSettingsStyles.listNotice}>{t("resourcesSkillRemoteSkillsEmpty")}</p>;
  }

  if (rows) {
    return (
      <>
        <ResourceSearchField
          value={hub?.remoteSkillsSearch || ""}
          placeholder={t("iaSkillsMarketSearchPlaceholder")}
          onChange={(value) => hub?.setRemoteSkillsSearch?.(value)}
        />
        <div className="agent-skill-candidates-list" role="list">
          {remoteSkills.map((item) => {
            const installKey = item.remotePath || item.name;
            const installName = remoteSkillInstallName(item);
            const installed = hasSkillName(installedSkills, installName);
            const installing = hub?.remoteInstallBusy === installKey;
            return (
              <div key={installKey} className="agent-skill-candidate is-static">
                <ResourceFeaturedIcon>
                  <SkillFeaturedIcon />
                </ResourceFeaturedIcon>
                <button type="button" className="agent-skill-candidate-copy" onClick={() => onOpenSkill(item)}>
                  <span className="agent-skill-name">
                    <span>{item.name}</span>
                    {item.recommended ? <RecommendationBadge label={t("resourceRecommended")} /> : null}
                  </span>
                  <span className="agent-skill-description">{item.description || item.name}</span>
                </button>
                <Button
                  variant={installed ? "secondaryGray" : "primary"}
                  size="sm"
                  loading={installing}
                  disabled={Boolean(hub?.remoteInstallBusy)}
                  onClick={() => onInstall(item, installed)}
                >
                  {installed ? t("iaSkillsUpdate") : t("resourcesSkillRemoteInstallAction")}
                </Button>
              </div>
            );
          })}
        </div>
        {hub?.remoteSkillsHasMore ? (
          <div className={resourceSettingsStyles.listMore}>
            <Button
              size="sm"
              variant="secondaryGray"
              loading={Boolean(hub?.remoteSkillsLoadingMore)}
              onClick={() => void hub?.loadMoreRemoteSkills?.()}
            >
              {t("iaSkillsMarketLoadMore")}
            </Button>
          </div>
        ) : null}
        {hub?.remoteInstallError ? (
          <InlineError as="p" role="alert">
            {hub.remoteInstallError}
          </InlineError>
        ) : null}
      </>
    );
  }

  return (
    <>
      <ResourceList>
        {remoteSkills.map((item) => {
          const installKey = item.remotePath || item.name;
          const installName = remoteSkillInstallName(item);
          const installed = hasSkillName(installedSkills, installName);
          const installing = hub?.remoteInstallBusy === installKey;
          return (
            <ResourceListCard
              key={installKey}
              title={item.name}
              badge={
                <span className="flex flex-wrap items-center gap-2 text-xs">
                  {item.recommended ? <RecommendationBadge label={t("resourceRecommended")} /> : null}
                  {item.version ? <span>{skillVersionLabel(item.version)}</span> : null}
                </span>
              }
              description={item.description || item.name}
              icon={
                <ResourceFeaturedIcon>
                  <SkillFeaturedIcon />
                </ResourceFeaturedIcon>
              }
              active={false}
              onOpen={() => onOpenSkill(item)}
              actions={
                installed ? (
                  <Tooltip content={t("iaSkillsUpdate")}>
                    <span>
                      <Button
                        iconOnly
                        size="sm"
                        variant="secondaryGray"
                        aria-label={t("iaSkillsUpdate")}
                        disabled={Boolean(hub?.remoteInstallBusy)}
                        onClick={() => onInstall(item, true)}
                      >
                        <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                      </Button>
                    </span>
                  </Tooltip>
                ) : (
                  <Tooltip content={t("resourcesSkillRemoteInstallAction")}>
                    <span>
                      <Button
                        iconOnly
                        size="sm"
                        variant="secondaryGray"
                        loading={installing}
                        aria-label={t("resourcesSkillRemoteInstallAction")}
                        disabled={Boolean(hub?.remoteInstallBusy)}
                        onClick={() => onInstall(item, false)}
                      >
                        <Plus size={16} strokeWidth={2} aria-hidden="true" />
                      </Button>
                    </span>
                  </Tooltip>
                )
              }
            />
          );
        })}
      </ResourceList>

      {hub?.remoteSkillsHasMore ? (
        <div className={resourceSettingsStyles.listMore}>
          <Button
            size="sm"
            variant="secondaryGray"
            loading={Boolean(hub?.remoteSkillsLoadingMore)}
            onClick={() => void hub?.loadMoreRemoteSkills?.()}
          >
            {t("iaSkillsMarketLoadMore")}
          </Button>
        </div>
      ) : null}

      {hub?.remoteInstallError ? (
        <InlineError as="p" role="alert">
          {hub.remoteInstallError}
        </InlineError>
      ) : null}
    </>
  );
}
