export function resolveHubListSelection<T>(
  items: readonly T[],
  selectedID: string,
  itemID: (item: T) => string,
): T | null {
  if (!selectedID) {
    return null;
  }
  return items.find((item) => itemID(item) === selectedID) ?? null;
}
