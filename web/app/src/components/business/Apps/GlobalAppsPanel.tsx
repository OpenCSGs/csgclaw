import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { CircleAlert, Plus, MoreHorizontal } from "lucide-react";
import {
  createAppResource,
  deleteAppResource,
  fetchAppDefinitions,
  fetchAppResources,
  probeAppResource,
  updateAppResource,
  type AppDefinition,
  type AppInstallation,
} from "@/api/apps";
import {
  Button,
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DialogFooter,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogCloseButton,
  DismissibleAlert,
} from "@/components/ui";
import { localizeAPIError } from "@/shared/i18n";
import { errorMessage } from "@/api/client";
import type { TranslateFn } from "@/models/conversations";
import { pathForPane } from "@/models/routing";
import { CLIDialog } from "./CLIDialog";
import { AppSettingsDialog } from "./AppSettingsDialog";
import { AppIcon } from "./AgentAppsPanel";
import { appName, appStatus, appDescription, appConnectionError } from "./appForm";
import styles from "./AgentAppsPanel.module.css";

export function GlobalAppsPanel({
  t,
  embedded = false,
  search = "",
  catalogOpen,
  onCatalogOpenChange,
  onCountChange,
  showHeading = true,
}: {
  t: TranslateFn;
  embedded?: boolean;
  search?: string;
  catalogOpen?: boolean;
  onCatalogOpenChange?: (open: boolean) => void;
  onCountChange?: (count: number) => void;
  showHeading?: boolean;
}) {
  const [items, setItems] = useState<AppInstallation[]>([]);
  const [definitions, setDefinitions] = useState<AppDefinition[]>([]);
  const [adding, setAdding] = useState<AppDefinition | null>(null);
  const [localCatalog, setLocalCatalog] = useState(false);
  const catalog = catalogOpen ?? localCatalog;
  const setCatalog = onCatalogOpenChange ?? setLocalCatalog;
  const [error, setError] = useState<unknown>(null);
  const [removing, setRemoving] = useState<AppInstallation | null>(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const seq = useRef(0);
  const navigate = useNavigate();
  const location = useLocation();
  const [localEditingID, setLocalEditingID] = useState("");
  useEffect(() => {
    onCountChange?.(items.length);
  }, [items.length, onCountChange]);
  const { resourceId } = useParams();
  const [searchParams] = useSearchParams();
  const reload = useCallback(
    async (signal?: AbortSignal) => {
      const request = ++seq.current;
      try {
        const [resources, catalog] = await Promise.all([fetchAppResources(signal), fetchAppDefinitions(signal)]);
        if (!signal?.aborted && request === seq.current) {
          setItems(resources);
          setDefinitions(catalog);
          setLoaded(true);
          setError(null);
          if (resourceId && !resources.some((item) => item.installation_id === resourceId)) {
            void navigate(pathForPane({ type: "apps", id: "" }), {
              replace: true,
              state: { missingConnectorId: resourceId },
            });
          }
        }
      } catch (e) {
        if (!signal?.aborted && request === seq.current) setError(e);
      }
    },
    [navigate, resourceId],
  );
  useEffect(() => {
    const abort = new AbortController();
    void reload(abort.signal);
    const interval = setInterval(() => {
      if (!document.hidden) void reload(abort.signal);
    }, 15000);
    return () => {
      abort.abort();
      seq.current += 1;
      clearInterval(interval);
    };
  }, [reload]);
  const editing = items.find((item) => item.installation_id === (resourceId ?? localEditingID)) || null;
  const availableDefinitions = definitions.filter(
    (definition) =>
      definition.app_id !== "llm-wiki" &&
      !(definition.kind === "cli" && items.some((item) => item.kind === "cli" && item.app_id === definition.app_id)),
  );
  const definition =
    adding ||
    (editing
      ? definitions.find((d) => d.app_id === editing.app_id)
      : availableDefinitions.find((d) => d.app_id === searchParams.get("add_connector")));
  const close = () => {
    setAdding(null);
    setLocalEditingID("");
    void reload();
    if (resourceId || !embedded) void navigate(pathForPane({ type: "apps", id: "" }));
  };
  async function mutate(operation: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await operation();
      await reload();
      return true;
    } catch (failure) {
      setError(failure);
      return false;
    } finally {
      setBusy(false);
    }
  }
  const query = search.trim().toLocaleLowerCase();
  const matches = (name: string, description: string) =>
    !query || `${name} ${description}`.toLocaleLowerCase().includes(query);
  const describe = (item: AppInstallation) => {
    const entry = definitions.find((candidate) => candidate.app_id === item.app_id);
    return entry ? appDescription(entry, t) : item.config.url || item.config.command || appName(item.app_id, t);
  };
  const description = (item: AppInstallation) => {
    const operationInProgress =
      item.status === "connecting" ||
      item.status === "applying" ||
      item.operation?.status === "running" ||
      item.operation?.status === "pending";
    return operationInProgress ? appStatus(item, t) : item.last_error ? appConnectionError(item, t) : describe(item);
  };
  const filteredItems = items.filter((item) =>
    matches(item.name, `${appName(item.app_id, t)} ${describe(item)} ${item.config.url || item.config.command || ""}`),
  );
  return (
    <section className={embedded ? styles.applicationSection : `entity-pane ${styles.resourcePage}`}>
      {embedded ? (
        showHeading ? (
          <h2 className={styles.sectionTitle}>{t("connectorsApplications")}</h2>
        ) : null
      ) : (
        <div className={styles.heading}>
          <div>
            <h1>{t("agentAppsTab")}</h1>
            <p className={styles.hint}>{t("appResourcesDescription")}</p>
          </div>
          <Button variant="primary" onClick={() => setCatalog(true)}>
            <Plus size={16} />
            {t("appAdd")}
          </Button>
        </div>
      )}
      {error ? (
        <div className="form-error" role="alert">
          {localizeAPIError(error, t) || errorMessage(error)}
        </div>
      ) : null}
      {!loaded ? (
        <p role="status" className={styles.hint}>
          {t("loading")}
        </p>
      ) : null}
      {loaded && !filteredItems.length && !definitions.some((d) => d.kind === "cli") ? (
        <p className={styles.hint}>{t(query ? "workspaceSearchNoResults" : "appEmptyTitle")}</p>
      ) : null}
      <div className={styles.applicationGrid}>
        {loaded &&
        !items.some((item) => item.kind === "cli" && item.app_id === "feishu") &&
        matches(appName("feishu", t), t("feishuCLIIntro")) ? (
          <button
            type="button"
            className={`${styles.connectorCard} ${styles.connectorOpen}`}
            onClick={() => setAdding(definitions.find((d) => d.app_id === "feishu") ?? null)}
          >
            <span className={styles.connectorIcon}>
              <AppIcon appID="feishu" />
            </span>
            <span className={styles.connectorCopy}>
              <span className={styles.connectorTitle}>{appName("feishu", t)}</span>
              <span className={styles.connectorDescription}>{t("feishuCLIIntro")}</span>
            </span>
            <span className={styles.status}>{t("appStatusNeedsConfiguration")}</span>
          </button>
        ) : null}
        {filteredItems.map((app) => {
          const cardDescription = description(app);
          return (
            <article key={app.installation_id} className={styles.resourceConnectorRow}>
              <button
                type="button"
                className={`${styles.connectorCard} ${styles.resourceConnectorCard}`}
                onClick={() =>
                  embedded
                    ? setLocalEditingID(app.installation_id)
                    : void navigate(pathForPane({ type: "apps", id: app.installation_id }))
                }
              >
                <span className={styles.connectorIcon}>
                  <AppIcon appID={app.app_id} />
                </span>
                <span className={styles.connectorCopy}>
                  <span className={styles.connectorTitle} title={app.name}>
                    {app.name}
                  </span>
                  <span className={styles.connectorDescription} title={cardDescription}>
                    {cardDescription}
                  </span>
                </span>
                <span className={styles.status} data-status={app.enabled ? app.status : "disabled"}>
                  {appStatus(app, t)}
                </span>
              </button>
              <DropdownMenuRoot>
                <DropdownMenuTrigger asChild>
                  <Button size="sm" variant="tertiaryGray" aria-label={t("connectorsActions", { name: app.name })}>
                    <MoreHorizontal size={16} />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent>
                  <DropdownMenuItem
                    onSelect={() =>
                      embedded
                        ? setLocalEditingID(app.installation_id)
                        : void navigate(pathForPane({ type: "apps", id: app.installation_id }))
                    }
                  >
                    {t("appSettings")}
                  </DropdownMenuItem>
                  {app.kind !== "cli" ? (
                    <>
                      <DropdownMenuItem
                        disabled={busy}
                        onSelect={() =>
                          void mutate(() => updateAppResource(app.installation_id, { enabled: !app.enabled }))
                        }
                      >
                        {t(app.enabled ? "appDisable" : "appEnable")}
                      </DropdownMenuItem>
                      <DropdownMenuItem danger disabled={busy} onSelect={() => setRemoving(app)}>
                        {t("appRemove")}
                      </DropdownMenuItem>
                    </>
                  ) : null}
                </DropdownMenuContent>
              </DropdownMenuRoot>
            </article>
          );
        })}
      </div>
      {location.state?.missingConnectorId ? (
        <DismissibleAlert
          className={`${styles.cliError} ${styles.resourceError}`}
          messageKey={location.state.missingConnectorId}
          closeLabel={t("close")}
        >
          <CircleAlert size={16} aria-hidden="true" />
          <span role="alert">{t("appNotFound")}</span>
        </DismissibleAlert>
      ) : null}
      <DialogRoot open={catalog} onOpenChange={setCatalog}>
        <DialogContent className={styles.catalogDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("connectorsAddApplication")}</DialogTitle>
              <DialogDescription>{t("connectorsChooseType")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} variant="tertiaryGray" size="sm" iconOnly />
          </DialogHeader>
          <DialogBody className={styles.catalog}>
            {availableDefinitions.map((definition) => (
              <Button
                key={definition.app_id}
                className={styles.catalogItem}
                onClick={() => {
                  setAdding(definition);
                  setCatalog(false);
                }}
              >
                <span className={styles.catalogIcon}>
                  <AppIcon appID={definition.app_id} />
                </span>
                <span className={styles.catalogCopy}>
                  <strong>{appName(definition.app_id, t)}</strong>
                  <span className={styles.catalogDescription}>{appDescription(definition, t)}</span>
                </span>
              </Button>
            ))}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
      {definition && (editing?.kind === "cli" || (!editing && definition.kind === "cli")) ? (
        <CLIDialog
          key={editing?.installation_id || definition.app_id}
          definition={definition}
          existing={editing}
          t={t}
          onClose={close}
          onChanged={reload}
        />
      ) : definition ? (
        <AppSettingsDialog
          key={editing?.installation_id || definition.app_id}
          globalResource
          definition={definition}
          existing={adding ? null : editing}
          savedConnectionStatus={
            editing && !adding ? (
              <div className={styles.resourceDetails}>
                <p className={styles.hint}>{appStatus(editing, t)}</p>
                {editing.last_error ? (
                  <p role="alert" className={styles.cardError}>
                    {appConnectionError(editing, t)}
                  </p>
                ) : null}
              </div>
            ) : undefined
          }
          resourceDetails={
            editing && !adding ? (
              <div className={styles.resourceDetails}>
                <p className={styles.hint}>{t("appResourceUsers", { count: editing.bindings?.length || 0 })}</p>
                <ul className={styles.bindingList}>
                  {editing.bindings?.map((binding) => (
                    <li key={binding.installation_id}>
                      <a
                        href={`#/agents/${encodeURIComponent(binding.agent_id)}?tab=connectors&connector=${encodeURIComponent(binding.installation_id)}`}
                      >
                        {binding.agent_name || binding.agent_id}
                      </a>
                      <span>{appStatus({ ...editing, enabled: binding.enabled, status: binding.status }, t)}</span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : undefined
          }
          t={t}
          onClose={close}
          onProbe={(payload) =>
            probeAppResource({
              config: payload.config,
              credentials: payload.credentials,
              app_id: definition.app_id,
              installation_id: adding ? undefined : editing?.installation_id,
            })
          }
          onSave={async (payload) => {
            if (editing && !adding) await updateAppResource(editing.installation_id, payload);
            else await createAppResource({ ...payload, app_id: definition.app_id, connect: false, kind: "mcp" });
            await reload();
          }}
        />
      ) : null}
      <DialogRoot
        open={!!removing}
        onOpenChange={(open) => {
          if (!open) setRemoving(null);
        }}
      >
        <DialogContent className={styles.catalogDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("appRemoveConfirmTitle", { name: removing?.name || "" })}</DialogTitle>
              <DialogDescription>{t("appDeleteResourceDescription")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} variant="tertiaryGray" size="sm" iconOnly />
          </DialogHeader>
          <DialogBody>
            <p>{t("appResourceUsers", { count: removing?.bindings?.length || 0 })}</p>
            <ul>
              {removing?.bindings?.map((binding) => (
                <li key={binding.installation_id}>{binding.agent_name || binding.agent_id}</li>
              ))}
            </ul>
          </DialogBody>
          <DialogFooter>
            <Button disabled={busy} onClick={() => setRemoving(null)}>
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              disabled={busy}
              onClick={() => {
                if (removing)
                  void mutate(() => deleteAppResource(removing.installation_id)).then((ok) => {
                    if (ok) {
                      setRemoving(null);
                      close();
                    }
                  });
              }}
            >
              {t("appRemove")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </section>
  );
}
