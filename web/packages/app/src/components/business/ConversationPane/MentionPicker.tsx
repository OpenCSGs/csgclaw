import avatarLayoutStyles from "@/components/business/AgentAvatar/AvatarLayout.module.css";
import messageLayoutStyles from "@/components/business/ConversationPane/MessageLayout.module.css";
import mentionPickerStyles from "./MentionPicker.module.css";
import { Fragment, useLayoutEffect, useRef } from "react";
import { File, Folder } from "lucide-react";
import { FileTypeIcon } from "@/components/business/FileTypeIcon";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { normalizeAgentAvatarPath } from "@/shared/avatarOptions";
import { avatarFallbackText } from "@/shared/avatar";
import { localizeRole } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";
import type { MentionPickerUser } from "./types";

export function MentionPersonAvatar({ user }: { user: MentionPickerUser }) {
  const avatar = normalizeAgentAvatarPath(user.avatar);
  return (
    <span
      className={`avatar ${avatarLayoutStyles["avatar"]} ${mentionPickerStyles["mention-person-avatar"]}`}
      aria-hidden="true"
    >
      <AgentAvatarContent avatar={avatar} fallback={avatarFallbackText(user.avatar, user.name, user.id)} />
    </span>
  );
}

export function MentionOptionText({ label, description }: { label: string; description: string }) {
  return (
    <span className={mentionPickerStyles["mention-text"]}>
      <span className={`message-author ${messageLayoutStyles["message-author"]}`} title={`@${label}`}>
        @{label}
      </span>
      <span className={`conversation-preview ${messageLayoutStyles["conversation-preview"]}`} title={description}>
        {description}
      </span>
    </span>
  );
}

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
        const Icon = directory ? Folder : File;
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
              {!file ? (
                <MentionPersonAvatar user={user} />
              ) : groupPeopleAndFiles ? (
                <span className={mentionPickerStyles["mention-icon"]} aria-hidden="true">
                  <Icon size={16} />
                </span>
              ) : (
                <span className={`avatar ${avatarLayoutStyles["avatar"]}`} aria-hidden="true">
                  <FileTypeIcon name={file.name} />
                </span>
              )}
              <MentionOptionText
                label={file && directory && groupPeopleAndFiles ? file.label : mentionLabel}
                description={
                  directory && groupPeopleAndFiles
                    ? t("chatDirectoryCompletionHint")
                    : `${file?.label ?? user.id}${showRole && !file ? ` · ${localizeRole(user.role || "", t)}` : ""}`
                }
              />
            </button>
          </Fragment>
        );
      })}
    </div>
  );
}
