import { useQueryClient } from "@tanstack/react-query";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { useEffect, useMemo, useState } from "react";
import { BookOpen, LoaderCircle, LogIn, Plus, RefreshCw } from "lucide-react";
import { Button, DismissibleAlert, LinkButton, Tooltip } from "@/components/ui";
import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import { agenticHubKnowledgeCreateURL } from "@/models/authEnvironment";
import { configuredKnowledgeBases, type RemoteKnowledgeBase } from "@/models/knowledgeBases";
import { ResourceFeaturedIcon, ResourceFilterTabs, ResourceSearchField } from "@/components/business/HubDetailPane";
import { KnowledgeBaseDetailDialog, type KnowledgeBaseDetailTarget } from "./KnowledgeBaseDetailDialog";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function KnowledgeBasesSection() {
  const controller = useWorkspaceControllerContext();
  const queryClient = useQueryClient();
  const [connectingID, setConnectingID] = useState("");
  const [connectFailure, setConnectFailure] = useState<{ id: string; message: string } | null>(null);
  const hub = controller.hubViewProps?.hub;
  const detail = hub?.detailPaneProps;
  const knowledgeBases = detail?.knowledgeBases;
  const t = controller.t;
  const [tab, setTab] = useState<KnowledgeTabID>("installed");
  const [query, setQuery] = useState("");
  const [detailTarget, setDetailTarget] = useState<KnowledgeBaseDetailTarget | null>(null);
  const setKnowledgeBasesVisible = hub?.setKnowledgeBasesVisible;
  useEffect(() => {
    setKnowledgeBasesVisible?.(true);
    return () => setKnowledgeBasesVisible?.(false);
  }, [setKnowledgeBasesVisible]);

  async function connectAndReveal(kb: RemoteKnowledgeBase) {
    if (connectingID) return;
    setConnectingID(kb.id);
    setConnectFailure(null);
    try {
      const payload = await fetchRemoteKnowledgeBaseMCPConfig(kb.id);
      const currentName = kb.configuredMCPName?.trim();
      const connected = currentName
        ? await detail?.onUpdateMCPInPlace?.(currentName, payload)
        : await detail?.onCreateMCPInPlace?.(payload);
      if (connected) {
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() });
        setDetailTarget(null);
        setTab("installed");
      } else {
        setConnectFailure({ id: kb.id, message: t("resourcesKnowledgeBaseConfigFailed") });
      }
    } catch {
      setConnectFailure({ id: kb.id, message: t("resourcesKnowledgeBaseConfigFailed") });
    } finally {
      setConnectingID("");
    }
  }
  const authed = isAuthenticated(controller.sidebarProps?.authStatus);

  // items contains connected knowledge bases; discoveryItems contains market results.
  const installed = useMemo(() => configuredKnowledgeBases(knowledgeBases?.items ?? []), [knowledgeBases?.items]);
  const installedVisible = useMemo(() => installed.filter((item) => matchesQuery(item, query)), [installed, query]);

  const marketItems = useMemo(
    () => (knowledgeBases?.discoveryItems ?? []).filter((item) => item.availability !== "unavailable"),
    [knowledgeBases?.discoveryItems],
  );

  const tabs = useMemo(
    () => [
      { id: "installed" as const, label: t("iaKnowledgeTabInstalled") },
      { id: "market" as const, label: t("iaKnowledgeTabMarket") },
    ],
    [t],
  );

  return (
    <SettingsRow
      title={
        <span>
          {t("iaSettingsCardKnowledge")}
          <span className={styles.countBadge}>{installed.length}</span>
        </span>
      }
      description={t("iaSettingsCardKnowledgeHint")}
    >
      {connectFailure && !detailTarget ? (
        <DismissibleAlert className="form-error" messageKey={connectFailure.message} closeLabel={t("close")}>
          <span role="alert">{connectFailure.message}</span>
        </DismissibleAlert>
      ) : null}
      <ResourceFilterTabs label={t("iaSettingsCardKnowledge")} tabs={tabs} value={tab} onChange={setTab} />

      {authed ? (
        tab === "installed" ? (
          <ResourceSearchField
            value={query}
            placeholder={t("iaKnowledgeInstalledSearchPlaceholder")}
            onChange={setQuery}
          />
        ) : (
          <ResourceSearchField
            value={knowledgeBases?.search || ""}
            placeholder={t("iaKnowledgeMarketSearchPlaceholder")}
            onChange={(value) => knowledgeBases?.setSearch?.(value)}
          />
        )
      ) : null}

      {!authed ? (
        <div className={styles.loginGate}>
          <LogIn size={20} strokeWidth={1.8} aria-hidden="true" />
          <p>{t("iaKnowledgeMarketLoginRequired")}</p>
          <Button
            variant="primary"
            size="md"
            onClick={() => controller.sidebarProps?.onRequireOpenCSGAuthentication?.()}
          >
            {t("modelProviderOpenCSGAuthorize")}
          </Button>
        </div>
      ) : tab === "installed" ? (
        <>
          {knowledgeBases?.loadError ? (
            <p className="form-error" role="alert">
              {knowledgeBases.loadError}
            </p>
          ) : null}
          {knowledgeBases?.loading && installed.length === 0 ? (
            <p className={styles.listNotice}>
              <LoaderCircle className="animate-spin" size={14} strokeWidth={2} aria-hidden="true" />
              {t("resourcesKnowledgeBasesLoading")}
            </p>
          ) : installedVisible.length === 0 ? (
            <p className={styles.listNotice}>
              {installed.length === 0 ? t("iaKnowledgeInstalledEmpty") : t("iaListSearchEmpty")}
            </p>
          ) : (
            <ResourceList>
              {installedVisible.map((item) => (
                <KnowledgeBaseCard
                  key={item.id}
                  item={item}
                  t={t}
                  onOpen={() => setDetailTarget({ kb: item, variant: "installed" })}
                />
              ))}
            </ResourceList>
          )}
        </>
      ) : (
        <MarketPane
          knowledgeBases={knowledgeBases}
          connectingID={connectingID}
          items={marketItems}
          onConnect={(item) => void connectAndReveal(item)}
          onOpenItem={(item) => setDetailTarget({ kb: item, variant: "market" })}
          onLoadMore={() => void knowledgeBases?.discoveryLoadMore?.()}
          onRetry={() => void knowledgeBases?.discoveryRefetch?.()}
          t={t}
        />
      )}

      <KnowledgeBaseDetailDialog
        target={detailTarget}
        connectError={connectFailure?.id === detailTarget?.kb.id ? connectFailure?.message : ""}
        connecting={Boolean(connectingID)}
        t={t}
        onConnect={(kb) => void connectAndReveal(kb)}
        onOpenChange={(open) => {
          if (!open) {
            setDetailTarget(null);
            setConnectFailure(null);
          }
        }}
      />
    </SettingsRow>
  );
}

type KnowledgeTabID = "installed" | "market";

type KnowledgeBasesView = NonNullable<
  NonNullable<ReturnType<typeof useWorkspaceControllerContext>["hubViewProps"]>["hub"]
>["detailPaneProps"]["knowledgeBases"];

function KnowledgeBaseCard({
  item,
  actions,
  onOpen,
  t,
}: {
  item: RemoteKnowledgeBase;
  actions?: React.ReactNode;
  onOpen?: () => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}) {
  const mcpName = String(item.configuredMCPName || "").trim();
  return (
    <ResourceListCard
      title={item.name || item.id}
      description={[
        item.description || item.name || item.id,
        mcpName ? `${t("resourcesKnowledgeBaseMCPNameLabel")}${mcpName}` : "",
      ]
        .filter(Boolean)
        .join(" · ")}
      icon={
        <ResourceFeaturedIcon>
          <BookOpen size={16} strokeWidth={2} />
        </ResourceFeaturedIcon>
      }
      active={false}
      onOpen={() => onOpen?.()}
      actions={
        actions ?? (
          <span className={styles.tag}>
            <span className={styles.tagDot} aria-hidden="true" />
            {t("iaKnowledgeBadgeAgenticHub")}
          </span>
        )
      }
    />
  );
}

function MarketPane({
  items,
  knowledgeBases,
  connectingID,
  onConnect,
  onLoadMore,
  onOpenItem,
  onRetry,
  t,
}: {
  items: readonly RemoteKnowledgeBase[];
  knowledgeBases: KnowledgeBasesView | undefined;
  connectingID: string;
  onConnect: (item: RemoteKnowledgeBase) => void;
  onLoadMore: () => void;
  onOpenItem: (item: RemoteKnowledgeBase) => void;
  onRetry: () => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}) {
  if (knowledgeBases?.discoveryLoadError) {
    return (
      <div className={styles.listNotice}>
        <span className="form-error" role="alert">
          {knowledgeBases.discoveryLoadError}
        </span>
        <Button size="sm" variant="secondaryGray" onClick={onRetry}>
          <RefreshCw size={14} strokeWidth={2} aria-hidden="true" />
          {t("resourcesSkillRemoteRefresh")}
        </Button>
      </div>
    );
  }

  if (knowledgeBases?.discoveryLoading && items.length === 0) {
    return (
      <p className={styles.listNotice}>
        <LoaderCircle className="animate-spin" size={14} strokeWidth={2} aria-hidden="true" />
        {t("resourcesKnowledgeBasesLoading")}
      </p>
    );
  }

  if (items.length === 0) {
    return <KnowledgeEmptyState t={t} />;
  }

  return (
    <>
      <ResourceList>
        {items.map((item) => {
          const connecting = connectingID === item.id;
          const connected = Boolean(String(item.configuredMCPName || "").trim());
          return (
            <KnowledgeBaseCard
              key={item.id}
              item={item}
              t={t}
              onOpen={() => onOpenItem(item)}
              actions={
                connected ? (
                  <Tooltip content={t("iaKnowledgeUpdate")}>
                    <span>
                      <Button
                        iconOnly
                        size="sm"
                        variant="secondaryGray"
                        loading={connecting}
                        aria-label={t("iaKnowledgeUpdate")}
                        disabled={Boolean(connectingID)}
                        onClick={() => onConnect(item)}
                      >
                        <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                      </Button>
                    </span>
                  </Tooltip>
                ) : (
                  <Tooltip content={t("iaKnowledgeConnect")}>
                    <span>
                      <Button
                        iconOnly
                        size="sm"
                        variant="secondaryGray"
                        loading={connecting}
                        aria-label={t("iaKnowledgeConnect")}
                        disabled={Boolean(connectingID)}
                        onClick={() => onConnect(item)}
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

      {knowledgeBases?.discoveryHasMore ? (
        <div className={styles.listMore}>
          <Button
            size="sm"
            variant="secondaryGray"
            loading={Boolean(knowledgeBases?.discoveryLoadingMore)}
            onClick={onLoadMore}
          >
            {t("iaKnowledgeMarketLoadMore")}
          </Button>
        </div>
      ) : null}

      <CreateKnowledgeHint t={t} />
    </>
  );
}

function KnowledgeEmptyState({ t }: { t: (key: string, vars?: Record<string, string | number>) => string }) {
  const { sidebarProps } = useWorkspaceControllerContext();
  const createURL = agenticHubKnowledgeCreateURL(sidebarProps?.authStatus, sidebarProps?.authEnvironment);
  return (
    <div className={styles.knowledgeEmptyState}>
      <div className={styles.knowledgeEmptyText}>
        <h3>{t("resourcesKnowledgeBasesEmpty")}</h3>
        <p>{t("resourcesKnowledgeBasesEmptyHint")}</p>
        <p>{t("iaKnowledgeCreateHint")}</p>
      </div>
      <div className={styles.knowledgeEmptyActions}>
        <LinkButton href={createURL} target="_blank" rel="noreferrer" variant="primary" size="md">
          <Plus size={16} strokeWidth={2} aria-hidden="true" />
          {t("iaKnowledgeCreateAction")}
        </LinkButton>
      </div>
    </div>
  );
}

function CreateKnowledgeHint({ t }: { t: (key: string, vars?: Record<string, string | number>) => string }) {
  const { sidebarProps } = useWorkspaceControllerContext();
  const createURL = agenticHubKnowledgeCreateURL(sidebarProps?.authStatus, sidebarProps?.authEnvironment);
  return (
    <p className={styles.createHint}>
      {t("iaKnowledgeCreateHint")}
      <a href={createURL} target="_blank" rel="noreferrer">
        {t("iaKnowledgeCreateAction")}
      </a>
    </p>
  );
}

function matchesQuery(item: { name?: string; description?: string; id?: string }, query: string): boolean {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle) {
    return true;
  }
  return [item.name, item.description, item.id].some((value) =>
    String(value ?? "")
      .toLocaleLowerCase()
      .includes(needle),
  );
}
