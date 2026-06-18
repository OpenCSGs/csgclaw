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
    <div className={`mention-picker ${className}`.trim()} role="listbox">
      {users.map((user, index) => (
        <button
          key={user.id}
          ref={index === activeIndex ? activeOptionRef : null}
          role="option"
          aria-selected={index === activeIndex}
          className={`mention-option ${index === activeIndex ? "active" : ""}`}
          onMouseDown={(event) => {
            event.preventDefault();
            onSelect(user);
          }}
        >
          <span className="avatar">
            <AgentAvatarContent
              avatar={user.avatar}
              fallback={avatarFallbackText(user.avatar, user.name, user.handle, user.id)}
            />
          </span>
          <div>
            <div className="message-author">{user.name}</div>
            <div className="conversation-preview">
              @{user.handle}
              {showRole ? ` · ${localizeRole(user.role || "", t)}` : ""}
            </div>
          </div>
        </button>
      ))}
    </div>
  );
}
