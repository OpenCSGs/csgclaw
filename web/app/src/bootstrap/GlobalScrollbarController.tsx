import { useEffect } from "react";

const SCROLLBAR_ACTIVE_ATTRIBUTE = "data-scrollbar-active";
const SCROLLBAR_HIDE_DELAY_MS = 700;

export function GlobalScrollbarController() {
  useEffect(() => {
    const hideTimers = new Map<Element, number>();

    function handleScroll(event: Event) {
      const element = event.target instanceof Element ? event.target : document.scrollingElement;
      if (!element) {
        return;
      }

      element.setAttribute(SCROLLBAR_ACTIVE_ATTRIBUTE, "true");
      const existingTimer = hideTimers.get(element);
      if (existingTimer !== undefined) {
        window.clearTimeout(existingTimer);
      }
      hideTimers.set(
        element,
        window.setTimeout(() => {
          element.removeAttribute(SCROLLBAR_ACTIVE_ATTRIBUTE);
          hideTimers.delete(element);
        }, SCROLLBAR_HIDE_DELAY_MS),
      );
    }

    document.addEventListener("scroll", handleScroll, true);
    return () => {
      document.removeEventListener("scroll", handleScroll, true);
      hideTimers.forEach((timer, element) => {
        window.clearTimeout(timer);
        element.removeAttribute(SCROLLBAR_ACTIVE_ATTRIBUTE);
      });
      hideTimers.clear();
    };
  }, []);

  return null;
}
