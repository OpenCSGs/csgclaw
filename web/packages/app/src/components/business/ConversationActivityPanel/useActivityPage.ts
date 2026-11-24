import { useState } from "react";

const pageSize = 100;

export function useActivityPage<T>(items: readonly T[], key: string, targetIndex = -1, itemKey?: (item: T) => string) {
  const [selection, setSelection] = useState<{ key: string; index?: number; firstItem?: string }>();
  const current = selection?.key === key ? selection : undefined;
  const anchor = current?.firstItem && itemKey ? items.findIndex((item) => itemKey(item) === current.firstItem) : -1;
  // Start a page at the previously visible row when prepending older history.
  const inset = anchor > 0 ? (pageSize - (anchor % pageSize)) % pageSize : 0;
  const count = Math.ceil((items.length + inset) / pageSize);
  const defaultIndex = Math.floor(((anchor >= 0 ? anchor : Math.max(targetIndex, 0)) + inset) / pageSize);
  const index = Math.min(current?.index ?? defaultIndex, Math.max(0, count - 1));
  const start = Math.max(0, index * pageSize - inset);
  return {
    items: items.slice(start, (index + 1) * pageSize - inset),
    count,
    index,
    onChange: (next: number) => setSelection({ key, index: next, firstItem: current?.firstItem }),
    keepFirst: () => {
      if (itemKey && start < items.length) setSelection({ key, firstItem: itemKey(items[start]) });
    },
  };
}
