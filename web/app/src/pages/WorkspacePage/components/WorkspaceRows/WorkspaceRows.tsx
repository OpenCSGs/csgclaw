import {
  agentModelID,
  formatProviderLabel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentRunning,
  notificationBotMetaLabel,
} from "@/models/agents";
import {
  agentMatchesUser,
  formatConversationPreview,
  formatMessagePreviewText,
  formatThreadReplyCount,
  formatTime,
  isDirectConversation,
  resolveConversationUser,
} from "@/models/conversations";
import { AgentIcon, ChevronIcon, ComputerIcon, RoomPlusIcon, RoomsIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";
import type { DragEvent, ReactNode } from "react";

export type WorkspaceGroupProps = {
  addLabel?: string;
  children: ReactNode;
  collapsed: boolean;
  count: number;
  dragOver?: boolean;
  dragging?: boolean;
  id: string;
  onAdd?: () => void;
  onDragEnd?: (event: DragEvent<HTMLElement>) => void;
  onDragLeave?: (event: DragEvent<HTMLElement>) => void;
  onDragOver?: (event: DragEvent<HTMLElement>) => void;
  onDragStart?: (event: DragEvent<HTMLElement>) => void;
  onDrop?: (event: DragEvent<HTMLElement>) => void;
  onToggle: () => void;
  title: string;
};

export function WorkspaceGroup({
  id,
  title,
  count,
  collapsed,
  dragging = false,
  dragOver = false,
  onToggle,
  onAdd,
  addLabel,
  onDragEnd,
  onDragLeave,
  onDragOver,
  onDragStart,
  onDrop,
  children,
}: WorkspaceGroupProps) {
  const itemsID = `workspace-group-items-${id || String(title).toLowerCase().replace(/\s+/g, "-")}`;
  const draggable = Boolean(onDragStart);
  return (
    <section
      className={`workspace-group ${collapsed ? "collapsed" : ""} ${draggable ? "workspace-group-sortable" : ""} ${
        dragging ? "dragging" : ""
      } ${dragOver ? "drag-over" : ""}`.trim()}
      onDragLeave={onDragLeave}
      onDragOver={onDragOver}
      onDrop={onDrop}
    >
      <div className="workspace-group-head" draggable={draggable} onDragEnd={onDragEnd} onDragStart={onDragStart}>
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
        <div className="workspace-group-actions">
          {onAdd ? (
            <Button
              variant="ghost"
              className="workspace-add-button"
              draggable={false}
              aria-label={addLabel || title}
              title={addLabel || title}
              onDragStart={(event) => event.stopPropagation()}
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
  const meta = notification
    ? notificationBotMetaLabel(item, t)
    : `${formatProviderLabel(item.provider || item.agent_profile?.provider)} · ${agentModelID(item)}`;
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
  agents = [],
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
  const directAgent = isDirect && displayUser ? agents.find((item) => agentMatchesUser(item, displayUser)) : null;
  const directAgentRunning = isAgentRunning(directAgent);
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
        <span className="workspace-row-title-line">
          <span className="workspace-row-title truncate">{title}</span>
          {directAgent ? (
            <span className={`workspace-status-dot ${directAgentRunning ? "online" : ""}`} aria-hidden="true"></span>
          ) : null}
        </span>
        <span className="workspace-row-meta truncate">
          {formatConversationPreview(lastMessage, conversation, currentUserID, usersById, locale, t)}
        </span>
      </span>
      <span className="workspace-row-time">{formatTime(lastMessage?.created_at, locale)}</span>
    </button>
  );
}

export function WorkspaceThreadRow({ conversation, thread, active, locale, t, onSelect }) {
  const root = thread?.root;
  const rootID = thread?.summary?.root_id || root?.id;
  if (!root || !rootID) {
    return null;
  }
  const latestReply = thread?.summary?.latest_reply;
  const title = formatMessagePreviewText(thread?.summary?.context_summary?.root_excerpt || root.content);
  const meta = latestReply
    ? `${t("latestThreadReply")}: ${formatMessagePreviewText(latestReply.content)}`
    : formatThreadReplyCount(thread?.summary?.reply_count, t);
  const updatedAt = latestReply?.created_at || root.created_at;

  return (
    <button
      className={`workspace-row thread-nav-row ${active ? "active" : ""}`}
      title={title}
      onClick={() => onSelect(conversation.id, root)}
    >
      <span className="workspace-row-icon">
        <RoomsIcon />
      </span>
      <span className="workspace-row-main">
        <span className="workspace-row-title truncate">{title}</span>
        <span className="workspace-row-meta truncate">{meta}</span>
      </span>
      <span className="workspace-row-time">{formatTime(updatedAt, locale)}</span>
    </button>
  );
}
