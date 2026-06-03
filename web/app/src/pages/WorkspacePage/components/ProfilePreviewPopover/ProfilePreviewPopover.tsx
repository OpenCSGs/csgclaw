import { useLayoutEffect, useState } from "react";
import { X } from "lucide-react";
import {
  agentStatusLabel,
  agentModelID,
  formatProviderLabel,
  formatRuntimeKindLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentUpgradeNeeded,
  isAgentRunning,
} from "@/models/agents";
import { localizeRole } from "@/shared/i18n";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { Button, IconButton } from "@/components/ui";

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function profilePreviewStyle(anchorRect, cardHeight = 420) {
  const offset = 12;
  const viewportPadding = 16;
  const width = Math.min(360, window.innerWidth - viewportPadding * 2);
  const maxLeft = Math.max(viewportPadding, window.innerWidth - viewportPadding - width);
  const visibleHeight = Math.min(cardHeight, window.innerHeight - viewportPadding * 2);
  const maxTop = Math.max(viewportPadding, window.innerHeight - viewportPadding - visibleHeight);

  if (!anchorRect) {
    return { top: `${viewportPadding}px`, left: `${viewportPadding}px`, width: `${width}px` };
  }

  const hasRoomRight = anchorRect.right + offset + width <= window.innerWidth - viewportPadding;
  const preferredLeft = hasRoomRight ? anchorRect.right + offset : anchorRect.left - width - offset;
  const left = clamp(preferredLeft, viewportPadding, maxLeft);
  const top = clamp(anchorRect.top - 12, viewportPadding, maxTop);
  return { top: `${top}px`, left: `${left}px`, width: `${width}px` };
}

function previewFieldLabel(label) {
  return String(label || "").toLocaleUpperCase();
}

function agentModelWithReasoning(agent) {
  const model = agentModelID(agent);
  const reasoning = agent?.reasoning_effort || agent?.agent_profile?.reasoning_effort || "medium";
  return reasoning ? `${model}(${reasoning})` : model;
}

function isBootstrapAdminUser(user) {
  return (
    user?.id === "u-admin" ||
    String(user?.handle ?? "").toLowerCase() === "admin" ||
    String(user?.name ?? "").toLowerCase() === "admin"
  );
}

export function ProfilePreviewPopover({
  previewRef,
  agent,
  user,
  anchorRect,
  t,
  inDirectConversation,
  busyKey,
  onClose,
  onOpenAgent,
  onOpenDM,
  onDelete,
}) {
  const localAdminPreview = !agent && isBootstrapAdminUser(user);
  const showAgentMetadataFields = Boolean(agent || localAdminPreview);
  const running = agent ? isAgentRunning(agent) : false;
  const incomplete = agent ? isAgentIncomplete(agent) : false;
  const restartNeeded = agent ? isAgentRestartNeeded(agent) : false;
  const upgradeNeeded = agent ? isAgentUpgradeNeeded(agent) : false;
  const provider = agent?.provider || agent?.agent_profile?.provider;
  const previewRuntime = agent
    ? formatRuntimeKindLabel(agent.runtime_kind || agent.agent_profile?.runtime_kind, t)
    : t("profileLocalRuntime");
  const previewProvider = agent ? formatProviderLabel(provider) : t("profileLocalProvider");
  const previewModel = agent ? agentModelWithReasoning(agent) : localizeRole(user?.role || "admin", t);
  const displayName = agent?.name || user?.name || "";
  const displayRole = agent ? agent.role || "worker" : user?.role;
  const deleteBusy = agent ? busyKey === `${agent.id}:delete-bot` : false;
  const canOpenDM = !inDirectConversation;
  const [cardHeight, setCardHeight] = useState(420);

  useLayoutEffect(() => {
    const preview = previewRef?.current;
    if (!preview) {
      return;
    }
    const nextHeight = Math.ceil(preview.getBoundingClientRect().height);
    if (nextHeight > 0 && nextHeight !== cardHeight) {
      setCardHeight(nextHeight);
    }
  }, [previewRef, cardHeight, agent?.id, user?.id, inDirectConversation]);

  return (
    <aside
      ref={previewRef}
      className="profile-preview-popover"
      style={profilePreviewStyle(anchorRect, cardHeight)}
      aria-label={t("profilePreview")}
      role="dialog"
      aria-modal="false"
    >
      <div className="preview-header">
        <div className="preview-title">{t("profilePreview")}</div>
        <IconButton
          className="modal-close"
          icon={<X size={20} strokeWidth={2} />}
          label={t("close")}
          markClassName="modal-close-icon"
          onClick={onClose}
          variant="tertiaryGray"
        />
      </div>
      <div className="preview-hero">
        {agent ? (
          <div className="entity-avatar preview-avatar">
            <AgentAvatarContent avatar={agent.avatar} fallback={avatarFallbackText(agent.avatar, displayName, agent.id)} alt="" />
          </div>
        ) : (
          <div className="avatar preview-avatar">
            <AgentAvatarContent avatar={user.avatar} fallback={avatarFallbackText(user.avatar, user.name, user.handle, user.id)} />
          </div>
        )}
        <div className="preview-identity">
          <div className="preview-name">{displayName}</div>
          <div className="preview-meta">
            @{user?.handle || agent?.id || ""} · {localizeRole(displayRole, t)}
          </div>
        </div>
      </div>
      {agent?.description ? <p className="preview-description">{agent.description}</p> : null}
      {showAgentMetadataFields ? (
        <>
          <div className="preview-fields">
            <div className="entity-field">
              <span>{previewFieldLabel(t("status"))}</span>
              <strong>{agent ? agentStatusLabel(agent.status, t) : t("online")}</strong>
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
                <span className={`agent-badge ${incomplete ? "warn" : ""}`}>
                  {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
                </span>
                {upgradeNeeded ? <span className="agent-badge warn">{t("profileUpgradeRequired")}</span> : null}
                {restartNeeded ? <span className="agent-badge warn">{t("profileRestartRequired")}</span> : null}
              </div>
              <div className="preview-actions">
                <Button variant="primary" size="md" onClick={() => onOpenAgent(agent)}>
                  {t("openProfile")}
                </Button>
                {canOpenDM ? (
                  <Button variant="secondaryGray" size="md" onClick={() => onOpenDM(agent)}>
                    {t("openDM")}
                  </Button>
                ) : null}
                {agent.role !== "manager" && agent.id !== "u-manager" ? (
                  <Button variant="danger" size="md" disabled={deleteBusy} onClick={() => onDelete(agent)}>
                    {t("agentDelete")}
                  </Button>
                ) : null}
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
            <strong>{localizeRole(user?.role, t)}</strong>
          </div>
          <div className="entity-field">
            <span>{previewFieldLabel(t("handleLabel"))}</span>
            <strong>{user?.handle ? `@${user.handle}` : "-"}</strong>
          </div>
          <div className="entity-field">
            <span>{previewFieldLabel(t("userIDLabel"))}</span>
            <strong>{user?.id || ""}</strong>
          </div>
        </div>
      )}
    </aside>
  );
}
