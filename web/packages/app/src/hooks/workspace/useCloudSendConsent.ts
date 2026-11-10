import { useEffect, useState } from "react";
import { CLOUD_SEND_CONSENT_STORAGE_KEY } from "@/shared/storage/keys";

export function useCloudSendConsent(scope: string) {
  const [pending, setPending] = useState<{ scope: string; send: () => void } | null>(null);
  useEffect(() => setPending(null), [scope]);

  function request(send: () => void) {
    try {
      if (localStorage.getItem(CLOUD_SEND_CONSENT_STORAGE_KEY) === "true") return false;
    } catch {
      // Confirm each send when storage is unavailable.
    }
    setPending({ scope, send });
    return true;
  }

  const confirmation = pending?.scope === scope ? pending : null;
  return {
    request,
    dialog: confirmation
      ? {
          onCancel: () => setPending(null),
          onConfirm: (remember: boolean) => {
            setPending(null);
            if (remember) {
              try {
                localStorage.setItem(CLOUD_SEND_CONSENT_STORAGE_KEY, "true");
              } catch {
                // Continue with this send even if the preference cannot be saved.
              }
            }
            confirmation.send();
          },
        }
      : null,
  };
}
