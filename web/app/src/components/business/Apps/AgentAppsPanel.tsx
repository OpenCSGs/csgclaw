import { useState } from "react";
import { BookOpen, Boxes, GitBranch, MessageCircle, Plus, RefreshCw } from "lucide-react";
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
import { errorMessage } from "@/api/client";
import { localizeAPIError } from "@/shared/i18n";
import type { AppInstallation } from "@/api/apps";
import type { TranslateFn } from "@/models/conversations";
import { appName, appStatus, appConnectionError } from "./appForm";
import { AppToolList } from "./AppToolList";
import type { AgentAppsController } from "./useAgentApps";
import styles from "./AgentAppsPanel.module.css";

type Props = {
  agentID: string;
  controller: AgentAppsController;
  t: TranslateFn;
  portalContainer?: HTMLElement | null;
  selectedID?: string;
  addAppID?: string;
  onSelect: (id: string | undefined) => void;
};

export function AgentAppsPanel({ controller, t, portalContainer, selectedID, addAppID, onSelect }: Props) {
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<AppInstallation | null>(null);
  const [error, setError] = useState<unknown>(null);
  const available = controller.resources.filter(
    (resource) =>
      !controller.items.some((item) => item.resource_id === resource.installation_id) &&
      (!addAppID || resource.app_id === addAppID),
  );
  async function run(operation: () => Promise<unknown>) {
    setError(null);
    try {
      await operation();
      return true;
    } catch (e) {
      setError(e);
      return false;
    }
  }
  const closePicker = () => {
    setAdding(false);
    onSelect(undefined);
  };
  return (
    <section id="agent-profile-apps" className={`profile-section ${styles.panel}`}>
      <div className={styles.heading}>
        <div>
          <div className="profile-section-title">{t("agentAppsTab")}</div>
          <p className={styles.hint}>{t("appBindingDescription")}</p>
        </div>
        <Button onClick={() => setAdding(true)}>
          <Plus size={16} />
          {t("appAddFromResources")}
        </Button>
      </div>
      {error || controller.error ? (
        <div role="alert" className="form-error">
          {localizeAPIError(error || controller.error, t) || errorMessage(error || controller.error)}
        </div>
      ) : null}
      {!controller.items.length && !controller.loading ? (
        <div className={styles.empty}>
          <Boxes size={30} />
          <strong>{t("appEmptyTitle")}</strong>
          <p>{t("appBindingDescription")}</p>
        </div>
      ) : null}
      <div className={styles.list}>
        {controller.items.map((app) => (
          <article key={app.installation_id} className={styles.card} data-selected={app.installation_id === selectedID}>
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
              <span className={styles.status} data-status={app.status}>
                {appStatus(app, t)}
              </span>
            </div>
            {app.resource_enabled === false ? <p className={styles.hint}>{t("appGlobalDisabledHint")}</p> : null}
            {app.last_error ? <p className={styles.cardError}>{appConnectionError(app, t)}</p> : null}
            {app.disconnected ? <p className={styles.hint}>{t("appDisconnectedHint")}</p> : null}
            <AppToolList tools={app.tools} t={t} />
            <div className={styles.actions}>
              <a
                className="btn btn-secondary-gray btn-sm"
                href={`#/connectors/${encodeURIComponent(app.resource_id || "")}`}
              >
                {t("appManageResource")}
              </a>
              <Button
                size="sm"
                disabled={!!controller.busyID || !app.enabled || app.resource_enabled === false}
                onClick={() => void run(() => controller.connect(app.installation_id))}
              >
                <RefreshCw size={14} />
                {app.status === "connected" ? t("appReconnect") : t("appConnect")}
              </Button>
              <Button
                size="sm"
                disabled={!!controller.busyID}
                onClick={() => void run(() => controller.update(app.installation_id, { enabled: !app.enabled }))}
              >
                {app.enabled ? t("appDisable") : t("appEnable")}
              </Button>
              <Button
                size="sm"
                disabled={!!controller.busyID || app.disconnected}
                onClick={() => void run(() => controller.disconnect(app.installation_id))}
              >
                {t("appDisconnect")}
              </Button>
              <Button
                size="sm"
                variant="tertiaryDanger"
                disabled={!!controller.busyID}
                onClick={() => setRemoving(app)}
              >
                {t("appRemove")}
              </Button>
            </div>
          </article>
        ))}
      </div>
      <DialogRoot
        open={adding || !!addAppID}
        onOpenChange={(open) => {
          if (!open) closePicker();
          else setAdding(true);
        }}
      >
        <DialogContent portalContainer={portalContainer} className={styles.catalogDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("appAddFromResources")}</DialogTitle>
              <DialogDescription>{t("appBindingDescription")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody className={styles.catalog}>
            {available.map((resource) => (
              <Button
                key={resource.installation_id}
                disabled={!resource.enabled || !!controller.busyID}
                onClick={() =>
                  void run(() => controller.bind(resource.installation_id)).then((ok) => {
                    if (ok) closePicker();
                  })
                }
              >
                <AppIcon appID={resource.app_id} />
                {resource.name}
              </Button>
            ))}
            {!available.length ? <p className={styles.hint}>{t("appNoAvailableResources")}</p> : null}
            <a href={addAppID ? `#/connectors?add_connector=${encodeURIComponent(addAppID)}` : "#/connectors"}>
              {t("appManageResources")}
            </a>
          </DialogBody>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={!!removing}
        onOpenChange={(open) => {
          if (!open) setRemoving(null);
        }}
      >
        <DialogContent portalContainer={portalContainer} className={styles.catalogDialog}>
          <DialogHeader>
            <DialogTitle>{t("appRemoveConfirmTitle", { name: removing?.name || "" })}</DialogTitle>
            <DialogCloseButton label={t("close")} />
          </DialogHeader>
          <DialogBody>
            <p>{t("appUnbindDescription")}</p>
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setRemoving(null)}>{t("cancel")}</Button>
            <Button
              variant="danger"
              disabled={!!controller.busyID}
              onClick={() => {
                if (removing)
                  void run(() => controller.remove(removing.installation_id)).then((ok) => {
                    if (ok) setRemoving(null);
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

export function AppManagedMCPRows({ controller, t, onSelect }: Pick<Props, "controller" | "t" | "onSelect">) {
  return controller.items.length ? (
    <div className={styles.managedList}>
      {controller.items.map((app) => (
        <article className={styles.managedRow} key={app.installation_id}>
          <span className={styles.icon}>
            <AppIcon appID={app.app_id} />
          </span>
          <div className={styles.cardTitle}>
            <strong>{app.name}</strong>
            <span>
              {t("appManagedMCP")} · {appStatus(app, t)}
            </span>
          </div>
          <Button size="sm" onClick={() => onSelect(app.installation_id)}>
            {t("appSettings")}
          </Button>
        </article>
      ))}
    </div>
  ) : null;
}

export function AppIcon({ appID }: { appID: string }) {
  if (appID === "gitlab") return <GitBranch size={22} aria-hidden="true" />;
  if (appID === "feishu") return <MessageCircle size={22} aria-hidden="true" />;
  if (appID === "llm-wiki") return <BookOpen size={22} aria-hidden="true" />;
  return <Boxes size={22} aria-hidden="true" />;
}
