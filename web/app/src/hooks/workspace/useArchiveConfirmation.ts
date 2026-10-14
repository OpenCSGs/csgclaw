import { useSyncExternalStore } from "react";

const storageKey = "csgclaw.task-entry.skip-archive-confirmation";
const changeEvent = "csgclaw:archive-confirmation-changed";
function readPreference() {
  try {
    return window.localStorage.getItem(storageKey) === "true";
  } catch {
    return false;
  }
}
function subscribe(listener: () => void) {
  window.addEventListener(changeEvent, listener);
  window.addEventListener("storage", listener);
  return () => {
    window.removeEventListener(changeEvent, listener);
    window.removeEventListener("storage", listener);
  };
}
export function useArchiveConfirmation() {
  const skip = useSyncExternalStore(subscribe, readPreference, () => false);
  function setSkip(value: boolean) {
    try {
      window.localStorage.setItem(storageKey, String(value));
    } catch {
      return;
    }
    window.dispatchEvent(new Event(changeEvent));
  }
  return [skip, setSkip] as const;
}
