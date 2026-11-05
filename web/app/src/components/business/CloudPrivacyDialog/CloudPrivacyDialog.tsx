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
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("cloudPrivacyTitle")}</DialogTitle>
          <DialogCloseButton label={t("close")} />
        </DialogHeader>
        <DialogBody>
          <DialogDescription>{t("cloudPrivacyAgent", { name: agentName })}</DialogDescription>
          <p>{t("cloudPrivacyMessage")}</p>
          <label className="flex items-center gap-2" htmlFor={checkboxID}>
            <Checkbox id={checkboxID} checked={remember} onCheckedChange={(checked) => setRemember(checked === true)} />
            {t("cloudPrivacyRemember")}
          </label>
        </DialogBody>
        <DialogFooter>
          <Button variant="secondaryGray" onClick={confirmation.onCancel}>
            {t("cancel")}
          </Button>
          <Button variant="primary" onClick={() => confirmation.onConfirm(remember)}>
            {t("cloudPrivacyContinue")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
