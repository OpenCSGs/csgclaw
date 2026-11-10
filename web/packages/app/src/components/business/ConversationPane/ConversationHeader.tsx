import avatarLayoutStyles from "@/components/business/AgentAvatar/AvatarLayout.module.css";
import conversationHeaderStyles from "./ConversationHeader.module.css";
import { memo } from "react";
import type { ReactNode, RefObject } from "react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { IconButton, Button, Tooltip, PanelHeader } from "@/components/ui";
import { AddUserIcon, TrashIcon, UsersIcon, WrenchIcon } from "@/components/business/Icons";
import type { IMConversation, IMUser, TranslateFn, UsersById } from "@/models/conversations";
import {
  isDirectConversation,
  localIdentitiesMatch,
  resolveConversationUser,
  resolveUserByLocalIdentity,
  userDisplayName,
} from "@/models/conversations";
import { avatarFallbackText } from "@/shared/avatar";
import { localizeRole } from "@/shared/i18n";
import type { BooleanStateSetter } from "./types";

export type ConversationHeaderProps = {
  channelToolsRef: RefObject<HTMLDivElement | null>;
  conversation: IMConversation;
  conversationMembers: IMUser[];
  currentUserID: string;
  description?: string;
  headerAccessory?: ReactNode;
  inviteActionLabel: string;
  memberMenuRef?: RefObject<HTMLDivElement | null>;
  onClearMessages: () => void;
  onDeleteRoom: () => void;
  onInviteAction: () => void;
  onPreviewUser: (user: IMUser, anchor: HTMLElement) => void;
  onToggleChannelTools: BooleanStateSetter;
  onToggleMemberList?: BooleanStateSetter;
  onToggleToolCalls: BooleanStateSetter;
  selectedMessageCount: number;
  selectedVisibleMessageCount?: number;
  showChannelTools: boolean;
  showChannelToolsAction?: boolean;
  showInviteAction: boolean;
  showMemberList?: boolean;
  showMemberListAction?: boolean;
  showToolCalls: boolean;
  memberManagementOpen?: boolean;
  t: TranslateFn;
  usersById: UsersById;
};

export const ConversationHeader = memo(function ConversationHeader({
  channelToolsRef,
  conversation,
  conversationMembers,
  currentUserID,
  description,
  headerAccessory,
  inviteActionLabel,
  memberMenuRef,
  selectedMessageCount,
  selectedVisibleMessageCount,
  showChannelTools,
  showChannelToolsAction = true,
  showInviteAction,
  showMemberList = false,
  showMemberListAction = true,
  showToolCalls,
  memberManagementOpen = false,
  t,
  usersById,
  onClearMessages,
  onDeleteRoom,
  onInviteAction,
  onPreviewUser,
  onToggleChannelTools,
  onToggleMemberList,
  onToggleToolCalls,
}: ConversationHeaderProps) {
  const visibleCount = selectedVisibleMessageCount ?? selectedMessageCount;
  const messageCountLabel =
    visibleCount === selectedMessageCount ? String(selectedMessageCount) : `${visibleCount}/${selectedMessageCount}`;
  const isDirect = isDirectConversation(conversation);
  const peer = isDirect
    ? conversation.cloud_agent
      ? resolveUserByLocalIdentity(conversation.cloud_agent.user_id, usersById)
      : resolveConversationUser(conversation, currentUserID, usersById)
    : undefined;
  const peerName =
    peer?.name?.trim() && !localIdentitiesMatch(peer.id, currentUserID)
      ? userDisplayName(peer.id, usersById).trim()
      : "";
  const headerName = isDirect ? peerName : t("iaRoomsTab");

  return (
    <PanelHeader className={`chat-header ${conversationHeaderStyles["chat-header"]}`}>
      <div className={conversationHeaderStyles["chat-header-main"]}>
        <div className={`chat-title-bar ${conversationHeaderStyles["chat-title-bar"]}`}>
          <div className={`chat-title-row ${conversationHeaderStyles["chat-title-row"]}`}>
            <div className={`chat-title-group ${conversationHeaderStyles["chat-title-group"]}`}>
              <div className={`chat-kicker ${conversationHeaderStyles["chat-kicker"]}`}>
                {headerName ? (
                  <span className="truncate" title={headerName}>
                    {headerName}
                  </span>
                ) : (
                  <>
                    <span>{isDirectConversation(conversation) ? t("directMessagesSection") : t("iaRoomsTab")}</span>
                    <Tooltip content={messageCountLabel}>
                      <strong>{messageCountLabel}</strong>
                    </Tooltip>
                  </>
                )}
              </div>
              <Tooltip
                content={conversation.title}
                contentProps={{
                  align: "start",
                  className: conversationHeaderStyles["chat-title-tooltip"],
                  side: "bottom",
                  sideOffset: 6,
                }}
              >
                <div className={`chat-title ${conversationHeaderStyles["chat-title"]} truncate`} tabIndex={0}>
                  {conversation.title}
                </div>
              </Tooltip>
              {showMemberListAction ? (
                <div ref={memberMenuRef} className={conversationHeaderStyles["header-menu"]}>
                  <Tooltip content={t("membersTitle")}>
                    <Button
                      className={`member-badge-button ${conversationHeaderStyles["member-badge-button"]}`}
                      active={showMemberList}
                      aria-label={t("membersTitle")}
                      aria-pressed={showMemberList}
                      onClick={() => {
                        onToggleMemberList?.((value) => !value);
                        onToggleChannelTools(false);
                      }}
                    >
                      <span className={`icon-button-mark ${conversationHeaderStyles.memberMark}`} aria-hidden="true">
                        <UsersIcon />
                      </span>
                      <span className={conversationHeaderStyles["member-badge-count"]}>
                        {conversationMembers.length}
                      </span>
                    </Button>
                  </Tooltip>
                  {showMemberList ? (
                    <div
                      className={`header-popover ${conversationHeaderStyles["header-popover"]} ${conversationHeaderStyles["members-popover"]}`}
                    >
                      <div className={`header-popover-title ${conversationHeaderStyles["header-popover-title"]}`}>
                        {t("membersTitle")}
                      </div>
                      <div className={conversationHeaderStyles["members-popover-list"]}>
                        {conversationMembers.map((user) => (
                          <div key={user.id} className={conversationHeaderStyles["member-row"]}>
                            <button
                              type="button"
                              className={`avatar ${avatarLayoutStyles["avatar"]} avatar-button ${avatarLayoutStyles["avatar-button"]}`}
                              aria-label={`${t("profilePreview")} ${user.name}`}
                              onClick={(event) => onPreviewUser(user, event.currentTarget)}
                            >
                              <AgentAvatarContent
                                avatar={user.avatar}
                                fallback={avatarFallbackText(user.avatar, user.name, user.id)}
                              />
                            </button>
                            <div className={conversationHeaderStyles["member-row-main"]}>
                              <div className={conversationHeaderStyles["member-row-name"]}>{user.name}</div>
                              <div className={conversationHeaderStyles["member-row-meta"]}>
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
          <div className={`chat-title-actions ${conversationHeaderStyles["chat-title-actions"]}`}>
            {showInviteAction ? (
              <Tooltip content={inviteActionLabel} contentProps={{ side: "bottom" }}>
                <IconButton
                  icon={<AddUserIcon />}
                  iconSize={18}
                  markClassName="icon-button-mark"
                  className="icon-button member-management-button"
                  size="lg"
                  variant="secondaryGray"
                  label={inviteActionLabel}
                  active={memberManagementOpen}
                  aria-pressed={memberManagementOpen}
                  onClick={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    onInviteAction();
                  }}
                />
              </Tooltip>
            ) : null}
            {headerAccessory}
            {showChannelToolsAction ? (
              <div ref={channelToolsRef} className={`${conversationHeaderStyles["header-menu"]} tools-menu`}>
                <Tooltip content={t("channelTools")} contentProps={{ side: "bottom" }}>
                  <IconButton
                    icon={<WrenchIcon />}
                    iconSize={18}
                    markClassName="icon-button-mark"
                    className="icon-button"
                    active={showChannelTools}
                    size="lg"
                    variant="secondaryGray"
                    label={t("channelTools")}
                    aria-expanded={showChannelTools}
                    onClick={() => {
                      onToggleChannelTools((value) => !value);
                      onToggleMemberList?.(false);
                    }}
                  />
                </Tooltip>
                {showChannelTools ? (
                  <div
                    className={`header-popover ${conversationHeaderStyles["header-popover"]} ${conversationHeaderStyles["tools-popover"]}`}
                  >
                    <div className={`header-popover-title ${conversationHeaderStyles["header-popover-title"]}`}>
                      {t("channelTools")}
                    </div>
                    <Button
                      className={`tool-menu-row ${conversationHeaderStyles["tool-menu-row"]}`}
                      onClick={() => onToggleToolCalls((value) => !value)}
                    >
                      <span>{showToolCalls ? t("toggleToolCallsHide") : t("toggleToolCallsShow")}</span>
                      <strong>{showToolCalls ? t("enabled") : t("disabled")}</strong>
                    </Button>
                    <Button
                      variant="outlineDanger"
                      className={`tool-menu-row ${conversationHeaderStyles["tool-menu-row"]} danger`}
                      onClick={onClearMessages}
                    >
                      <span>{t("clearRoomMessages")}</span>
                      <span
                        className={`tool-menu-icon ${conversationHeaderStyles["tool-menu-icon"]}`}
                        aria-hidden="true"
                      >
                        <TrashIcon />
                      </span>
                    </Button>
                    {!isDirectConversation(conversation) ? (
                      <Button
                        variant="outlineDanger"
                        className={`tool-menu-row ${conversationHeaderStyles["tool-menu-row"]} danger`}
                        onClick={onDeleteRoom}
                      >
                        <span>{t("deleteRoom")}</span>
                        <span
                          className={`tool-menu-icon ${conversationHeaderStyles["tool-menu-icon"]}`}
                          aria-hidden="true"
                        >
                          <TrashIcon />
                        </span>
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
        {description ? (
          <div className={`chat-subtitle ${conversationHeaderStyles["chat-subtitle"]}`}>{description}</div>
        ) : null}
      </div>
    </PanelHeader>
  );
});
