import { useState } from "react";

const pageSize = 100;

export function useActivityPage<T>(items: readonly T[], key: string, targetIndex = -1) {
  const [selection, setSelection] = useState<{ key: string; index: number }>();
  const count = Math.ceil(items.length / pageSize);
  const defaultIndex = targetIndex < 0 ? 0 : Math.floor(targetIndex / pageSize);
  const index = Math.min(selection?.key === key ? selection.index : defaultIndex, Math.max(0, count - 1));
  return {
    items: items.slice(index * pageSize, (index + 1) * pageSize),
    count,
    index,
    onChange: (next: number) => setSelection({ key, index: next }),
  };
}
