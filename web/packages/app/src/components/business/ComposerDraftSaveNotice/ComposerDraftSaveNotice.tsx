import { Button } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ComposerDraftSaveNotice.module.css";

export function ComposerDraftSaveNotice({ onRetry, t }: { onRetry?: () => void; t: TranslateFn }) {
  return (
    <div className={onRetry ? styles.notice : undefined}>
      <span aria-live="polite">{onRetry ? t("composerDraftSaveFailed") : ""}</span>
      {onRetry ? (
        <Button size="sm" variant="secondaryGray" onClick={onRetry}>
          {t("composerDraftRetrySave")}
        </Button>
      ) : null}
    </div>
  );
}
