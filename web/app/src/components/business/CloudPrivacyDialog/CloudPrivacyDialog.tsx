import { useId, useState } from "react";
import {
  Button,
  Checkbox,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./CloudPrivacyDialog.module.css";

export type CloudSendConfirmation = {
  onCancel: () => void;
  onConfirm: (remember: boolean) => void;
};

export function CloudPrivacyDialog({
  agentName,
  confirmation,
  t,
}: {
  agentName: string;
  confirmation: CloudSendConfirmation;
  t: TranslateFn;
}) {
  const [remember, setRemember] = useState(false);
  const checkboxID = useId();
  return (
    <DialogRoot open onOpenChange={(open) => !open && confirmation.onCancel()}>
      <DialogContent className={styles.dialog}>
        <DialogHeader className={styles.header}>
          <DialogTitle>{t("cloudPrivacyTitle")}</DialogTitle>
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
        </DialogHeader>
        <DialogBody className={styles.body}>
          <p className={styles.agent}>{t("cloudPrivacyAgent", { name: agentName })}</p>
          <DialogDescription size="md" className={styles.message}>
            {t("cloudPrivacyMessage")}
          </DialogDescription>
          <label className={styles.remember} htmlFor={checkboxID}>
            <Checkbox id={checkboxID} checked={remember} onCheckedChange={(checked) => setRemember(checked === true)} />
            {t("cloudPrivacyRemember")}
          </label>
        </DialogBody>
        <DialogFooter className={styles.footer}>
          <Button variant="secondaryGray" size="md" onClick={confirmation.onCancel}>
            {t("cancel")}
          </Button>
          <Button variant="primary" size="md" onClick={() => confirmation.onConfirm(remember)}>
            {t("cloudPrivacyContinue")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
