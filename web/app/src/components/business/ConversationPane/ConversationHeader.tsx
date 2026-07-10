import { memo } from "react";
import type { ReactNode, RefObject } from "react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { Button } from "@/components/ui";
import { AddUserIcon, IconImage, TrashIcon, UsersIcon, WrenchIcon } from "@/components/ui/Icons";
import type { AgentLike } from "@/models/agents";
import type { IMConversation, IMUser, TranslateFn } from "@/models/conversations";
import { isDirectConversation } from "@/models/conversations";
import { avatarFallbackText } from "@/shared/avatar";
import { localizeRole } from "@/shared/i18n";
import type { BooleanStateSetter } from "./types";

export type ConversationHeaderProps = {
  channelToolsRef: RefObject<HTMLDivElement | null>;
  conversation: IMConversation;
  conversationMembers: IMUser[];
  description?: string;
  headerAccessory?: ReactNode;
  inviteActionLabel: string;
  logAgent?: AgentLike | null;
  logModalOpen: boolean;
  memberMenuRef?: RefObject<HTMLDivElement | null>;
  onClearMessages: () => void;
  onDeleteRoom: () => void;
  onInviteAction: () => void;
  onOpenAgentLogs: () => void;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onToggleChannelTools: BooleanStateSetter;
  onToggleMemberList?: BooleanStateSetter;
  onToggleToolCalls: BooleanStateSetter;
  selectedMessageCount: number;
  selectedVisibleMessageCount?: number;
  showChannelTools: boolean;
  showInviteAction: boolean;
  showMemberList?: boolean;
  showMemberListAction?: boolean;
  showToolCalls: boolean;
  t: TranslateFn;
};

export const ConversationHeader = memo(function ConversationHeader({
  channelToolsRef,
  conversation,
  conversationMembers,
  description,
  headerAccessory,
  inviteActionLabel,
  logAgent,
  logModalOpen,
  memberMenuRef,
  selectedMessageCount,
  selectedVisibleMessageCount,
  showChannelTools,
  showInviteAction,
  showMemberList = false,
  showMemberListAction = true,
  showToolCalls,
  t,
  onClearMessages,
  onDeleteRoom,
  onInviteAction,
  onOpenAgentLogs,
  onPreviewUser,
  onToggleChannelTools,
  onToggleMemberList,
  onToggleToolCalls,
}: ConversationHeaderProps) {
  const visibleCount = selectedVisibleMessageCount ?? selectedMessageCount;
  const messageCountLabel =
    visibleCount === selectedMessageCount ? String(selectedMessageCount) : `${visibleCount}/${selectedMessageCount}`;

  return (
    <header className="chat-header">
      <div className="chat-header-main">
        <div className="chat-title-bar">
          <div className="chat-title-row">
            <div className="chat-title-group">
              <div className="chat-kicker">
                <span>{isDirectConversation(conversation) ? t("directMessagesSection") : t("conversationLabel")}</span>
                <strong title={messageCountLabel}>{messageCountLabel}</strong>
              </div>
              <div className="chat-title truncate">{conversation.title}</div>
              {showMemberListAction ? (
                <div ref={memberMenuRef} className="header-menu">
                  <Button
                    className="member-badge-button"
                    active={showMemberList}
                    aria-label={t("membersTitle")}
                    aria-pressed={showMemberList}
                    title={t("membersTitle")}
                    onClick={() => {
                      onToggleMemberList?.((value) => !value);
                      onToggleChannelTools(false);
                    }}
                  >
                    <span className="icon-button-mark" aria-hidden="true">
                      <UsersIcon />
                    </span>
                    <span className="member-badge-count">{conversationMembers.length}</span>
                  </Button>
                  {showMemberList ? (
                    <div className="header-popover members-popover">
                      <div className="header-popover-title">{t("membersTitle")}</div>
                      <div className="members-popover-list">
                        {conversationMembers.map((user) => (
                          <div key={user.id} className="member-row">
                            <button
                              type="button"
                              className="avatar avatar-button"
                              aria-label={`${t("profilePreview")} ${user.name}`}
                              onClick={(event) => onPreviewUser(user, event.currentTarget)}
                            >
                              <AgentAvatarContent
                                avatar={user.avatar}
                                fallback={avatarFallbackText(user.avatar, user.name, user.id)}
                              />
                            </button>
                            <div className="member-row-main">
                              <div className="member-row-name">{user.name}</div>
                              <div className="member-row-meta">
                                {user.id} · {localizeRole(user.role || "", t)}
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>
              ) : null}
            </div>
          </div>
          <div className="chat-title-actions">
            {headerAccessory}
            {logAgent ? (
              <Button
                className="icon-button log-button"
                active={logModalOpen}
                iconOnly
                size="lg"
                variant="secondaryGray"
                aria-label={t("agentLogs")}
                data-tooltip={t("agentLogs")}
                data-tooltip-side="bottom"
                onClick={onOpenAgentLogs}
              >
                <span className="icon-button-mark" aria-hidden="true">
                  {IconImage("log")}
                </span>
              </Button>
            ) : null}
            <div ref={channelToolsRef} className="header-menu tools-menu">
              <Button
                className="icon-button"
                active={showChannelTools}
                iconOnly
                size="lg"
                variant="secondaryGray"
                aria-label={t("channelTools")}
                aria-expanded={showChannelTools}
                data-tooltip={t("channelTools")}
                data-tooltip-side="bottom"
                onClick={() => {
                  onToggleChannelTools((value) => !value);
                  onToggleMemberList?.(false);
                }}
              >
                <span className="icon-button-mark">
                  <WrenchIcon />
                </span>
              </Button>
              {showChannelTools ? (
                <div className="header-popover tools-popover">
                  <div className="header-popover-title">{t("channelTools")}</div>
                  <Button className="tool-menu-row" onClick={() => onToggleToolCalls((value) => !value)}>
                    <span>{showToolCalls ? t("toggleToolCallsHide") : t("toggleToolCallsShow")}</span>
                    <strong>{showToolCalls ? t("enabled") : t("disabled")}</strong>
                  </Button>
                  <Button variant="outlineDanger" className="tool-menu-row danger" onClick={onClearMessages}>
                    <span>{t("clearRoomMessages")}</span>
                    <span className="tool-menu-icon" aria-hidden="true">
                      <TrashIcon />
                    </span>
                  </Button>
                  {!isDirectConversation(conversation) ? (
                    <Button variant="outlineDanger" className="tool-menu-row danger" onClick={onDeleteRoom}>
                      <span>{t("deleteRoom")}</span>
                      <span className="tool-menu-icon" aria-hidden="true">
                        <TrashIcon />
                      </span>
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </div>
            {showInviteAction ? (
              <Button
                className="icon-button member-management-button"
                iconOnly
                size="lg"
                variant="secondaryGray"
                aria-label={inviteActionLabel}
                data-tooltip={inviteActionLabel}
                data-tooltip-side="bottom"
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  onInviteAction();
                }}
              >
                <span className="icon-button-mark">
                  <AddUserIcon />
                </span>
              </Button>
            ) : null}
          </div>
        </div>
        {description ? <div className="chat-subtitle">{description}</div> : null}
      </div>
    </header>
  );
});
