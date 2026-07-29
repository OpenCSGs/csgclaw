import { useCallback, useEffect } from "react";
import type { UIEvent } from "react";
import { CloudDownload, RefreshCw, Server } from "lucide-react";
import { Button, TextInput } from "@/components/ui";
import { hasMCPServerName } from "@/models/mcp";
import type { MCPServer, RemoteMCPServer } from "@/models/mcp";
import type { TranslateFn } from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import styles from "./RemoteMCPList.module.css";

export type RemoteMCPListProps = {
  error: string;
  hasMore: boolean;
  installedServers: readonly MCPServer[];
  installBusy: string;
  items: readonly RemoteMCPServer[];
  loading: boolean;
  loadingMore: boolean;
  onInstall?: (item: RemoteMCPServer) => Promise<boolean> | boolean;
  onLoadMore?: () => Promise<unknown> | unknown;
  onRefresh?: () => Promise<unknown> | unknown;
  onSearchChange?: (value: string) => void;
  onVisibleChange?: (visible: boolean) => void;
  search: string;
  t: TranslateFn;
};

export function RemoteMCPList({
  error,
  hasMore,
  installedServers,
  installBusy,
  items,
  loading,
  loadingMore,
  onInstall,
  onLoadMore,
  onRefresh,
  onSearchChange,
  onVisibleChange,
  search,
  t,
}: RemoteMCPListProps) {
  useEffect(() => {
    onVisibleChange?.(true);
    return () => onVisibleChange?.(false);
  }, [onVisibleChange]);

  const handleScroll = useCallback(
    (event: UIEvent<HTMLDivElement>) => {
      if (!onLoadMore || loadingMore || !hasMore) {
        return;
      }
      const target = event.currentTarget;
      if (target.scrollHeight - target.scrollTop - target.clientHeight <= 80) {
        void onLoadMore();
      }
    },
    [hasMore, loadingMore, onLoadMore],
  );

  return (
    <div className={styles.panel} role="tabpanel">
      <TextInput
        type="search"
        aria-label={t("resourcesMCPRemoteServersSearchPlaceholder")}
        value={search}
        placeholder={t("resourcesMCPRemoteServersSearchPlaceholder")}
        onChange={(event) => onSearchChange?.(event.currentTarget.value)}
      />
      {error ? (
        <div className={styles.state}>
          <span>{error}</span>
          {onRefresh ? (
            <Button size="sm" variant="secondaryGray" onClick={() => void onRefresh()}>
              <RefreshCw size={14} strokeWidth={2} aria-hidden="true" />
              {t("resourcesMCPRemoteServersRefresh")}
            </Button>
          ) : null}
        </div>
      ) : loading && !items.length ? (
        <div className={styles.state}>{t("resourcesMCPRemoteServersLoading")}</div>
      ) : items.length ? (
        <div className={styles.list} onScroll={handleScroll}>
          {items.map((item) => {
            const installKey = item.id || item.name;
            const installed = hasMCPServerName(installedServers, item.name);
            const description = item.description || item.url || item.name;
            const metadata = [item.protocol, item.url].filter(Boolean).join(" · ");
            return (
              <div className={styles.row} key={installKey}>
                <span className={styles.icon} aria-hidden="true">
                  <Server size={16} strokeWidth={2} />
                </span>
                <div className={styles.main}>
                  <span className={classNames(styles.title, "truncate")}>{item.name}</span>
                  <span className={classNames(styles.description, "truncate")}>{description}</span>
                  {metadata ? <span className={classNames(styles.meta, "truncate")}>{metadata}</span> : null}
                </div>
                <Button
                  size="sm"
                  variant="primary"
                  loading={installBusy === installKey}
                  disabled={!onInstall || Boolean(installBusy)}
                  onClick={() => void onInstall?.(item)}
                >
                  {installBusy === installKey
                    ? t("resourcesMCPRemoteInstalling")
                    : installed
                      ? t("resourcesMCPRemoteReplaceAction")
                      : t("resourcesMCPRemoteInstallAction")}
                </Button>
              </div>
            );
          })}
          {loadingMore ? <div className={styles.listState}>{t("resourcesMCPRemoteServersLoading")}</div> : null}
        </div>
      ) : (
        <div className={styles.state}>
          <CloudDownload size={16} strokeWidth={2} aria-hidden="true" />
          {t("resourcesMCPRemoteServersEmpty")}
        </div>
      )}
    </div>
  );
}
