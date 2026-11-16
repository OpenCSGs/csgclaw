import { useRef, useState } from "react";
import {
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Field,
  InlineError,
  TextInput,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";

const deletionImpact = [
  "archivedAgentDeleteData",
  "archivedAgentDeleteBindings",
  "archivedAgentDeleteMemberships",
  "archivedAgentKeepSchedules",
  "archivedAgentDeleteChats",
  "archivedAgentKeepHistory",
  "archivedAgentKeepFiles",
  "archivedAgentKeepSnapshot",
];

export function AgentDeleteDialog({
  name,
  t,
  onClose,
  onConfirm,
  returnFocus,
  portalContainer,
}: {
  name: string;
  t: TranslateFn;
  onClose: () => void;
  onConfirm: (name: string) => Promise<void>;
  returnFocus?: HTMLElement | null;
  portalContainer?: HTMLElement | null;
}) {
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);

  async function confirm() {
    if (pending.current || confirmation !== name) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      await onConfirm(confirmation);
      onClose();
    } catch (failure) {
      setError(localizeAPIError(failure, t, t("agentActionFailed")));
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <DialogRoot open onOpenChange={(open) => !open && !pending.current && onClose()}>
      <DialogContent
        className="break-words"
        scrollBody
        portalContainer={portalContainer}
        onCloseAutoFocus={(event) => {
          if (returnFocus?.isConnected) {
            event.preventDefault();
            returnFocus.focus();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>{t("archivedAgentDeleteTitle", { name })}</DialogTitle>
          <DialogCloseButton label={t("close")} disabled={busy} />
        </DialogHeader>
        <DialogBody className="flex flex-col gap-3">
          <DialogDescription size="md">{t("archivedAgentDeleteHint")}</DialogDescription>
          <ul className="m-0 list-disc space-y-2 pl-5 text-sm">
            {deletionImpact.map((key) => (
              <li key={key}>{t(key)}</li>
            ))}
          </ul>
          <Field label={t("archivedAgentConfirmName", { name })}>
            <TextInput
              aria-label={t("archivedAgentName")}
              value={confirmation}
              disabled={busy}
              onChange={(event) => setConfirmation(event.target.value)}
              autoComplete="off"
              placeholder={name}
            />
          </Field>
          {error && <InlineError role="alert">{error}</InlineError>}
        </DialogBody>
        <DialogFooter>
          <Button variant="secondaryGray" disabled={busy} onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="danger"
            loading={busy}
            disabled={busy || confirmation !== name}
            onClick={() => void confirm()}
          >
            {t("archivedAgentDelete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
