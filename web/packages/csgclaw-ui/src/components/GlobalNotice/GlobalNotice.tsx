/*! Derived from CSGClaw. Modified for standalone csgclaw-ui distribution. */
import { classNames } from "../../lib/classNames.js";
import styles from "./GlobalNotice.module.css";
import { Toast as RadixToast } from "radix-ui";
import { AlertTriangle, Info, X } from "lucide-react";
import { useCallback, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { GlobalNoticeContext } from "./GlobalNoticeContext.js";
import type { GlobalNoticeInput } from "./GlobalNoticeContext.js";

type GlobalNoticeState = GlobalNoticeInput & { id: number };

export function GlobalNoticeProvider({
  children,
  noticeClassName,
  viewportClassName,
  closeIcon,
}: {
  children: ReactNode;
  noticeClassName?: string;
  viewportClassName?: string;
  closeIcon?: ReactNode;
}) {
  const nextID = useRef(0);
  const [notice, setNotice] = useState<GlobalNoticeState | null>(null);
  const showNotice = useCallback((input: GlobalNoticeInput) => {
    nextID.current += 1;
    setNotice({ ...input, id: nextID.current });
  }, []);
  const value = useMemo(() => ({ showNotice }), [showNotice]);
  const Icon = notice?.tone === "warning" ? AlertTriangle : Info;

  return (
    <GlobalNoticeContext.Provider value={value}>
      <RadixToast.Provider swipeDirection="up" duration={10_000}>
        {children}
        {notice ? (
          <RadixToast.Root
            key={notice.id}
            className={classNames(
              styles["global-notice"],
              styles[notice.tone ?? "info"],
              noticeClassName,
            )}
            open
            onOpenChange={(open) => {
              if (!open) setNotice(null);
            }}
          >
            {notice.icon ?? (
              <Icon
                className={styles["global-notice-icon"]}
                size={20}
                aria-hidden="true"
              />
            )}
            <div
              className={styles["global-notice-content"]}
              data-notice-part="content"
            >
              {notice.title ? (
                <RadixToast.Title
                  className={styles["global-notice-title"]}
                  data-notice-part="title"
                >
                  {notice.title}
                </RadixToast.Title>
              ) : null}
              <RadixToast.Description
                className={styles["global-notice-description"]}
                data-notice-part="description"
              >
                {notice.message}
              </RadixToast.Description>
              {notice.dismissLabel || notice.action ? (
                <div
                  data-notice-part="actions"
                  className={styles["global-notice-actions"]}
                >
                  {notice.dismissLabel && (
                    <RadixToast.Close type="button">
                      {notice.dismissLabel}
                    </RadixToast.Close>
                  )}
                  {notice.action && (
                    <RadixToast.Action
                      type="button"
                      altText={notice.action.label}
                      onClick={notice.action.onSelect}
                    >
                      {notice.action.label}
                    </RadixToast.Action>
                  )}
                </div>
              ) : null}
            </div>
            <RadixToast.Close
              className={styles["global-notice-close"]}
              aria-label={notice.closeLabel}
              data-notice-part="close"
            >
              {closeIcon ?? <X size={16} aria-hidden="true" />}
            </RadixToast.Close>
          </RadixToast.Root>
        ) : null}
        <RadixToast.Viewport
          className={classNames(
            styles["global-notice-viewport"],
            viewportClassName,
          )}
        />
      </RadixToast.Provider>
    </GlobalNoticeContext.Provider>
  );
}
