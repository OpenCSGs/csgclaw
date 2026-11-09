import { createUUID } from "@/shared/lib/uuid";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { getDesktopBridge, type DesktopBrowserState } from "@/shared/platform/desktopBridge";

export function useNativeBrowser({
  url,
  active,
  onNavigate,
  onPopup,
}: {
  url: string;
  active: boolean;
  onNavigate: (url: string) => void;
  onPopup: (url: string) => void;
}) {
  const bridge = getDesktopBridge();
  const [id] = useState(() => createUUID());
  const hostRef = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<DesktopBrowserState | null>(null);
  const currentPageURL = useRef("");
  const [failure, setFailure] = useState(false);
  const callbacks = useRef({ url, onNavigate, onPopup });
  useLayoutEffect(() => {
    callbacks.current = { url, onNavigate, onPopup };
  }, [url, onNavigate, onPopup]);
  useEffect(() => {
    if (!bridge) return;
    const unsubscribe = bridge.onBrowserState((next) => {
      if (next.id !== id) return;
      if (next.shortcut === "escape") {
        hostRef.current
          ?.closest(".chat-browser-tab")
          ?.querySelector<HTMLInputElement>("input")
          ?.focus({ preventScroll: true });
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", cancelable: true }));
        return;
      }
      if (next.popupURL) {
        callbacks.current.onPopup(next.popupURL);
        return;
      }
      currentPageURL.current = next.url;
      setState(next);
      if (next.url && next.url !== callbacks.current.url && !next.error) callbacks.current.onNavigate(next.url);
    });
    return () => {
      unsubscribe();
      void bridge.browserCommand({ id, action: "close" }).catch(() => {});
    };
  }, [bridge, id]);
  useEffect(() => {
    if (!bridge || !url || url === currentPageURL.current) return;
    currentPageURL.current = url;
    setFailure(false);
    void bridge.browserCommand({ id, action: "navigate", url }).catch(() => setFailure(true));
  }, [bridge, id, url]);
  const failed = failure || Boolean(state?.error);
  useEffect(() => {
    if (!bridge || !url) return;
    let alive = true;
    let frame = 0;
    let lastLayout = "";
    const sync = () => {
      frame = 0;
      const host = hostRef.current;
      if (!host) return;
      const rect = host.getBoundingClientRect();
      const covered = Array.from(
        document.querySelectorAll<HTMLElement>(
          '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"], .header-popover, [data-radix-popper-content-wrapper]',
        ),
      ).some((node) => {
        if (
          node.hasAttribute("data-radix-popper-content-wrapper") &&
          !node.querySelector('[data-state="open"]:not([role="tooltip"])')
        )
          return false;
        const style = getComputedStyle(node),
          box = node.getBoundingClientRect();
        return (
          style.display !== "none" &&
          style.visibility !== "hidden" &&
          style.opacity !== "0" &&
          box.width > 0 &&
          box.height > 0 &&
          box.left < rect.right &&
          box.right > rect.left &&
          box.top < rect.bottom &&
          box.bottom > rect.top
        );
      });
      const request = {
        id,
        action: "sync" as const,
        url,
        visible: active && !failed && !covered && document.visibilityState !== "hidden",
        bounds: { x: rect.x, y: rect.y, width: rect.width, height: rect.height },
      };
      const signature = JSON.stringify(request);
      if (signature === lastLayout) return;
      lastLayout = signature;
      void bridge
        .browserCommand(request)
        .then((next) => {
          if (alive && next?.error) setState(next);
        })
        .catch(() => {
          if (alive) setFailure(true);
        });
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(sync);
    };
    const resize = new ResizeObserver(schedule);
    if (hostRef.current) resize.observe(hostRef.current);
    const overlays = new MutationObserver(schedule);
    overlays.observe(document.body, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["data-state", "hidden", "open", "style", "class"],
    });
    window.addEventListener("resize", schedule);
    document.addEventListener("visibilitychange", schedule);
    schedule();
    return () => {
      alive = false;
      cancelAnimationFrame(frame);
      resize.disconnect();
      overlays.disconnect();
      window.removeEventListener("resize", schedule);
      document.removeEventListener("visibilitychange", schedule);
    };
  }, [bridge, id, url, active, failed]);
  async function command(action: "reload" | "back" | "forward") {
    if (!bridge) return;
    setFailure(false);
    try {
      const next = await bridge.browserCommand({ id, action });
      if (next) setState(next);
    } catch {
      setFailure(true);
    }
  }
  return { available: Boolean(bridge), hostRef, state, failed, command };
}
