import { useEffect, useState } from "react";
import { X } from "lucide-react";
import {
  Button,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Tooltip,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { UpgradeChannel } from "@/models/upgradeStatus";
import styles from "./SwitchVersionDialog.module.css";

export type SwitchVersionDialogProps = {
  busy?: boolean;
  currentChannel: UpgradeChannel;
  error?: string;
  open: boolean;
  t: TranslateFn;
  onConfirm: (channel: UpgradeChannel) => boolean | Promise<boolean>;
  onOpenChange: (open: boolean) => void;
};

export function SwitchVersionDialog({
  busy = false,
  currentChannel,
  error = "",
  open,
  t,
  onConfirm,
  onOpenChange,
}: SwitchVersionDialogProps) {
  const [selected, setSelected] = useState<UpgradeChannel>(currentChannel);
  const [confirmAttempted, setConfirmAttempted] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    setSelected(currentChannel);
    setConfirmAttempted(false);
  }, [currentChannel, open]);

  async function handleConfirm() {
    if (selected === currentChannel) {
      onOpenChange(false);
      return;
    }
    setConfirmAttempted(true);
    const ok = await onConfirm(selected);
    if (ok) {
      onOpenChange(false);
    }
  }

  return (
    <DialogRoot open={open} onOpenChange={onOpenChange}>
      <DialogContent className={styles.dialog}>
        <DialogHeader className={styles.header}>
          <div className={styles.copy}>
            <DialogTitle>{t("upgradeChannelSwitchTitle")}</DialogTitle>
            <DialogDescription>{t("upgradeChannelSwitchDescription")}</DialogDescription>
          </div>
          <Tooltip content={t("close")}>
            <DialogClose asChild>
              <button type="button" className={styles.closeButton} aria-label={t("close")} disabled={busy}>
                <X size={18} strokeWidth={1.75} aria-hidden="true" />
              </button>
            </DialogClose>
          </Tooltip>
        </DialogHeader>
        <DialogBody className={styles.body}>
          <fieldset className={styles.options} disabled={busy}>
            <legend className={styles.srOnly}>{t("upgradeChannelSwitchTitle")}</legend>
            {(["release", "beta"] as const).map((value) => (
              <label key={value} className={styles.option}>
                <input
                  checked={selected === value}
                  name="upgrade-channel"
                  type="radio"
                  value={value}
                  onChange={() => setSelected(value)}
                />
                <span>
                  <strong>{t(value === "release" ? "upgradeChannelRelease" : "upgradeChannelBeta")}</strong>
                </span>
              </label>
            ))}
          </fieldset>
          {confirmAttempted && error ? <div className={styles.error}>{error}</div> : null}
        </DialogBody>
        <DialogFooter className={styles.actions}>
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={busy}
            disabled={busy || selected === currentChannel}
            onClick={() => void handleConfirm()}
          >
            {t("confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
