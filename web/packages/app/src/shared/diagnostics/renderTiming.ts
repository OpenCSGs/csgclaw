import { reportDiagnosticTiming } from "@/api/diagnostics";

type Pending = {
  started: number;
  turns: Map<string, { first: number; complete?: number; firstText?: { ms: number; at: string } }>;
};
const pending = new Map<string, Pending>();
const listeners = new Set<() => void>();
const key = (room: string, source: string) => `${room}:${source}`;

// Background-tab throttling is not application latency. Discard those origins.
if (typeof document !== "undefined") {
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState !== "visible") pending.clear();
  });
  window.addEventListener("pagehide", () => pending.clear());
}

// Only the submitting document reports timing. A reload intentionally loses
// these monotonic origins instead of fabricating browser/server clock deltas.
export function trackDiagnosticSubmission(room: string, source: string, started: number) {
  if (document.visibilityState !== "visible") return;
  if (pending.size >= 100) pending.delete(pending.keys().next().value ?? "");
  pending.set(key(room, source), { started, turns: new Map() });
  listeners.forEach((listener) => listener());
}
export function observeDiagnosticRender(
  room: string,
  source: string,
  turn: string,
  final: boolean,
  root?: Element | null,
) {
  let firstFrame = 0;
  let secondFrame = 0;
  const visibleText = new Set<Element>();
  const observe = () => {
    if (document.visibilityState !== "visible" || !pending.has(key(room, source))) return;
    if (firstFrame || secondFrame) return;
    firstFrame = requestAnimationFrame(() => {
      firstFrame = 0;
      secondFrame = requestAnimationFrame(() => {
        secondFrame = 0;
        const item = pending.get(key(room, source));
        if (!item || document.visibilityState !== "visible") return;
        const elapsed = performance.now() - item.started;
        const timing = item.turns.get(turn) ?? { first: elapsed };
        const isNew = !item.turns.has(turn);
        const newText =
          !timing.firstText &&
          [...visibleText].some((el) => el.isConnected && Boolean(el.textContent?.replace(/\u200b/g, "").trim()));
        const newComplete = final && timing.complete === undefined;
        if (!isNew && !newText && !newComplete) return;
        if (newText) timing.firstText = { ms: elapsed, at: new Date().toISOString() };
        if (newComplete) timing.complete = elapsed;
        item.turns.set(turn, timing);
        void reportDiagnosticTiming(room, source, turn, timing.first, timing.complete, timing.firstText).catch(
          () => {},
        );
      });
    });
  };
  const seen = new WeakSet<Element>();
  const intersection =
    root && typeof IntersectionObserver !== "undefined"
      ? new IntersectionObserver((entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting && entry.intersectionRatio > 0) visibleText.add(entry.target);
            else visibleText.delete(entry.target);
          }
          observe();
        })
      : null;
  const scan = () => {
    root?.querySelectorAll("[data-diagnostic-response-text]").forEach((el) => {
      if (!seen.has(el)) {
        seen.add(el);
        intersection?.observe(el);
      }
    });
    observe();
  };
  const mutation = root && intersection ? new MutationObserver(scan) : null;
  mutation?.observe(root!, { subtree: true, childList: true, characterData: true });
  scan();
  listeners.add(observe);
  observe();
  return () => {
    mutation?.disconnect();
    intersection?.disconnect();
    listeners.delete(observe);
    cancelAnimationFrame(firstFrame);
    cancelAnimationFrame(secondFrame);
  };
}
