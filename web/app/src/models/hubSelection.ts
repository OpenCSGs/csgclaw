export function resolveHubListSelection<T>(
  items: readonly T[],
  selectedID: string,
  itemID: (item: T) => string,
): T | null {
  const selected = items.find((item) => itemID(item) === selectedID);
  if (selected) {
    return selected;
  }
  return selectedID ? null : (items[0] ?? null);
}
