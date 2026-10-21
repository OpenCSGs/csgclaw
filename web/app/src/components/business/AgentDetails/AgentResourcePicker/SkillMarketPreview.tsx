import { Plus } from "lucide-react";
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
} from "@/components/ui";
import { SkillDetailFiles } from "@/components/business/SkillDetailFiles";
import type { SkillSummary } from "@/models/skillhub";
import type { TranslateFn } from "@/models/conversations";
import styles from "./SkillMarketPreview.module.css";

export function SkillMarketPreview({
  skill,
  t,
  busy,
  installed,
  installDisabled,
  error,
  onClose,
  onAfterClose,
  onInstall,
  portalContainer,
}: {
  skill: SkillSummary;
  t: TranslateFn;
  busy: boolean;
  installed: boolean;
  installDisabled: boolean;
  error?: string;
  onClose: () => void;
  onAfterClose: () => void;
  onInstall: () => void;
  portalContainer?: HTMLElement | null;
}) {
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className={styles.dialog}
        portalContainer={portalContainer}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          onAfterClose();
        }}
      >
        <DialogHeader>
          <div>
            <div className={styles.titleRow}>
              <DialogTitle>{skill.name}</DialogTitle>
              <span className={styles.origin}>{t("profileSkillMarket")}</span>
            </div>
            <DialogDescription>{skill.description || skill.name}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} disabled={busy} size="sm" variant="tertiaryGray" />
        </DialogHeader>
        <DialogBody className={styles.body}>
          {error ? (
            <p className="form-error" role="alert">
              {error}
            </p>
          ) : null}
          <SkillDetailFiles
            key={JSON.stringify([skill.remotePath, skill.remoteRef, skill.name])}
            skill={skill}
            market
            t={t}
          />
        </DialogBody>
        <DialogFooter className={styles.footer}>
          <Button
            size="sm"
            variant="primary"
            loading={busy}
            disabled={busy || installed || installDisabled}
            onClick={onInstall}
          >
            <Plus size={15} aria-hidden="true" />
            {t(installed ? "profileResourceEnabled" : "resourcesSkillRemoteInstallAction")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
