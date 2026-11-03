import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { OPEN_ONBOARDING_GUIDE_EVENT } from "@/shared/lib/onboardingGuide";
import { getDesktopBridge } from "@/shared/platform/desktopBridge";
import styles from "./OnboardingGuide.module.css";
import { GUIDE_ACKNOWLEDGED_KEY, GUIDE_REVISION, hasAcknowledgedGuide } from "./onboardingGuidePersistence";

export function OnboardingGuide() {
  const controller = useWorkspaceControllerContext();
  const locale = controller.sidebarProps?.locale || "zh";
  const [open, setOpen] = useState(false);
  const frameRef = useRef<HTMLIFrameElement>(null);
  const closeRef = useRef<HTMLDivElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const readyFrameRef = useRef<Window | null>(null);
  const bridge = getDesktopBridge();

  useEffect(() => {
    if (!open || !bridge?.setWindowChromeAppearance) return;
    void bridge.setWindowChromeAppearance("onboarding").catch(() => undefined);
    return () => {
      void bridge.setWindowChromeAppearance?.("default").catch(() => undefined);
    };
  }, [open, bridge]);

  const dismissGuide = useCallback(() => {
    if (readyFrameRef.current && readyFrameRef.current === frameRef.current?.contentWindow) {
      try {
        window.localStorage.setItem(GUIDE_ACKNOWLEDGED_KEY, GUIDE_REVISION);
      } catch {
        // Storage failures must not prevent closing the guide.
      }
    }
    setOpen(false);
  }, []);

  useEffect(() => {
    if (!controller.ready) return;
    try {
      if (hasAcknowledgedGuide(window.localStorage)) return;
    } catch {
      // Storage can be unavailable in restricted browser contexts.
    }
    const activeElement = document.activeElement;
    returnFocusRef.current = activeElement instanceof HTMLElement ? activeElement : null;
    setOpen(true);
  }, [controller.ready]);

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
      if (event.origin !== window.location.origin || event.source !== frameRef.current?.contentWindow) return;
      if (event.data?.type === "csgclaw:guide-ready") readyFrameRef.current = frameRef.current?.contentWindow ?? null;
      if (event.data?.type === "csgclaw:guide-exit") dismissGuide();
    };
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [open, dismissGuide]);

  useEffect(() => {
    if (!open) return;
    const frame = frameRef.current;
    const close = closeRef.current;
    if (!frame || !close) return;
    // Iframes do not inherit the shell's CSS variables or the host button size.
    const syncLayout = () => {
      frame.contentWindow?.postMessage(
        {
          type: "csgclaw:guide-layout",
          controlsWidth: parseFloat(getComputedStyle(frame).getPropertyValue("--desktop-window-controls-width")) || 0,
          exitWidth: close.getBoundingClientRect().width,
        },
        window.location.origin,
      );
    };
    const resize = new ResizeObserver(syncLayout);
    resize.observe(close);
    const chrome = frame.closest("[data-desktop-platform]");
    const platform = new MutationObserver(syncLayout);
    if (chrome) platform.observe(chrome, { attributes: true, attributeFilter: ["data-desktop-platform"] });
    frame.addEventListener("load", syncLayout);
    syncLayout();
    return () => {
      resize.disconnect();
      platform.disconnect();
      frame.removeEventListener("load", syncLayout);
    };
  }, [open, locale]);

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
          dismissGuide();
        }
      }}
    >
      <div ref={closeRef} className={styles.close}>
        <Button variant="secondaryGray" size="sm" onClick={dismissGuide}>
          {controller.t("onboardingGuideExit")}
        </Button>
      </div>
      <iframe
        key={locale}
        ref={frameRef}
        className={styles.frame}
        src={`onboarding-guide/guide.html?locale=${locale}&embedded=1`}
        title={title}
        onLoad={() => {
          readyFrameRef.current = null;
          frameRef.current?.focus();
          frameRef.current?.contentWindow?.postMessage({ type: "csgclaw:guide-focus" }, window.location.origin);
        }}
      />
    </div>
  );
}
