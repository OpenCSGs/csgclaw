import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Plus } from "lucide-react";
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
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogCloseButton,
} from "@/components/ui";
import { localizeAPIError } from "@/shared/i18n";
import { errorMessage } from "@/api/client";
import type { TranslateFn } from "@/models/conversations";
import { AppSettingsDialog } from "./AppSettingsDialog";
import { AppIcon } from "./AgentAppsPanel";
import { appName, appStatus } from "./appForm";
import styles from "./AgentAppsPanel.module.css";

export function GlobalAppsPanel({ t }: { t: TranslateFn }) {
  const [items, setItems] = useState<AppInstallation[]>([]);
  const [definitions, setDefinitions] = useState<AppDefinition[]>([]);
  const [adding, setAdding] = useState<AppDefinition | null>(null);
  const [catalog, setCatalog] = useState(false);
  const [removing, setRemoving] = useState<AppInstallation | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const seq = useRef(0);
  const navigate = useNavigate();
  const { resourceId } = useParams();
  const [search] = useSearchParams();
  const reload = useCallback(async (signal?: AbortSignal) => {
    const request = ++seq.current;
    try {
      const [resources, catalog] = await Promise.all([fetchAppResources(signal), fetchAppDefinitions(signal)]);
      if (!signal?.aborted && request === seq.current) {
        setItems(resources);
        setDefinitions(catalog);
        setLoaded(true);
      }
    } catch (e) {
      if (!signal?.aborted) setError(e);
    }
  }, []);
  useEffect(() => {
    const abort = new AbortController();
    void reload(abort.signal);
    const interval = setInterval(() => {
      if (!document.hidden) void reload(abort.signal);
    }, 15000);
    return () => {
      abort.abort();
      clearInterval(interval);
    };
  }, [reload]);
  const editing = items.find((item) => item.installation_id === resourceId) || null;
  const definition = adding || definitions.find((d) => d.app_id === (editing?.app_id || search.get("add_app")));
  const close = () => {
    setAdding(null);
    void navigate("/apps");
  };
  async function mutate(operation: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await operation();
      await reload();
      return true;
    } catch (e) {
      setError(e);
      return false;
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className={`entity-pane ${styles.resourcePage}`}>
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
      {error ? (
        <div className="form-error" role="alert">
          {localizeAPIError(error, t) || errorMessage(error)}
        </div>
      ) : null}
      {loaded && !items.length ? (
        <div className={styles.empty}>
          <strong>{t("appEmptyTitle")}</strong>
          <p>{t("appResourcesDescription")}</p>
        </div>
      ) : null}
      <div className={styles.list}>
        {items.map((app) => (
          <article key={app.installation_id} className={styles.card}>
            <div className={styles.cardHeader}>
              <span className={styles.icon}>
                <AppIcon appID={app.app_id} />
              </span>
              <div className={styles.cardTitle}>
                <strong>{app.name}</strong>
                <span>
                  {appName(app.app_id, t)} · {app.config.url || app.config.command}
                </span>
              </div>
              <span className={styles.status}>{appStatus(app, t)}</span>
            </div>
            <p className={styles.hint}>{t("appResourceUsers", { count: app.bindings?.length || 0 })}</p>
            {app.bindings?.length ? (
              <ul className={styles.bindingList}>
                {app.bindings.map((binding) => (
                  <li key={binding.installation_id}>
                    <a
                      href={`#/agents/${encodeURIComponent(binding.agent_id)}?tab=apps&app=${encodeURIComponent(binding.installation_id)}`}
                    >
                      {binding.agent_name || binding.agent_id}
                    </a>
                    <span>{appStatus({ ...app, enabled: binding.enabled, status: binding.status }, t)}</span>
                  </li>
                ))}
              </ul>
            ) : null}
            <div className={styles.actions}>
              <Button size="sm" onClick={() => void navigate(`/apps/${app.installation_id}`)}>
                {t("appSettings")}
              </Button>
              <Button
                size="sm"
                disabled={busy}
                onClick={() => void mutate(() => updateAppResource(app.installation_id, { enabled: !app.enabled }))}
              >
                {app.enabled ? t("appDisable") : t("appEnable")}
              </Button>
              <Button size="sm" variant="tertiaryDanger" disabled={busy} onClick={() => setRemoving(app)}>
                {t("appRemove")}
              </Button>
            </div>
          </article>
        ))}
      </div>
      {resourceId && loaded && !editing ? <p className="form-error">{t("appNotFound")}</p> : null}
      <DialogRoot open={catalog} onOpenChange={setCatalog}>
        <DialogContent className={styles.catalogDialog}>
          <DialogHeader>
            <DialogTitle>{t("appAdd")}</DialogTitle>
            <DialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody className={styles.catalog}>
            {definitions.map((definition) => (
              <Button
                key={definition.app_id}
                onClick={() => {
                  setAdding(definition);
                  setCatalog(false);
                }}
              >
                <AppIcon appID={definition.app_id} />
                {appName(definition.app_id, t)}
              </Button>
            ))}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
      {definition ? (
        <AppSettingsDialog
          key={editing?.installation_id || definition.app_id}
          globalResource
          definition={definition}
          existing={adding ? null : editing}
          hasFeishuChannel={false}
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
            else await createAppResource({ ...payload, app_id: definition.app_id, connect: false });
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
            <DialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody>
            <p>{t("appResourceUsers", { count: removing?.bindings?.length || 0 })}</p>
            <ul>
              {removing?.bindings?.map((b) => (
                <li key={b.installation_id}>{b.agent_name || b.agent_id}</li>
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
