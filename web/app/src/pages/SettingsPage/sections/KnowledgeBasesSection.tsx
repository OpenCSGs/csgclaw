import { useQueryClient } from "@tanstack/react-query";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { useMemo, useState } from "react";
import { BookOpen, LoaderCircle, LogIn, Plus, RefreshCw } from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import { configuredKnowledgeBases, type RemoteKnowledgeBase } from "@/models/knowledgeBases";
import { ResourceFeaturedIcon, ResourceFilterTabs, ResourceSearchField } from "@/components/business/HubDetailPane";
import { KnowledgeBaseDetailDialog, type KnowledgeBaseDetailTarget } from "./KnowledgeBaseDetailDialog";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function KnowledgeBasesSection() {
  const controller = useWorkspaceControllerContext();
  const queryClient = useQueryClient();
  const [connecting, setConnecting] = useState(false);
  const [connectError, setConnectError] = useState("");
  const hub = controller.hubViewProps?.hub;
  const detail = hub?.detailPaneProps;
  const knowledgeBases = detail?.knowledgeBases;
  const t = controller.t;
  const [tab, setTab] = useState<KnowledgeTabID>("installed");
  const [query, setQuery] = useState("");
  const [detailTarget, setDetailTarget] = useState<KnowledgeBaseDetailTarget | null>(null);

  async function connectAndReveal(kb: RemoteKnowledgeBase) {
    if (connecting) return;
    setConnecting(true);
    setConnectError("");
    try {
      const payload = await fetchRemoteKnowledgeBaseMCPConfig(kb.id);
      const connected = await detail?.onCreateMCPInPlace?.(payload);
      if (connected) {
        await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() });
        setDetailTarget(null);
        setTab("installed");
      }
    } catch {
      setConnectError(t("resourcesKnowledgeBaseConfigFailed"));
    } finally {
      setConnecting(false);
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
      {connectError ? (
        <p role="alert" className="form-error">
          {connectError}
        </p>
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
        t={t}
        onConnect={(kb) => void connectAndReveal(kb)}
        onOpenChange={(open) => (open ? undefined : setDetailTarget(null))}
      />
    </SettingsRow>
  );
}

type KnowledgeTabID = "installed" | "market";

const AGENTIC_HUB_CREATE_URL = "https://opencsg.com/agentichub?view=resources&subview=rag";

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
  onConnect,
  onLoadMore,
  onOpenItem,
  onRetry,
  t,
}: {
  items: readonly RemoteKnowledgeBase[];
  knowledgeBases: KnowledgeBasesView | undefined;
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
    return (
      <>
        <p className={styles.listNotice}>
          {t("resourcesKnowledgeBasesEmpty")}
          <span className={styles.hintInline}>{t("resourcesKnowledgeBasesEmptyHint")}</span>
        </p>
        <CreateKnowledgeHint t={t} />
      </>
    );
  }

  return (
    <>
      <ResourceList>
        {items.map((item) => {
          const connecting = knowledgeBases?.copyBusyID === item.id;
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
                        disabled={Boolean(knowledgeBases?.copyBusyID)}
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
                        disabled={Boolean(knowledgeBases?.copyBusyID)}
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

function CreateKnowledgeHint({ t }: { t: (key: string, vars?: Record<string, string | number>) => string }) {
  return (
    <p className={styles.createHint}>
      {t("iaKnowledgeCreateHint")}
      <a href={AGENTIC_HUB_CREATE_URL} target="_blank" rel="noreferrer">
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
