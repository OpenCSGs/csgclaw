import { useCallback, useEffect } from "react";
import type { UIEvent } from "react";
import { BookOpen, RefreshCw, Search } from "lucide-react";
import {
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  TextInput,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";
import { classNames } from "@/shared/lib/classNames";
import styles from "./KnowledgeBaseDiscoveryDialog.module.css";

export type KnowledgeBaseDiscoveryDialogProps = {
  copyBusyID: string;
  copyError: string;
  hasMore: boolean;
  items: readonly RemoteKnowledgeBase[];
  loadError: string;
  loading: boolean;
  loadingMore: boolean;
  loginRequired: boolean;
  onAdd: (id: string) => Promise<boolean>;
  onLogin: () => void | Promise<void>;
  onLoadMore: () => void | Promise<unknown>;
  onOpenChange: (open: boolean) => void;
  onRetry: () => void | Promise<unknown>;
  open: boolean;
  onSearchChange: (value: string) => void;
  search: string;
  t: TranslateFn;
};

export function KnowledgeBaseDiscoveryDialog({
  copyBusyID,
  copyError,
  hasMore,
  items,
  loadError,
  loading,
  loadingMore,
  loginRequired,
  onAdd,
  onLogin,
  onLoadMore,
  onOpenChange,
  onRetry,
  open,
  onSearchChange,
  search,
  t,
}: KnowledgeBaseDiscoveryDialogProps) {
  useEffect(() => {
    if (!open) {
      onSearchChange("");
    }
  }, [onSearchChange, open]);

  const handleScroll = useCallback(
    (event: UIEvent<HTMLDivElement>) => {
      if (!hasMore || loadingMore) {
        return;
      }
      const target = event.currentTarget;
      if (target.scrollHeight - target.scrollTop - target.clientHeight <= 80) {
        void onLoadMore();
      }
    },
    [hasMore, loadingMore, onLoadMore],
  );

  async function addKnowledgeBase(id: string) {
    if (await onAdd(id)) {
      onOpenChange(false);
    }
  }

  return (
    <DialogRoot open={open} onOpenChange={onOpenChange}>
      <DialogContent className={styles.dialog}>
        <DialogHeader>
          <div>
            <DialogTitle>{t("resourcesKnowledgeBaseDiscoverTitle")}</DialogTitle>
            <DialogDescription>{t("resourcesKnowledgeBaseDiscoverDescription")}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} variant="tertiaryGray" />
        </DialogHeader>
        <DialogBody className={styles.body}>
          <label className={styles.search}>
            <Search size={18} strokeWidth={1.8} aria-hidden="true" />
            <TextInput
              className={styles.searchInput}
              type="search"
              value={search}
              placeholder={t("resourcesKnowledgeBaseSearchPlaceholder")}
              aria-label={t("resourcesKnowledgeBaseSearchPlaceholder")}
              onChange={(event) => onSearchChange(event.currentTarget.value)}
            />
          </label>
          {copyError ? <div className="form-error">{copyError}</div> : null}
          {loginRequired ? (
            <div className={styles.state}>
              <span>{t("resourcesKnowledgeBasesLoginRequired")}</span>
              <Button size="sm" variant="primary" onClick={() => void onLogin()}>
                {t("resourcesKnowledgeBasesLogin")}
              </Button>
            </div>
          ) : loadError ? (
            <div className={styles.state}>
              <span>{loadError}</span>
              <Button size="sm" variant="secondaryGray" onClick={() => void onRetry()}>
                <RefreshCw size={14} strokeWidth={2} aria-hidden="true" />
                {t("retry")}
              </Button>
            </div>
          ) : loading && !items.length ? (
            <div className={styles.state}>{t("resourcesKnowledgeBasesLoading")}</div>
          ) : items.length ? (
            <div className={styles.list} onScroll={handleScroll}>
              {items.map((item) => {
                const added = Boolean(item.configuredMCPName);
                const available = item.availability === "available";
                return (
                  <div key={item.id} className={styles.row}>
                    <span className={styles.icon} aria-hidden="true">
                      <BookOpen size={18} strokeWidth={1.8} />
                    </span>
                    <span className={styles.main}>
                      <span className={styles.titleRow}>
                        <strong className="truncate">{item.name}</strong>
                        <small className={classNames(styles.status, !available && styles.unavailable)}>
                          {added
                            ? t("resourcesKnowledgeBaseAdded")
                            : available
                              ? t("resourcesKnowledgeBaseAvailable")
                              : t("resourcesKnowledgeBaseUnavailable")}
                        </small>
                      </span>
                      <span className={classNames(styles.description, "truncate")}>
                        {item.description || t("resourcesKnowledgeBaseAgenticHub")}
                      </span>
                    </span>
                    <Button
                      size="sm"
                      variant="primary"
                      loading={copyBusyID === item.id}
                      disabled={added || !available}
                      onClick={() => void addKnowledgeBase(item.id)}
                    >
                      {added ? t("resourcesKnowledgeBaseAdded") : t("resourcesKnowledgeBaseAddMCP")}
                    </Button>
                  </div>
                );
              })}
              {loadingMore ? <div className={styles.listState}>{t("resourcesKnowledgeBasesLoading")}</div> : null}
            </div>
          ) : (
            <div className={styles.state}>
              {search.trim() ? t("workspaceSearchNoResults") : t("resourcesKnowledgeBasesEmpty")}
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}
