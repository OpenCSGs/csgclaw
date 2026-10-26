import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { OPEN_ONBOARDING_GUIDE_EVENT } from "@/shared/lib/onboardingGuide";
import styles from "./OnboardingGuide.module.css";

const SEEN_GUIDE_KEY = "csgclaw:onboarding-guide:seen";

export function OnboardingGuide() {
  const controller = useWorkspaceControllerContext();
  const version = controller.sidebarProps?.appVersion;
  const locale = controller.sidebarProps?.locale || "zh";
  const [open, setOpen] = useState(false);
  const frameRef = useRef<HTMLIFrameElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!controller.ready || !version) return;
    const key = `${SEEN_GUIDE_KEY}:${version}`;
    try {
      if (window.localStorage.getItem(key) === "seen") return;
      window.localStorage.setItem(key, "seen");
    } catch {
      // Storage can be unavailable in restricted browser contexts.
    }
    const activeElement = document.activeElement;
    returnFocusRef.current = activeElement instanceof HTMLElement ? activeElement : null;
    setOpen(true);
  }, [controller.ready, version]);

  useEffect(() => {
    const handleOpen = () => {
      const activeElement = document.activeElement;
      returnFocusRef.current = activeElement instanceof HTMLElement ? activeElement : null;
      setOpen(true);
    };
    window.addEventListener(OPEN_ONBOARDING_GUIDE_EVENT, handleOpen);
    return () => window.removeEventListener(OPEN_ONBOARDING_GUIDE_EVENT, handleOpen);
  }, []);

  useEffect(() => {
    if (open) return;
    const returnFocus = returnFocusRef.current;
    if (!returnFocus?.isConnected) return;
    requestAnimationFrame(() => returnFocus.focus());
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handleMessage = (event: MessageEvent) => {
      if (event.source !== frameRef.current?.contentWindow) return;
      if (event.data?.type === "csgclaw:guide-exit") setOpen(false);
    };
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [open]);

  if (!open) return null;
  const title = locale === "en" ? "CSGClaw onboarding guide" : "CSGClaw 新手引导";

  return (
    <div
      className={styles.overlay}
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.stopPropagation();
          setOpen(false);
        }
      }}
    >
      <div className={styles.close}>
        <Button variant="secondaryGray" size="sm" onClick={() => setOpen(false)}>
          {controller.t("onboardingGuideExit")}
        </Button>
      </div>
      <iframe
        ref={frameRef}
        className={styles.frame}
        src={`onboarding-guide/guide.html?locale=${locale}&embedded=1`}
        title={title}
        onLoad={() => {
          frameRef.current?.focus();
          frameRef.current?.contentWindow?.postMessage({ type: "csgclaw:guide-focus" }, window.location.origin);
        }}
      />
    </div>
  );
}
