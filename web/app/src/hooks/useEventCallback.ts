import { useCallback, useLayoutEffect, useRef } from "react";

// Keep event props stable across renders while invoking the latest committed handler.
// Rendering functions must use useCallback with their actual data dependencies instead.
export function useEventCallback<Args extends unknown[], Result>(
  callback: ((...args: Args) => Result) | undefined,
): (...args: Args) => Result | undefined {
  const callbackRef = useRef(callback);
  useLayoutEffect(() => {
    callbackRef.current = callback;
  }, [callback]);
  return useCallback((...args: Args) => callbackRef.current?.(...args), []);
}
