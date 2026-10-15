import { LoaderCircle, LogIn, Plus, RefreshCw } from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { SidebarMcpIcon } from "@/components/ui/Icons";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ResourceFeaturedIcon, ResourceSearchField } from "@/components/business/HubDetailPane";
import styles from "../SettingsPage.module.css";
import { isRemoteMCPInstalled, type MCPServer } from "@/models/mcp";

type MCPHub = NonNullable<ReturnType<typeof useWorkspaceControllerContext>["hubViewProps"]>["hub"];

export function MCPMarketPane({
  authed,
  hub,
  installedServers,
  onInstall,
  onLogin,
  onOpenServer,
  remoteServers,
  rows = false,
  t,
}: {
  authed: boolean;
  hub: MCPHub | undefined;
  installedServers: readonly MCPServer[];
  onInstall: (item: NonNullable<MCPHub["remoteMCPServers"]>[number]) => void;
  onLogin: () => void;
  onOpenServer: (server: NonNullable<MCPHub["remoteMCPServers"]>[number]) => void;
  remoteServers: NonNullable<MCPHub["remoteMCPServers"]>;
  rows?: boolean;
  t: (key: string, vars?: Record<string, string | number>) => string;
}) {
  if (!authed) {
    return (
      <div className={styles.loginGate}>
        <LogIn size={20} strokeWidth={1.8} aria-hidden="true" />
        <p>{t("iaMCPMarketLoginRequired")}</p>
        <Button variant="primary" size="md" onClick={onLogin}>
          {t("modelProviderOpenCSGAuthorize")}
        </Button>
      </div>
    );
  }

  if (hub?.remoteMCPServersError) {
    return (
      <div className={styles.listNotice}>
        <span className="form-error" role="alert">
          {hub.remoteMCPServersError}
        </span>
        <Button size="sm" variant="secondaryGray" onClick={() => void hub?.refetchRemoteMCPServers?.()}>
          <RefreshCw size={14} strokeWidth={2} aria-hidden="true" />
          {t("resourcesMCPRemoteServersRefresh")}
        </Button>
      </div>
    );
  }

  if (hub?.remoteMCPServersLoading && remoteServers.length === 0) {
    return (
      <p className={styles.listNotice}>
        <LoaderCircle className="animate-spin" size={14} strokeWidth={2} aria-hidden="true" />
        {t("resourcesMCPRemoteServersLoading")}
      </p>
    );
  }

  if (remoteServers.length === 0) {
    return <p className={styles.listNotice}>{t("resourcesMCPRemoteServersEmpty")}</p>;
  }
  const fallbackError = hub?.remoteMCPInstallFailure ? "" : hub?.mcpMutationError;

  if (rows) {
    return (
      <>
        <ResourceSearchField
          value={hub?.remoteMCPServersSearch || ""}
          placeholder={t("iaMCPMarketSearchPlaceholder")}
          onChange={(value) => hub?.setRemoteMCPServersSearch?.(value)}
        />
        {fallbackError ? (
          <p className="form-error" role="alert">
            {fallbackError}
          </p>
        ) : null}
        <div className="agent-skill-candidates-list" role="list">
          {remoteServers.map((item) => {
            const installed = isRemoteMCPInstalled(installedServers, item);
            const installing = hub?.remoteMCPInstallBusy === item.id;
            return (
              <div key={item.id} className="agent-skill-candidate is-static">
                <ResourceFeaturedIcon>
                  <SidebarMcpIcon size={16} aria-hidden="true" />
                </ResourceFeaturedIcon>
                <button type="button" className="agent-skill-candidate-copy" onClick={() => onOpenServer(item)}>
                  <span className="agent-skill-name">{item.name}</span>
                  <span className="agent-skill-description">{item.description || item.url || item.name}</span>
                  <MCPMarketItemFeedback
                    installed={installed}
                    error={hub?.remoteMCPInstallFailure?.id === item.id ? hub.remoteMCPInstallFailure.message : ""}
                    t={t}
                  />
                </button>
                <Button
                  variant={installed ? "secondaryGray" : "primary"}
                  size="sm"
                  loading={installing}
                  disabled={Boolean(hub?.remoteMCPInstallBusy)}
                  onClick={() => onInstall(item)}
                >
                  {installed ? t("resourcesMCPRemoteReplaceAction") : t("resourcesMCPRemoteInstallAction")}
                </Button>
              </div>
            );
          })}
        </div>
        {hub?.remoteMCPServersHasMore ? (
          <div className={styles.listMore}>
            <Button
              size="sm"
              variant="secondaryGray"
              loading={Boolean(hub?.remoteMCPServersLoadingMore)}
              onClick={() => void hub?.loadMoreRemoteMCPServers?.()}
            >
              {t("iaMCPMarketLoadMore")}
            </Button>
          </div>
        ) : null}
      </>
    );
  }

  return (
    <>
      {fallbackError ? (
        <p className="form-error" role="alert">
          {fallbackError}
        </p>
      ) : null}
      <ResourceList>
        {remoteServers.map((item) => {
          const installed = isRemoteMCPInstalled(installedServers, item);
          const installing = hub?.remoteMCPInstallBusy === item.id;
          return (
            <ResourceListCard
              key={item.id}
              title={item.name}
              description={item.description || item.url || item.name}
              badge={
                <MCPMarketItemFeedback
                  installed={installed}
                  error={hub?.remoteMCPInstallFailure?.id === item.id ? hub.remoteMCPInstallFailure.message : ""}
                  t={t}
                />
              }
              icon={
                <ResourceFeaturedIcon>
                  <SidebarMcpIcon size={16} aria-hidden="true" />
                </ResourceFeaturedIcon>
              }
              active={false}
              onOpen={() => onOpenServer(item)}
              actions={
                installed ? (
                  <Tooltip content={t("resourcesMCPRemoteReplaceAction")}>
                    <span>
                      <Button
                        iconOnly
                        size="sm"
                        variant="secondaryGray"
                        aria-label={t("resourcesMCPRemoteReplaceAction")}
                        loading={installing}
                        disabled={Boolean(hub?.remoteMCPInstallBusy)}
                        onClick={() => void onInstall?.(item)}
                      >
                        <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                      </Button>
                    </span>
                  </Tooltip>
                ) : (
                  <Tooltip content={t("resourcesMCPRemoteInstallAction")}>
                    <span>
                      <Button
                        iconOnly
                        size="sm"
                        variant="secondaryGray"
                        loading={installing}
                        aria-label={t("resourcesMCPRemoteInstallAction")}
                        disabled={Boolean(hub?.remoteMCPInstallBusy)}
                        onClick={() => void onInstall?.(item)}
                      >
                        <Plus size={16} strokeWidth={2} aria-hidden="true" />
                      </Button>
                    </span>
                  </Tooltip>
                )
              }
            />
          );
        })}
      </ResourceList>

      {hub?.remoteMCPServersHasMore ? (
        <div className={styles.listMore}>
          <Button
            size="sm"
            variant="secondaryGray"
            loading={Boolean(hub?.remoteMCPServersLoadingMore)}
            onClick={() => void hub?.loadMoreRemoteMCPServers?.()}
          >
            {t("iaMCPMarketLoadMore")}
          </Button>
        </div>
      ) : null}
    </>
  );
}

function MCPMarketItemFeedback({
  installed,
  error,
  t,
}: {
  installed: boolean;
  error: string;
  t: (key: string) => string;
}) {
  return (
    <>
      {installed ? <span className={styles.marketInstalled}>{t("iaMCPTabInstalled")}</span> : null}
      {error ? (
        <span className={styles.marketInstallError} role="alert">
          {error}
        </span>
      ) : null}
    </>
  );
}
