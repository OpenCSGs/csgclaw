import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import type { AuthNotice } from "@/hooks/workspace/useAuthController";
import { X } from "lucide-react";
import { useEffect } from "react";
import styles from "./AuthLoginNotice.module.css";

type AuthLoginNoticeProps = {
  closeLabel: string;
  notice?: AuthNotice | null;
  onDismiss?: () => void;
};

export function AuthLoginNotice({ closeLabel, notice, onDismiss }: AuthLoginNoticeProps) {
  useEffect(() => {
    if (!notice) {
      return undefined;
    }
    const timeout = window.setTimeout(() => onDismiss?.(), 4800);
    return () => window.clearTimeout(timeout);
  }, [notice, onDismiss]);

  if (!notice) {
    return null;
  }

  return (
    <div className={styles.toastViewport} aria-live="polite" aria-atomic="true">
      <div key={notice.id} className={styles.toastRoot} role="status">
        <div className={styles.toastBody}>
          <span className={styles.toastAvatar} aria-hidden="true">
            <AgentAvatarContent avatar={notice.avatar} fallback={notice.avatarFallback} />
          </span>
          <span className={styles.toastText}>
            <strong className={styles.toastTitle}>{notice.title}</strong>
            <span className={styles.toastMessage}>{notice.message}</span>
          </span>
        </div>
        <button
          type="button"
          className={styles.toastClose}
          aria-label={closeLabel}
          title={closeLabel}
          onClick={onDismiss}
        >
          <X size={16} strokeWidth={2.3} aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}
