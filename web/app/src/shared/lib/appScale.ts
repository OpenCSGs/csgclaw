export function rootZoomScale(): number {
  const zoom = Number.parseFloat(window.getComputedStyle(document.documentElement).zoom);
  return Number.isFinite(zoom) && zoom > 0 ? zoom : 1;
}
