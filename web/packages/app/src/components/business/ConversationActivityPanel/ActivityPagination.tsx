import { Button } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ConversationActivityPanel.module.css";

export function ActivityPagination({
  count,
  index,
  onChange,
  t,
}: {
  count: number;
  index: number;
  onChange: (next: number) => void;
  t: TranslateFn;
}) {
  if (count < 2) return null;
  return (
    <div className={styles.pageActions}>
      <Button size="sm" variant="tertiaryGray" disabled={index === 0} onClick={() => onChange(index - 1)}>
        {t("activityPreviousPage")}
      </Button>
      <span aria-live="polite">{t("activityPageCount", { page: index + 1, count })}</span>
      <Button size="sm" variant="tertiaryGray" disabled={index + 1 === count} onClick={() => onChange(index + 1)}>
        {t("activityNextPage")}
      </Button>
    </div>
  );
}

export function LoadEarlierActivity({ loading, onLoad, t }: { loading: boolean; onLoad: () => void; t: TranslateFn }) {
  return (
    <div className={styles.pageActions}>
      <Button size="sm" variant="secondaryGray" disabled={loading} onClick={onLoad}>
        {t(loading ? "agentActivityLoading" : "activityLoadEarlier")}
      </Button>
    </div>
  );
}
