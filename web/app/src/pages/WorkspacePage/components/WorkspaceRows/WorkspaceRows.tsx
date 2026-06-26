import {
  agentProfileConfig,
  agentModelID,
  formatProviderLabel,
  hasConnectedAgentChannel,
  isAgentIncomplete,
  isAgentRestartNeeded,
  isAgentUpgradeNeeded,
  isAgentRunning,
  notificationBotMetaLabel,
} from "@/models/agents";
import { providerNameForProviderID } from "@/models/modelProviders";
import {
  agentMatchesUser,
  formatConversationPreview,
  formatMessagePreviewText,
  formatThreadReplyCount,
  formatTime,
  hasConnectedHumanChannel,
  isDirectConversation,
  resolveConversationUser,
} from "@/models/conversations";
import { MessagePreviewText } from "@/components/business/MessageContent";
import { ChevronIcon, ComputerIcon, RoomPlusIcon, RoomsIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { localizeRole } from "@/shared/i18n";
import { RoomAvatar, resolveRoomAvatarMembers } from "@/components/business/RoomAvatar";
import type { DragEvent, ReactNode } from "react";
import type { AgentLike } from "@/models/agents";
import type {
  IMConversation,
  IMMessage,
  IMUser,
  LocaleCode,
  ThreadView,
  TranslateFn,
  UsersById,
} from "@/models/conversations";

export type WorkspaceGroupProps = {
  addIcon?: ReactNode;
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
  addIcon,
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
              data-tooltip={addLabel || title}
              data-tooltip-side="bottom"
              data-tooltip-align="end"
              onDragStart={(event) => event.stopPropagation()}
              onClick={(event) => {
                event.preventDefault();
                event.stopPropagation();
                onAdd?.();
              }}
            >
              <span className="icon-button-mark" aria-hidden="true">
                {addIcon || <RoomPlusIcon />}
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

export type WorkspaceComputerRowProps = {
  active: boolean;
  onSelect: () => void;
  subtitle: string;
  title: string;
};

export function WorkspaceComputerRow({ title, active, subtitle, onSelect }: WorkspaceComputerRowProps) {
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

export type WorkspaceHumanRowProps = {
  active: boolean;
  onPreview?: (user: IMUser, anchor: HTMLElement) => void;
  onSelect: (user: IMUser) => void;
  t: TranslateFn;
  user: IMUser;
};

export function WorkspaceHumanRow({ user, active, t, onPreview, onSelect }: WorkspaceHumanRowProps) {
  const displayName = user.name || user.id;
  const role = localizeRole(user.role || "admin", t);
  const feishuConnected = hasConnectedHumanChannel(user, "feishu");

  return (
    <button className={`workspace-row human-nav-row ${active ? "active" : ""}`} onClick={() => onSelect(user)}>
      <span
        className="workspace-row-icon avatar-icon workspace-row-icon-clickable"
        role="button"
        tabIndex={0}
        aria-label={`${t("profilePreview")} ${displayName}`}
        onClick={(event) => {
          event.stopPropagation();
          onPreview?.(user, event.currentTarget);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            event.stopPropagation();
            onPreview?.(user, event.currentTarget);
          }
        }}
      >
        <AgentAvatarContent avatar={user.avatar} fallback={avatarFallbackText(user.avatar, displayName, user.id)} />
      </span>
      <span className="workspace-row-main">
        <span className="workspace-row-title truncate">{displayName}</span>
        <span className="workspace-row-meta truncate">
          {user.id} · {role}
        </span>
      </span>
      <span className="workspace-row-badges">
        {feishuConnected ? (
          <span
            className="workspace-channel-badge feishu-channel-badge"
            aria-label={t("feishuConnected")}
            title={t("feishuConnected")}
          >
            <img src="icons/feishu.png" alt="" />
          </span>
        ) : null}
      </span>
    </button>
  );
}

export type WorkspaceAgentRowProps = {
  active: boolean;
  item: AgentLike;
  notification?: boolean;
  onPreview?: (item: AgentLike, anchor: HTMLElement) => void;
  onSelect: (item: AgentLike) => void;
  t: TranslateFn;
};

export function WorkspaceAgentRow({
  item,
  active,
  t,
  onSelect,
  onPreview,
  notification = false,
}: WorkspaceAgentRowProps) {
  const incomplete = isAgentIncomplete(item);
  const restartNeeded = isAgentRestartNeeded(item);
  const upgradeNeeded = isAgentUpgradeNeeded(item);
  const running = isAgentRunning(item);
  const feishuConnected = hasConnectedAgentChannel(item, "feishu");
  const profile = agentProfileConfig(item);
  const provider = item.provider || profile?.provider || providerNameForProviderID(profile?.model_provider_id || "");
  const meta = notification
    ? notificationBotMetaLabel(item, t)
    : `${formatProviderLabel(provider)} · ${agentModelID(item)}`;
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
        <AgentAvatarContent avatar={item.avatar} fallback={avatarFallbackText(item.avatar, item.name, item.id)} />
      </span>
      <span className="workspace-row-main">
        <span className="workspace-row-title-line">
          <span className="workspace-row-title truncate">{item.name}</span>
          <span className={`workspace-status-dot ${running ? "online" : ""}`} aria-hidden="true"></span>
        </span>
        <span className="workspace-row-meta truncate">{meta}</span>
      </span>
      <span className="workspace-row-badges">
        {feishuConnected ? (
          <span
            className="workspace-channel-badge feishu-channel-badge"
            aria-label={t("feishuConnected")}
            title={t("feishuConnected")}
          >
            <img src="icons/feishu.png" alt="" />
          </span>
        ) : null}
        {incomplete ? <span className="mini-badge warn">{t("profileIncompleteBadge")}</span> : null}
        {upgradeNeeded ? <span className="mini-badge warn">{t("profileUpgradeRequired")}</span> : null}
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
}: {
  active: boolean;
  agents?: AgentLike[];
  conversation: IMConversation;
  currentUserID: string;
  locale: LocaleCode;
  onPreviewUser?: (user: IMUser, anchor: HTMLElement) => void;
  onSelect: (id: string) => void;
  t: TranslateFn;
  usersById: UsersById;
}) {
  const lastMessage = conversation.messages[conversation.messages.length - 1];
  const isDirect = isDirectConversation(conversation);
  const displayUser = isDirect ? resolveConversationUser(conversation, currentUserID, usersById) : null;
  const directAgent = isDirect && displayUser ? agents.find((item) => agentMatchesUser(item, displayUser)) : null;
  const directAgentRunning = isAgentRunning(directAgent);
  const directAvatar = displayUser?.avatar || directAgent?.avatar || "";
  const directAvatarFallback = directAgent
    ? avatarFallbackText(displayUser?.avatar || directAgent.avatar, directAgent.name, directAgent.id)
    : avatarFallbackText(displayUser?.avatar, displayUser?.name, displayUser?.id);
  const title = isDirect && displayUser ? displayUser.name : conversation.title;
  const roomAvatarMembers = resolveRoomAvatarMembers(conversation, usersById, currentUserID);
  const preview = formatConversationPreview(lastMessage, conversation, currentUserID, usersById, locale, t);
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
        {isDirect && displayUser ? (
          <AgentAvatarContent avatar={directAvatar} fallback={directAvatarFallback} />
        ) : (
          <RoomAvatar members={roomAvatarMembers} count={conversation.members.length} />
        )}
      </span>
      <span className="workspace-row-main">
        <span className="workspace-row-title-line">
          <span className="workspace-row-title truncate">{title}</span>
          {directAgent ? (
            <span className={`workspace-status-dot ${directAgentRunning ? "online" : ""}`} aria-hidden="true"></span>
          ) : null}
        </span>
        <span className="workspace-row-meta truncate">
          <MessagePreviewText content={preview} />
        </span>
      </span>
      <span className="workspace-row-time">{formatTime(lastMessage?.created_at, locale)}</span>
    </button>
  );
}

export type WorkspaceThreadRowProps = {
  active: boolean;
  conversation: IMConversation;
  locale: LocaleCode;
  onSelect: (conversationID: string, message: IMMessage) => void | Promise<void>;
  t: TranslateFn;
  thread: ThreadView;
};

export function WorkspaceThreadRow({ conversation, thread, active, locale, t, onSelect }: WorkspaceThreadRowProps) {
  const root = thread?.root;
  const rootID = thread?.summary?.root_id || root?.id;
  if (!root || !rootID) {
    return null;
  }
  const latestReply = thread?.summary?.latest_reply;
  const title = formatMessagePreviewText(thread?.summary?.context_summary?.root_excerpt || root.content);
  const latestReplyText = latestReply ? formatMessagePreviewText(latestReply.content) : "";
  const meta = latestReply ? null : formatThreadReplyCount(thread?.summary?.reply_count, t);
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
        <span className="workspace-row-title truncate" title={title}>
          <MessagePreviewText content={title} />
        </span>
        <span className="workspace-row-meta truncate">
          {latestReply ? (
            <>
              <span>{`${t("latestThreadReply")}: `}</span>
              <MessagePreviewText content={latestReplyText} />
            </>
          ) : (
            meta
          )}
        </span>
      </span>
      <span className="workspace-row-time">{formatTime(updatedAt, locale)}</span>
    </button>
  );
}
