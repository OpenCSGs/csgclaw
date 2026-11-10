import avatarLayoutStyles from "@/components/business/AgentAvatar/AvatarLayout.module.css";
import messageLayoutStyles from "@/components/business/ConversationPane/MessageLayout.module.css";
import mentionPickerStyles from "./MentionPicker.module.css";
import { useLayoutEffect, useRef } from "react";
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
  t: TranslateFn;
  users?: MentionPickerUser[];
};

export function MentionPicker({
  users = [],
  activeIndex = 0,
  className = "",
  showRole = true,
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
        return (
          <button
            key={user.id}
            ref={index === activeIndex ? activeOptionRef : null}
            role="option"
            aria-selected={index === activeIndex}
            className={`${mentionPickerStyles["mention-option"]} ${index === activeIndex ? "active" : ""}`}
            onMouseDown={(event) => {
              event.preventDefault();
              onSelect(user);
            }}
          >
            <span className={`avatar ${avatarLayoutStyles["avatar"]}`}>
              <AgentAvatarContent
                avatar={user.avatar}
                fallback={avatarFallbackText(user.avatar, mentionLabel, user.id)}
              />
            </span>
            <div>
              <div className={`message-author ${messageLayoutStyles["message-author"]}`}>@{mentionLabel}</div>
              <div className={`conversation-preview ${messageLayoutStyles["conversation-preview"]}`}>
                {user.id}
                {showRole ? ` · ${localizeRole(user.role || "", t)}` : ""}
              </div>
            </div>
          </button>
        );
      })}
    </div>
  );
}
