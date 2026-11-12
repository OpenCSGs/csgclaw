import avatarLayoutStyles from "@/components/business/AgentAvatar/AvatarLayout.module.css";
import messageLayoutStyles from "@/components/business/ConversationPane/MessageLayout.module.css";
import mentionPickerStyles from "./MentionPicker.module.css";
import { Fragment, useLayoutEffect, useRef } from "react";
import { File, Folder, User } from "lucide-react";
import { FileTypeIcon } from "@/components/business/FileTypeIcon";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { localizeRole } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";
import type { MentionPickerUser } from "./types";

export type MentionPickerProps = {
  activeIndex?: number;
  className?: string;
  onSelect: (user: MentionPickerUser) => void;
  showRole?: boolean;
  groupPeopleAndFiles?: boolean;
  t: TranslateFn;
  users?: MentionPickerUser[];
};

export function MentionPicker({
  users = [],
  activeIndex = 0,
  className = "",
  showRole = true,
  groupPeopleAndFiles = false,
  t,
  onSelect,
}: MentionPickerProps) {
  const activeOptionRef = useRef<HTMLButtonElement | null>(null);
  const activeUserID = users[activeIndex]?.id || "";

  useLayoutEffect(() => {
    activeOptionRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, activeUserID, users.length]);

  return (
    <div className={`mention-picker ${mentionPickerStyles["mention-picker"]} ${className}`.trim()} role="listbox">
      {users.map((user, index) => {
        const mentionLabel = user.name || user.id;
        const file = user.fileReference;
        const directory = file?.label.endsWith("/");
        const Icon = file ? (directory ? Folder : File) : User;
        const startsGroup = index === 0 || Boolean(users[index - 1].fileReference) !== Boolean(file);
        return (
          <Fragment key={user.id}>
            {groupPeopleAndFiles && startsGroup ? (
              <div className={mentionPickerStyles["mention-group"]} aria-hidden="true">
                {t(file ? "chatMentionFiles" : "chatMentionPeople")}
              </div>
            ) : null}
            <button
              type="button"
              ref={index === activeIndex ? activeOptionRef : null}
              role="option"
              aria-selected={index === activeIndex}
              className={`${mentionPickerStyles["mention-option"]} ${index === activeIndex ? "active" : ""}`}
              onMouseDown={(event) => {
                event.preventDefault();
                onSelect(user);
              }}
            >
              {groupPeopleAndFiles ? (
                <span className={mentionPickerStyles["mention-icon"]} aria-hidden="true">
                  <Icon size={16} />
                </span>
              ) : (
                <span className={`avatar ${avatarLayoutStyles["avatar"]}`}>
                  {file ? (
                    <FileTypeIcon name={file.name} />
                  ) : (
                    <AgentAvatarContent
                      avatar={user.avatar}
                      fallback={avatarFallbackText(user.avatar, mentionLabel, user.id)}
                    />
                  )}
                </span>
              )}
              <div>
                <div className={`message-author ${messageLayoutStyles["message-author"]}`}>
                  @{file && directory && groupPeopleAndFiles ? file.label : mentionLabel}
                </div>
                <div className={`conversation-preview ${messageLayoutStyles["conversation-preview"]}`}>
                  {directory && groupPeopleAndFiles ? t("chatDirectoryCompletionHint") : (file?.label ?? user.id)}
                  {showRole && !file ? ` · ${localizeRole(user.role || "", t)}` : ""}
                </div>
              </div>
            </button>
          </Fragment>
        );
      })}
    </div>
  );
}
