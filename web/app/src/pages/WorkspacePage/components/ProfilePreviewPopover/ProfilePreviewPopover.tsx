import { useLayoutEffect, useState } from "react";
import {
  agentModelID,
  formatProviderLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentRunning,
} from "@/models/agents";
import { localizeRole } from "@/shared/i18n";
import { AgentIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";

export function profilePreviewStyle(anchorRect, cardHeight = 420) {
  const offset = 12;
  const viewportPadding = 12;
  const width = Math.min(360, window.innerWidth - 24);
  const preferRight = anchorRect ? anchorRect.right + offset + width <= window.innerWidth - viewportPadding : true;
  const left = anchorRect
    ? preferRight
      ? Math.max(viewportPadding, anchorRect.right + offset)
      : Math.max(viewportPadding, anchorRect.left - width - offset)
    : viewportPadding;
  const maxTop = Math.max(
    viewportPadding,
    window.innerHeight - viewportPadding - Math.min(cardHeight, window.innerHeight - viewportPadding * 2),
  );
  const top = anchorRect ? Math.min(Math.max(viewportPadding, anchorRect.top - 12), maxTop) : viewportPadding;
  return { top: `${top}px`, left: `${left}px`, width: `${width}px` };
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
  const running = agent ? isAgentRunning(agent) : false;
  const incomplete = agent ? isAgentIncomplete(agent) : false;
  const restartNeeded = agent ? isAgentRestartNeeded(agent) : false;
  const provider = agent?.provider || agent?.agent_profile?.provider;
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
    >
      <div className="preview-header">
        <div className="preview-title">{agent ? t("profilePreview") : t("personProfile")}</div>
        <Button variant="secondaryGray" size="md" className="modal-close" aria-label={t("close")} onClick={onClose}>
          <span aria-hidden="true">×</span>
        </Button>
      </div>
      <div className="preview-hero">
        {agent ? (
          <div className="entity-avatar preview-avatar">
            <AgentIcon />
          </div>
        ) : (
          <div
            className="avatar preview-avatar"
            style={{ background: `linear-gradient(135deg, ${user.accent_hex}, #10233f)` }}
          >
            {user.avatar}
          </div>
        )}
        <div className="preview-identity">
          <div className="preview-name">{displayName}</div>
          <div className="preview-meta">
            @{user?.handle || agent?.id || ""} · {localizeRole(displayRole, t)}
          </div>
        </div>
      </div>
      {agent?.description || user?.name ? <p className="preview-description">{agent?.description || ""}</p> : null}
      {agent ? (
        <>
          <div className="preview-fields">
            <div className="entity-field">
              <span>{t("status")}</span>
              <strong>{agent.status || "unknown"}</strong>
            </div>
            <div className="entity-field">
              <span>{t("profileProvider")}</span>
              <strong>{formatProviderLabel(provider)}</strong>
            </div>
            <div className="entity-field">
              <span>{t("profileModel")}</span>
              <strong>{agentModelID(agent)}</strong>
            </div>
            <div className="entity-field">
              <span>{t("profileReasoning")}</span>
              <strong>{agent.reasoning_effort || agent.agent_profile?.reasoning_effort || "medium"}</strong>
            </div>
          </div>
          <div className="entity-badge-row">
            <span className={`agent-badge ${running ? "" : "warn"}`}>{running ? t("online") : t("offline")}</span>
            <span className={`agent-badge ${incomplete ? "warn" : ""}`}>
              {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
            </span>
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
      ) : (
        <div className="preview-fields">
          <div className="entity-field">
            <span>{t("status")}</span>
            <strong>{t("online")}</strong>
          </div>
          <div className="entity-field">
            <span>{t("roleLabel")}</span>
            <strong>{localizeRole(user?.role, t)}</strong>
          </div>
          <div className="entity-field">
            <span>{t("handleLabel")}</span>
            <strong>{user?.handle ? `@${user.handle}` : "-"}</strong>
          </div>
          <div className="entity-field">
            <span>{t("userIDLabel")}</span>
            <strong>{user?.id || ""}</strong>
          </div>
        </div>
      )}
    </aside>
  );
}
