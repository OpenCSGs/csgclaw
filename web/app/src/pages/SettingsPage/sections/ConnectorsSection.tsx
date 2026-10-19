import { mcpOriginLabel as originLabel } from "./resourceOrigins";
import { useEffect, useMemo, useState } from "react";
import { ChevronDown, LoaderCircle, Plus } from "lucide-react";
import { GlobalAppsPanel } from "@/components/business/Apps";
import { Button, DropdownMenuContent, DropdownMenuItem, DropdownMenuRoot, DropdownMenuTrigger } from "@/components/ui";
import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { SidebarMcpIcon } from "@/components/ui/Icons";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import { isRemoteMCPInstalled, mcpServerDisplayName } from "@/models/mcp";
import {
  MCPServerCreateDialog,
  ResourceFeaturedIcon,
  ResourceFilterTabs,
  ResourceSearchField,
} from "@/components/business/HubDetailPane";
import { MCPDetailDialog, type MCPDetailTarget } from "./MCPDetailDialog";
import { MCPMarketPane } from "./MCPMarketPane";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function ConnectorsSection() {
  const controller = useWorkspaceControllerContext();
  const hub = controller.hubViewProps?.hub;
  const detail = hub?.detailPaneProps;
  const t = controller.t;
  const [appCount, setAppCount] = useState(0);
  const [appCatalogOpen, setAppCatalogOpen] = useState(false);
  const [tab, setTab] = useState<ConnectorTabID>("installed");
  const [query, setQuery] = useState("");
  const [target, setTarget] = useState<MCPDetailTarget | null>(null);

  const [createOpen, setCreateOpen] = useState(false);

  const servers = useMemo(() => hub?.mcpServers ?? [], [hub?.mcpServers]);
  const remoteServers = hub?.remoteMCPServers ?? [];
  const authed = isAuthenticated(controller.sidebarProps?.authStatus);
  const setRemoteEnabled = hub?.setRemoteMCPServersEnabled;

  useEffect(() => {
    setRemoteEnabled?.(tab === "market" && authed);
  }, [authed, setRemoteEnabled, tab]);

  async function installFromMarket(item: NonNullable<typeof hub>["remoteMCPServers"][number]) {
    const installed = await detail?.onInstallRemoteMCP?.(item);
    if (installed) {
      setTarget(null);
    }
  }

  const installedVisible = useMemo(() => servers.filter((server) => matchesQuery(server, query)), [query, servers]);

  const tabs = useMemo(
    () => [
      { id: "installed" as const, label: t("iaMCPTabInstalled") },
      { id: "market" as const, label: t("iaMCPTabMarket") },
    ],
    [t],
  );

  return (
    <SettingsRow
      title={
        <span>
          {t("iaSettingsCardConnectors")}
          <span className={styles.countBadge}>{appCount + servers.length}</span>
        </span>
      }
      description={t("iaSettingsCardConnectorsHint")}
      actions={
        <DropdownMenuRoot>
          <DropdownMenuTrigger asChild>
            <Button variant="primary" size="md">
              <Plus size={14} strokeWidth={2.2} aria-hidden="true" />
              {t("connectorsAdd")}
              <ChevronDown size={14} aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onSelect={() => setCreateOpen(true)}>{t("resourcesMCPCreateTitle")}</DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => {
                setTab("installed");
                setAppCatalogOpen(true);
              }}
            >
              {t("appAdd")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenuRoot>
      }
    >
      <ResourceFilterTabs label={t("iaSettingsCardConnectors")} tabs={tabs} value={tab} onChange={setTab} />

      {tab === "installed" ? (
        <>
          <ResourceSearchField
            value={query}
            placeholder={t("iaConnectorsInstalledSearchPlaceholder")}
            onChange={setQuery}
          />

          <section className={styles.group} aria-label={t("iaConnectorsGroupApps")}>
            <h3 className={styles.groupTitle}>
              {t("iaConnectorsGroupApps")}
              <span className={styles.countBadge}>{appCount}</span>
            </h3>
            <GlobalAppsPanel
              embedded
              showHeading={false}
              search={query}
              t={t}
              onCountChange={setAppCount}
              catalogOpen={appCatalogOpen}
              onCatalogOpenChange={setAppCatalogOpen}
            />
          </section>

          <section className={styles.group} aria-label={t("iaConnectorsGroupTools")}>
            <h3 className={styles.groupTitle}>
              {t("iaConnectorsGroupTools")}
              <span className={styles.countBadge}>{servers.length}</span>
            </h3>
            {hub?.mcpStateError ? (
              <p className="form-error" role="alert">
                {hub.mcpStateError}
              </p>
            ) : null}
            {/* Use resource-specific loading state; hub.loaded tracks templates. */}
            {!detail?.loaded ? (
              <p className={styles.listNotice}>
                <LoaderCircle className="animate-spin" size={14} strokeWidth={2} aria-hidden="true" />
                {t("resourcesLoading")}
              </p>
            ) : installedVisible.length === 0 ? (
              <p className={styles.listNotice}>
                {servers.length === 0 ? t("resourcesMCPEmpty") : t("iaListSearchEmpty")}
              </p>
            ) : (
              <ResourceList>
                {installedVisible.map((server) => (
                  <ResourceListCard
                    key={server.name}
                    title={mcpServerDisplayName(server)}
                    description={server.description || server.name}
                    icon={
                      <ResourceFeaturedIcon>
                        <SidebarMcpIcon size={16} aria-hidden="true" />
                      </ResourceFeaturedIcon>
                    }
                    active={false}
                    onOpen={() => setTarget({ variant: "installed", name: server.name })}
                    actions={
                      <span className={styles.tag}>
                        <span className={styles.tagDot} aria-hidden="true" />
                        {originLabel(server, t)}
                      </span>
                    }
                  />
                ))}
              </ResourceList>
            )}
          </section>
        </>
      ) : (
        <>
          <ResourceSearchField
            value={hub?.remoteMCPServersSearch || ""}
            placeholder={t("iaMCPMarketSearchPlaceholder")}
            onChange={(value) => hub?.setRemoteMCPServersSearch?.(value)}
          />
          <MCPMarketPane
            authed={authed}
            hub={hub}
            installedServers={servers}
            onInstall={(item) => void installFromMarket(item)}
            onLogin={() => controller.sidebarProps?.onRequireOpenCSGAuthentication?.()}
            onOpenServer={(server) => setTarget({ variant: "market", server })}
            remoteServers={remoteServers}
            t={t}
          />
        </>
      )}

      <MCPServerCreateDialog
        busy={Boolean(detail?.mcpMutationBusy)}
        createError={detail?.mcpCreateError || ""}
        initialDocument=""
        manualOnly
        open={createOpen}
        remote={{
          error: "",
          hasMore: false,
          installBusy: "",
          installedServers: servers,
          items: [],
          loading: false,
          loadingMore: false,
          search: "",
        }}
        t={t}
        onCreate={async (payload) => {
          const created = (await detail?.onCreateMCPInPlace?.(payload)) === true;
          if (created) {
            setTab("installed");
          }
          return created;
        }}
        onOpenChange={setCreateOpen}
      />

      <MCPDetailDialog
        target={target}
        t={t}
        market={
          target?.variant === "market"
            ? {
                installed: isRemoteMCPInstalled(servers, target.server),
                busy: hub?.remoteMCPInstallBusy === target.server.id,
                error: hub?.remoteMCPInstallFailure?.id === target.server.id ? hub.remoteMCPInstallFailure.message : "",
                onAction: () => void installFromMarket(target.server),
              }
            : undefined
        }
        onOpenChange={(open) => (open ? undefined : setTarget(null))}
      />
    </SettingsRow>
  );
}

type ConnectorTabID = "installed" | "market";

function matchesQuery(server: { name: string; description?: string }, query: string): boolean {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle) {
    return true;
  }
  return [server.name, server.description].some((value) =>
    String(value ?? "")
      .toLocaleLowerCase()
      .includes(needle),
  );
}
