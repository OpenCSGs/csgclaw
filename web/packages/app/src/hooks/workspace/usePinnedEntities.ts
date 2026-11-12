import { useCallback, useMemo, useSyncExternalStore } from "react";
import { PINNED_ENTITIES_STORAGE_KEY } from "@/shared/storage/keys";

export type PinKind = "agent" | "cloud" | "team" | "template";
export type Pinnable = { kind: PinKind; id: string };

export const entityPinKey = ({ kind, id }: Pinnable) => `${kind}:${id}`;

const memory = new Map<string, string>();

function read(key: string): string {
  try {
    return memory.get(key) ?? localStorage.getItem(key) ?? "[]";
  } catch {
    return memory.get(key) ?? "[]";
  }
}

function parse(raw: string): string[] {
  try {
    const values: unknown = JSON.parse(raw);
    return Array.isArray(values) ? values.filter((value): value is string => typeof value === "string") : [];
  } catch {
    return [];
  }
}

function subscribe(listener: () => void) {
  window.addEventListener("storage", listener);
  window.addEventListener(PINNED_ENTITIES_STORAGE_KEY, listener);
  return () => {
    window.removeEventListener("storage", listener);
    window.removeEventListener(PINNED_ENTITIES_STORAGE_KEY, listener);
  };
}

export function usePinnedEntities(userID: string) {
  const key = `${PINNED_ENTITIES_STORAGE_KEY}:${location.pathname}:${encodeURIComponent(userID)}`;
  const snapshot = useSyncExternalStore(
    subscribe,
    useCallback(() => read(key), [key]),
  );
  const order = useMemo(() => parse(snapshot), [snapshot]);

  function toggle(entity: Pinnable) {
    if (!entity.id) return;
    const id = entityPinKey(entity);
    const current = parse(read(key));
    const next = JSON.stringify(current.includes(id) ? current.filter((item) => item !== id) : [id, ...current]);
    try {
      localStorage.setItem(key, next);
      memory.delete(key);
    } catch {
      memory.set(key, next);
    }
    window.dispatchEvent(new Event(PINNED_ENTITIES_STORAGE_KEY));
  }

  return { order, toggle };
}

export function partitionPinned<T extends Pinnable>(items: readonly T[], order: readonly string[]) {
  const byKey = new Map(items.map((item) => [entityPinKey(item), item]));
  const pinned: T[] = [];
  for (const key of order) {
    const item = byKey.get(key);
    if (!item) continue;
    pinned.push(item);
    byKey.delete(key);
  }
  return { pinned, rest: [...byKey.values()] };
}
