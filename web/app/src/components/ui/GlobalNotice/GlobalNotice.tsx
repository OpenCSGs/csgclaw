import { Toast as RadixToast } from "radix-ui";
import { AlertTriangle, Info, X } from "lucide-react";
import { useCallback, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { GlobalNoticeContext } from "./GlobalNoticeContext";
import type { GlobalNoticeInput } from "./GlobalNoticeContext";

type GlobalNoticeState = GlobalNoticeInput & { id: number };

export function GlobalNoticeProvider({ children }: { children: ReactNode }) {
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
            className={`global-notice ${notice.tone ?? "info"}`}
            open
            onOpenChange={(open) => {
              if (!open) setNotice(null);
            }}
          >
            <Icon className="global-notice-icon" size={20} aria-hidden="true" />
            <div className="global-notice-content">
              {notice.title ? (
                <RadixToast.Title className="global-notice-title">{notice.title}</RadixToast.Title>
              ) : null}
              <RadixToast.Description className="global-notice-description">{notice.message}</RadixToast.Description>
            </div>
            <RadixToast.Close className="global-notice-close" aria-label={notice.closeLabel}>
              <X size={16} aria-hidden="true" />
            </RadixToast.Close>
          </RadixToast.Root>
        ) : null}
        <RadixToast.Viewport className="global-notice-viewport" />
      </RadixToast.Provider>
    </GlobalNoticeContext.Provider>
  );
}
