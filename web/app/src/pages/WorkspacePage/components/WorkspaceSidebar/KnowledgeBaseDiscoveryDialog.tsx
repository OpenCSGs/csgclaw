import { useEffect, useMemo, useState } from "react";
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
  items: readonly RemoteKnowledgeBase[];
  loadError: string;
  loading: boolean;
  loginRequired: boolean;
  onAdd: (id: string) => Promise<boolean>;
  onLogin: () => void | Promise<void>;
  onOpenChange: (open: boolean) => void;
  onRetry: () => void | Promise<unknown>;
  open: boolean;
  t: TranslateFn;
};

export function KnowledgeBaseDiscoveryDialog({
  copyBusyID,
  copyError,
  items,
  loadError,
  loading,
  loginRequired,
  onAdd,
  onLogin,
  onOpenChange,
  onRetry,
  open,
  t,
}: KnowledgeBaseDiscoveryDialogProps) {
  const [search, setSearch] = useState("");
  const visibleItems = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    if (!query) {
      return items;
    }
    return items.filter((item) => `${item.name} ${item.description || ""}`.toLocaleLowerCase().includes(query));
  }, [items, search]);

  useEffect(() => {
    if (!open) {
      setSearch("");
    }
  }, [open]);

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
              onChange={(event) => setSearch(event.currentTarget.value)}
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
          ) : visibleItems.length ? (
            <div className={styles.list}>
              {visibleItems.map((item) => {
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
            </div>
          ) : (
            <div className={styles.state}>
              {items.length ? t("workspaceSearchNoResults") : t("resourcesKnowledgeBasesEmpty")}
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}
