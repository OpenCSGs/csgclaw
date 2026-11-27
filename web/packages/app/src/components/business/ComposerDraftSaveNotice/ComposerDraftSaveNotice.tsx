import { Button } from "@/components/ui";
import { ErrorNotice } from "@/components/business/ErrorNotice";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ComposerDraftSaveNotice.module.css";

export function ComposerDraftSaveNotice({
  onDismiss,
  onRetry,
  t,
}: {
  onDismiss?: () => void;
  onRetry?: () => void;
  t: TranslateFn;
}) {
  if (!onRetry || !onDismiss) return null;
  return (
    <ErrorNotice
      className={styles.notice}
      dismissLabel={t("close")}
      onDismiss={onDismiss}
      action={
        <Button size="sm" variant="secondaryGray" onClick={onRetry}>
          {t("composerDraftRetrySave")}
        </Button>
      }
    >
      <span>{t("composerDraftSaveFailed")}</span>
    </ErrorNotice>
  );
}
