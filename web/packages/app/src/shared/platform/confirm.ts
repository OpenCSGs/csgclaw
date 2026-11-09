import { getDesktopBridge } from "./desktopBridge";

export function confirmWithFocusRecovery(message: string): boolean {
  const confirmed = window.confirm(message);
  const bridge = getDesktopBridge();
  if (bridge?.restoreFocusAfterDialog) {
    window.setTimeout(() => {
      void bridge.restoreFocusAfterDialog?.().catch(() => undefined);
    }, 0);
  }
  return confirmed;
}
