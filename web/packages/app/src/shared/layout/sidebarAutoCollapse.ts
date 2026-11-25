const listeners = new Set<() => void>();

/** A panel may make room by collapsing navigation; expanding remains a user action. */
export function requestSidebarAutoCollapse(): void {
  listeners.forEach((listener) => listener());
}

export function subscribeSidebarAutoCollapse(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
