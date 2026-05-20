import {
  agentModelID,
  formatProviderLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentRunning,
  notificationBotMetaLabel,
} from "@/models/agents";
import {
  formatConversationPreview,
  formatTime,
  isDirectConversation,
  resolveConversationUser,
} from "@/models/conversations";
import { AgentIcon, ChevronIcon, ComputerIcon, RoomPlusIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";
import type { ReactNode } from "react";

export type WorkspaceGroupProps = {
  addLabel?: string;
  children: ReactNode;
  collapsed: boolean;
  count: number;
  id: string;
  onAdd?: () => void;
  onToggle: () => void;
  title: string;
};

export function WorkspaceGroup({
  id,
  title,
  count,
  collapsed,
  onToggle,
  onAdd,
  addLabel,
  children,
}: WorkspaceGroupProps) {
  const itemsID = `workspace-group-items-${id || String(title).toLowerCase().replace(/\s+/g, "-")}`;
  return (
    <section className={`workspace-group ${collapsed ? "collapsed" : ""}`}>
      <div className="workspace-group-head">
        <button
          className="workspace-group-toggle"
          type="button"
          aria-expanded={!collapsed}
          aria-controls={itemsID}
          onClick={onToggle}
        >
          <span className="workspace-group-arrow" aria-hidden="true">
            <ChevronIcon />
          </span>
          <span className="workspace-group-title">
            <span>{title}</span>
            <small>{count}</small>
          </span>
        </button>
        {onAdd ? (
          <Button
            variant="ghost"
            className="workspace-add-button"
            aria-label={addLabel || title}
            title={addLabel || title}
            onClick={(event) => {
              event.preventDefault();
              event.stopPropagation();
              onAdd?.();
            }}
          >
            <span className="icon-button-mark" aria-hidden="true">
              <RoomPlusIcon />
            </span>
          </Button>
        ) : null}
      </div>
      {collapsed ? null : (
        <div id={itemsID} className="workspace-group-items">
          {children}
        </div>
      )}
    </section>
  );
}

export function WorkspaceComputerRow({ title, active, subtitle, onSelect }) {
  return (
    <button className={`workspace-row computer-row ${active ? "active" : ""}`} onClick={onSelect}>
      <span className="workspace-row-icon">
        <ComputerIcon />
      </span>
      <span className="workspace-row-main">
        <span className="workspace-row-title truncate">{title}</span>
        <span className="workspace-row-meta truncate">{subtitle}</span>
      </span>
      <span className="workspace-status-dot online" aria-hidden="true"></span>
    </button>
  );
}

export function WorkspaceAgentRow({ item, active, t, onSelect, onPreview, notification = false }) {
  const incomplete = isAgentIncomplete(item);
  const restartNeeded = isAgentRestartNeeded(item);
  const running = isAgentRunning(item);
  const meta = notification ? notificationBotMetaLabel(item, t) : `${formatProviderLabel(item.provider || item.agent_profile?.provider)} · ${agentModelID(item)}`;
  return (
    <button
      className={`workspace-row agent-nav-row ${active ? "active" : ""} ${incomplete ? "warn" : ""}`.trim()}
      onClick={() => onSelect(item)}
    >
      <span
        className="workspace-row-icon workspace-row-icon-clickable"
        role="button"
        tabIndex={0}
        aria-label={`${t("profilePreview")} ${item.name}`}
        onClick={(event) => {
          event.stopPropagation();
          onPreview?.(item, event.currentTarget);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            event.stopPropagation();
            onPreview?.(item, event.currentTarget);
          }
        }}
      >
        <AgentIcon />
      </span>
      <span className="workspace-row-main">
        <span className="workspace-row-title-line">
          <span className="workspace-row-title truncate">{item.name}</span>
          <span className={`workspace-status-dot ${running ? "online" : ""}`} aria-hidden="true"></span>
        </span>
        <span className="workspace-row-meta truncate">{meta}</span>
      </span>
      <span className="workspace-row-badges">
        {incomplete ? <span className="mini-badge warn">{t("profileIncompleteBadge")}</span> : null}
        {restartNeeded ? <span className="mini-badge warn">{t("profileRestartRequired")}</span> : null}
      </span>
    </button>
  );
}

export function WorkspaceConversationRow({
  conversation,
  active,
  currentUserID,
  usersById,
  locale,
  t,
  onSelect,
  onPreviewUser,
}) {
  const lastMessage = conversation.messages[conversation.messages.length - 1];
  const isDirect = isDirectConversation(conversation);
  const displayUser = isDirect ? resolveConversationUser(conversation, currentUserID, usersById) : null;
  const title = isDirect && displayUser ? displayUser.name : conversation.title;
  const icon = isDirect && displayUser ? displayUser.avatar : "#";
  return (
    <button
      className={`workspace-row conversation-nav-row ${active ? "active" : ""}`}
      onClick={() => onSelect(conversation.id)}
    >
      <span
        className={`workspace-row-icon ${isDirect ? "avatar-icon workspace-row-icon-clickable" : ""}`}
        role={isDirect ? "button" : undefined}
        tabIndex={isDirect ? 0 : undefined}
        aria-label={isDirect && displayUser ? `${t("profilePreview")} ${displayUser.name}` : undefined}
        onClick={
          isDirect && displayUser
            ? (event) => {
                event.stopPropagation();
                onPreviewUser?.(displayUser, event.currentTarget);
              }
            : undefined
        }
        onKeyDown={
          isDirect && displayUser
            ? (event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  event.stopPropagation();
                  onPreviewUser?.(displayUser, event.currentTarget);
                }
              }
            : undefined
        }
      >
        {icon}
      </span>
      <span className="workspace-row-main">
        <span className="workspace-row-title truncate">{title}</span>
        <span className="workspace-row-meta truncate">
          {formatConversationPreview(lastMessage, conversation, currentUserID, usersById, locale, t)}
        </span>
      </span>
      <span className="workspace-row-time">{formatTime(lastMessage?.created_at, locale)}</span>
    </button>
  );
}
