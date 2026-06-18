import { CircleHelp, UserPlus, Users } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { TranslateFn } from "@/models/conversations";
import styles from "./FloatingChat.module.css";

const FLOATING_CHAT_SUGGESTIONS = [
  {
    icon: UserPlus,
    key: "floatingChatSuggestionCreateWorker",
  },
  {
    icon: Users,
    key: "floatingChatSuggestionManageWorkspace",
  },
  {
    icon: CircleHelp,
    key: "floatingChatSuggestionAskQuestion",
  },
] satisfies Array<{
  icon: LucideIcon;
  key: string;
}>;

export type FloatingChatPromptSuggestionsProps = {
  agentName: string;
  t: TranslateFn;
  onPickPrompt: (text: string) => void;
};

export function FloatingChatPromptSuggestions({ agentName, t, onPickPrompt }: FloatingChatPromptSuggestionsProps) {
  return (
    <div className={styles.prompts}>
      <div className={styles.promptsCopy}>
        <strong>{t("floatingChatGreeting", { name: agentName })}</strong>
        <span>{t("floatingChatTryAsking")}</span>
      </div>
      <div className={styles.promptList}>
        {FLOATING_CHAT_SUGGESTIONS.map(({ icon: Icon, key }) => {
          const text = t(key);
          return (
            <button key={key} type="button" className={styles.prompt} onClick={() => onPickPrompt(text)}>
              <span className={styles.promptIcon} aria-hidden="true">
                <Icon size={16} strokeWidth={1.9} />
              </span>
              <span>{text}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
