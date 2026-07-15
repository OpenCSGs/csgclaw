import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import type { AuthNotice } from "@/hooks/workspace/useAuthController";
import { X } from "lucide-react";
import { Toast } from "radix-ui";
import { Tooltip } from "@/components/ui";
import styles from "./AuthLoginNotice.module.css";

type AuthLoginNoticeProps = {
  closeLabel: string;
  notice?: AuthNotice | null;
  onDismiss?: () => void;
};

export function AuthLoginNotice({ closeLabel, notice, onDismiss }: AuthLoginNoticeProps) {
  if (!notice) {
    return null;
  }

  return (
    <Toast.Provider>
      <Toast.Root
        key={notice.id}
        open
        duration={4800}
        onOpenChange={(open) => {
          if (!open) {
            onDismiss?.();
          }
        }}
        className={styles.toastRoot}
      >
        <div className={styles.toastBody}>
          <span className={styles.toastAvatar} aria-hidden="true">
            <AgentAvatarContent avatar={notice.avatar} fallback={notice.avatarFallback} />
          </span>
          <span className={styles.toastText}>
            <Toast.Title className={styles.toastTitle}>{notice.title}</Toast.Title>
            <Toast.Description className={styles.toastMessage}>{notice.message}</Toast.Description>
          </span>
        </div>
        <Tooltip content={closeLabel}>
          <Toast.Close asChild>
            <button type="button" className={styles.toastClose} aria-label={closeLabel}>
              <X size={16} strokeWidth={2.3} aria-hidden="true" />
            </button>
          </Toast.Close>
        </Tooltip>
      </Toast.Root>
      <Toast.Viewport className={styles.toastViewport} />
    </Toast.Provider>
  );
}
