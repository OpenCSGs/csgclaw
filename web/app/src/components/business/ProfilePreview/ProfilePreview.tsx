import {
  agentModelID,
  agentProfileConfig,
  agentRuntimeKind,
  agentRuntimeState,
  agentStatusLabel,
  formatProviderLabel,
  formatRuntimeKindLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentRunning,
  isAgentUpgradeNeeded,
} from "@/models/agents";
import { providerNameForProviderID } from "@/models/modelProviders";
import { localizeRole } from "@/shared/i18n";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { Button } from "@/components/ui";
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

function agentModelWithReasoning(agent: AgentLike): string {
  const model = agentModelID(agent);
  const profile = agentProfileConfig(agent);
  const reasoning = agent?.reasoning_effort || profile?.reasoning_effort || "medium";
  return reasoning ? `${model}(${reasoning})` : model;
}

function isBootstrapAdminUser(user: IMUser | null | undefined): boolean {
  return user?.id === "u-admin" || String(user?.name ?? "").toLowerCase() === "admin";
}

export function ProfilePreviewContent({ agent, user, t, onOpenAgent, onOpenDM }: ProfilePreviewContentProps) {
  const localAdminPreview = !agent && isBootstrapAdminUser(user);
  const showAgentMetadataFields = Boolean(agent || localAdminPreview);
  const running = agent ? isAgentRunning(agent) : false;
  const incomplete = agent ? isAgentIncomplete(agent) : false;
  const restartNeeded = agent ? isAgentRestartNeeded(agent) : false;
  const upgradeNeeded = agent ? isAgentUpgradeNeeded(agent) : false;
  const profile = agentProfileConfig(agent);
  const provider = agent?.provider || profile?.provider || providerNameForProviderID(profile?.model_provider_id || "");
  const previewRuntime = agent ? formatRuntimeKindLabel(agentRuntimeKind(agent), t) : t("profileLocalRuntime");
  const previewProvider = agent ? formatProviderLabel(provider) : t("profileLocalProvider");
  const previewModel = agent ? agentModelWithReasoning(agent) : localizeRole(user?.role || "admin", t);
  const displayName = agent?.name || user?.name || "";
  const displayRole = agent ? agent.role || "worker" : user?.role || "";

  return (
    <>
      <div className="preview-hero">
        {agent ? (
          <div className="entity-avatar preview-avatar">
            <AgentAvatarContent
              avatar={agent.avatar}
              fallback={avatarFallbackText(agent.avatar, displayName, agent.id)}
              alt=""
            />
          </div>
        ) : (
          <div className="avatar preview-avatar">
            <AgentAvatarContent
              avatar={user?.avatar}
              fallback={avatarFallbackText(user?.avatar, user?.name, user?.id)}
            />
          </div>
        )}
        <div className="preview-identity">
          <div className="preview-name">{displayName}</div>
          <div className="preview-meta">
            {user?.id || agent?.id || ""} · {localizeRole(displayRole, t)}
          </div>
        </div>
      </div>
      {agent?.description ? <p className="preview-description">{agent.description}</p> : null}
      {showAgentMetadataFields ? (
        <>
          <div className="preview-fields">
            <div className="entity-field">
              <span>{previewFieldLabel(t("status"))}</span>
              <strong>{agent ? agentStatusLabel(agentRuntimeState(agent), t) : t("online")}</strong>
            </div>
            <div className="entity-field">
              <span>{previewFieldLabel(t("profileRuntimeKind"))}</span>
              <strong>{previewRuntime}</strong>
            </div>
            <div className="entity-field">
              <span>{previewFieldLabel(t("profileProvider"))}</span>
              <strong>{previewProvider}</strong>
            </div>
            <div className="entity-field">
              <span>{previewFieldLabel(t("profileModel"))}</span>
              <strong>{previewModel}</strong>
            </div>
          </div>
          {agent ? (
            <>
              <div className="entity-badge-row">
                <span className={`agent-badge ${running ? "" : "warn"}`}>{running ? t("online") : t("offline")}</span>
                <span className={`agent-badge ${incomplete ? "warn" : "ready"}`}>
                  {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
                </span>
                {upgradeNeeded ? <span className="agent-badge warn">{t("profileUpgradeRequired")}</span> : null}
                {restartNeeded ? <span className="agent-badge warn">{t("profileRestartRequired")}</span> : null}
              </div>
              <div className="preview-actions">
                <Button variant="primary" size="md" onClick={() => onOpenAgent(agent)}>
                  {t("openProfile")}
                </Button>
                <Button variant="secondaryGray" size="md" onClick={() => onOpenDM(agent)}>
                  {t("openDM")}
                </Button>
              </div>
            </>
          ) : null}
        </>
      ) : (
        <div className="preview-fields">
          <div className="entity-field">
            <span>{previewFieldLabel(t("status"))}</span>
            <strong>{t("online")}</strong>
          </div>
          <div className="entity-field">
            <span>{previewFieldLabel(t("roleLabel"))}</span>
            <strong>{localizeRole(user?.role || "", t)}</strong>
          </div>
          <div className="entity-field">
            <span>{previewFieldLabel(t("userIDLabel"))}</span>
            <strong>{user?.id || ""}</strong>
          </div>
        </div>
      )}
    </>
  );
}
