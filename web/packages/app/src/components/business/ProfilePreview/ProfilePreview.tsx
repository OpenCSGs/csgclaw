import { EntityField, EntityAvatar } from "@/components/business/EntityDetail";
import avatarLayoutStyles from "@/components/business/AgentAvatar/AvatarLayout.module.css";
import previewStyles from "./ProfilePreview.module.css";
import {
  agentModelID,
  agentAvailabilityStatusLabel,
  agentProfileConfig,
  agentRuntimeKind,
  formatProviderLabel,
  formatRuntimeKindLabel,
  isAgentAvailable,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentUpgradeNeeded,
} from "@/models/agents";
import { providerNameForProviderID } from "@/models/modelProviders";
import { localizeRole } from "@/shared/i18n";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { reasoningEffortLabel } from "@/components/business/ProfileControls";
import { avatarFallbackText } from "@/shared/avatar";
import { StatusBadge, Button } from "@/components/ui";
import type { AgentLike } from "@/models/agents";
import type { IMUser, TranslateFn } from "@/models/conversations";

export type ProfilePreviewAnchorRect = {
  bottom: number;
  left: number;
  right: number;
  top: number;
};

export type ProfilePreviewContentProps = {
  agent: AgentLike | null;
  onOpenAgent: (item: AgentLike) => void;
  onOpenDM: (item: AgentLike) => void | Promise<void>;
  t: TranslateFn;
  user: IMUser | null;
};

function previewFieldLabel(label: string): string {
  return String(label || "").toLocaleUpperCase();
}

function agentModelWithReasoning(agent: AgentLike, t: TranslateFn): string {
  const model = agentModelID(agent);
  const profile = agentProfileConfig(agent);
  const reasoning = reasoningEffortLabel(t, agent?.reasoning_effort || profile?.reasoning_effort);
  return reasoning ? `${model}(${reasoning})` : model;
}

function isBootstrapAdminUser(user: IMUser | null | undefined): boolean {
  return user?.id === "u-admin" || String(user?.name ?? "").toLowerCase() === "admin";
}

export function ProfilePreviewContent({ agent, user, t, onOpenAgent, onOpenDM }: ProfilePreviewContentProps) {
  const cloud = user?.role === "cloud";
  const localAdminPreview = !agent && isBootstrapAdminUser(user);
  const showAgentMetadataFields = Boolean(agent || localAdminPreview);
  const running = agent ? isAgentAvailable(agent) : cloud && user.is_online === true;
  const incomplete = agent ? isAgentIncomplete(agent) : false;
  const restartNeeded = agent ? isAgentRestartNeeded(agent) : false;
  const upgradeNeeded = agent ? isAgentUpgradeNeeded(agent) : false;
  const profile = agentProfileConfig(agent);
  const provider = agent?.provider || profile?.provider || providerNameForProviderID(profile?.model_provider_id || "");
  const previewRuntime = agent ? formatRuntimeKindLabel(agentRuntimeKind(agent), t) : t("profileLocalRuntime");
  const previewProvider = agent ? formatProviderLabel(provider) : t("profileLocalProvider");
  const previewModel = agent ? agentModelWithReasoning(agent, t) : localizeRole(user?.role || "admin", t);
  const displayName = agent?.name || user?.name || "";
  const displayRole = agent ? agent.role || "worker" : user?.role || "";
  const statusLabel = user?.agent_state
    ? t(user.agent_state === "deleted" ? "agentStateDeleted" : "agentStateArchived")
    : agent
      ? agentAvailabilityStatusLabel(agent, t)
      : cloud
        ? t(user.is_online == null ? "cloudChatStatusUnknown" : running ? "agentPlazaOnline" : "agentPlazaOffline")
        : t("online");

  return (
    <>
      <div className={`preview-hero ${previewStyles["preview-hero"]}`}>
        {agent ? (
          <EntityAvatar className={`preview-avatar ${previewStyles["preview-avatar"]}`}>
            <AgentAvatarContent
              avatar={agent.avatar}
              fallback={avatarFallbackText(agent.avatar, displayName, agent.id)}
              alt=""
            />
          </EntityAvatar>
        ) : (
          <div className={`avatar ${avatarLayoutStyles["avatar"]} preview-avatar ${previewStyles["preview-avatar"]}`}>
            <AgentAvatarContent
              avatar={user?.avatar}
              fallback={avatarFallbackText(user?.avatar, user?.name, user?.id)}
            />
          </div>
        )}
        <div className={`preview-identity ${previewStyles["preview-identity"]}`}>
          <div className={`preview-name-line ${previewStyles["preview-name-line"]}`}>
            <div className={`preview-name ${previewStyles["preview-name"]}`}>{displayName}</div>
            <span
              className={`preview-presence ${previewStyles["preview-presence"]} ${running || localAdminPreview ? "online" : ""}`}
            >
              <span className={`preview-presence-dot ${previewStyles["preview-presence-dot"]}`} aria-hidden="true" />
              {statusLabel}
            </span>
          </div>
          <div className={`preview-meta ${previewStyles["preview-meta"]}`}>
            {user?.id || agent?.id || ""} · {localizeRole(displayRole, t)}
          </div>
        </div>
      </div>
      {agent?.description ? (
        <p className={`preview-description ${previewStyles["preview-description"]}`}>{agent.description}</p>
      ) : null}
      {showAgentMetadataFields ? (
        <>
          <div className={`preview-fields ${previewStyles["preview-fields"]}`}>
            <EntityField label={previewFieldLabel(t("profileRuntimeKind"))}>
              <strong title={previewRuntime}>{previewRuntime}</strong>
            </EntityField>
            <EntityField label={previewFieldLabel(t("profileProvider"))}>
              <strong title={previewProvider}>{previewProvider}</strong>
            </EntityField>
            <EntityField label={previewFieldLabel(t("profileModel"))}>
              <strong title={previewModel}>{previewModel}</strong>
            </EntityField>
          </div>
          {agent ? (
            <>
              <div className={`entity-badge-row ${previewStyles["entity-badge-row"]}`}>
                <StatusBadge
                  kind="compact"
                  tone={incomplete ? "warning" : "positive"}
                  className={`${incomplete ? "warn" : "ready"}`}
                >
                  {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
                </StatusBadge>
                {upgradeNeeded ? (
                  <StatusBadge kind="compact" tone="warning" className={"warn"}>
                    {t("profileUpgradeRequired")}
                  </StatusBadge>
                ) : null}
                {restartNeeded ? (
                  <StatusBadge kind="compact" tone="warning" className={"warn"}>
                    {t("profileRestartRequired")}
                  </StatusBadge>
                ) : null}
              </div>
              <div className={`preview-actions ${previewStyles["preview-actions"]}`}>
                <Button variant="primary" size="sm" onClick={() => onOpenAgent(agent)}>
                  {t("openProfile")}
                </Button>
                <Button variant="secondaryGray" size="sm" onClick={() => onOpenDM(agent)}>
                  {t("openDM")}
                </Button>
              </div>
            </>
          ) : null}
        </>
      ) : (
        <div className={`preview-fields ${previewStyles["preview-fields"]}`}>
          <EntityField label={previewFieldLabel(t("roleLabel"))}>
            <strong title={localizeRole(user?.role || "", t)}>{localizeRole(user?.role || "", t)}</strong>
          </EntityField>
          <EntityField label={previewFieldLabel(t("userIDLabel"))}>
            <strong title={user?.id || ""}>{user?.id || ""}</strong>
          </EntityField>
        </div>
      )}
    </>
  );
}
