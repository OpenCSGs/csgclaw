import { useLayoutEffect, type RefObject } from "react";

/** Fit the actual toolbar contents, including changes while the page is hidden. */
export function useComposerCompact(
  boxRef: RefObject<HTMLElement | null>,
  rowRef: RefObject<HTMLElement | null>,
  enabled = true,
): void {
  useLayoutEffect(() => {
    const box = boxRef.current;
    const row = rowRef.current;
    if (!enabled || !box || !row) return;
    let active = true;
    const apply = () => {
      // Hidden routes and intermediate layout states have no usable width.
      if (!active || box.clientWidth <= 0 || row.clientWidth <= 0) return;
      box.removeAttribute("data-compact");
      if (row.scrollWidth <= row.clientWidth + 1) return;
      box.setAttribute("data-compact", "config");
      if (row.scrollWidth <= row.clientWidth + 1) return;
      box.setAttribute("data-compact", "all");
    };
    // Do not defer to rAF: background tabs may never run that callback.
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(apply);
    resize?.observe(box);
    resize?.observe(row);
    const mutation = typeof MutationObserver === "undefined" ? null : new MutationObserver(apply);
    mutation?.observe(row, { childList: true, characterData: true, subtree: true });
    window.addEventListener("resize", apply);
    document.addEventListener("visibilitychange", apply);
    document.fonts?.addEventListener("loadingdone", apply);
    void document.fonts?.ready.then(apply);
    apply();
    return () => {
      active = false;
      resize?.disconnect();
      mutation?.disconnect();
      window.removeEventListener("resize", apply);
      document.removeEventListener("visibilitychange", apply);
      document.fonts?.removeEventListener("loadingdone", apply);
    };
  }, [boxRef, rowRef, enabled]);
}
