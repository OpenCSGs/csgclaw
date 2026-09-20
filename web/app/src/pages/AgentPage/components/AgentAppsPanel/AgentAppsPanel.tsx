import { BookOpen, Boxes, GitBranch, MessageCircle, Plus, RefreshCw, Settings2, Unplug } from "lucide-react";
import { useState } from "react";
import { probeAgentApp, type AppDefinition, type AppInstallation } from "@/api/apps";
import { errorMessage } from "@/api/client";
import {
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";
import { appConnectionError, appDescription, appName, appStatus } from "./appForm";
import { AppSettingsDialog } from "./AppSettingsDialog";
import { AppToolList } from "./AppToolList";
import type { AgentAppsController } from "./useAgentApps";
import styles from "./AgentAppsPanel.module.css";

type Props = {
  agentID: string;
  controller: AgentAppsController;
  hasFeishuChannel: boolean;
  t: TranslateFn;
  portalContainer?: HTMLElement | null;
  selectedID?: string;
  addAppID?: string;
  onSelect: (id: string | undefined) => void;
};

export function AgentAppsPanel({
  agentID,
  controller,
  hasFeishuChannel,
  t,
  portalContainer,
  selectedID,
  addAppID,
  onSelect,
}: Props) {
  const [catalogOpen, setCatalogOpen] = useState(false);
  const [adding, setAdding] = useState<AppDefinition | null>(null);
  const [pendingAction, setPendingAction] = useState<{ app: AppInstallation; action: "disconnect" | "remove" } | null>(
    null,
  );
  const [actionError, setActionError] = useState("");
  const editing = selectedID ? (controller.items.find((app) => app.installation_id === selectedID) ?? null) : null;
  const definition =
    adding ||
    (editing
      ? controller.definitions.find((app) => app.app_id === editing.app_id)
      : !selectedID && addAppID
        ? controller.definitions.find((app) => app.app_id === addAppID)
        : undefined);

  async function run(operation: () => Promise<unknown>) {
    setActionError("");
    try {
      await operation();
      return true;
    } catch (failure) {
      setActionError(localizeAPIError(failure, t) || errorMessage(failure, t("appActionFailed")));
      return false;
    }
  }

  return (
    <section id="agent-profile-apps" className={`profile-section ${styles.panel}`}>
      <div className={styles.heading}>
        <div className="profile-section-heading">
          <div className="profile-section-title">{t("agentAppsTab")}</div>
          <p className="profile-section-description">{t("appPanelDescription")}</p>
        </div>
        <Button
          size="sm"
          onClick={() => setCatalogOpen(true)}
          disabled={controller.loading && !controller.definitions.length}
        >
          <Plus size={16} />
          {t("appAdd")}
        </Button>
      </div>
      {controller.error ? (
        <div className="form-error" role="alert">
          {localizeAPIError(controller.error, t) || errorMessage(controller.error, t("appLoadFailed"))}
          <Button size="sm" variant="linkGray" onClick={() => void controller.reload()}>
            {t("retry")}
          </Button>
        </div>
      ) : null}
      {actionError ? (
        <div className="form-error" role="alert">
          {actionError}
        </div>
      ) : null}
      {controller.loading && !controller.items.length ? (
        <p className={styles.hint} role="status">
          {t("appLoading")}
        </p>
      ) : null}
      {!controller.loading && !controller.error && !controller.items.length ? (
        <div className={styles.empty}>
          <Boxes size={30} aria-hidden="true" />
          <strong>{t("appEmptyTitle")}</strong>
          <p>{t("appEmptyDescription")}</p>
          <Button onClick={() => setCatalogOpen(true)}>
            <Plus size={16} />
            {t("appAdd")}
          </Button>
        </div>
      ) : null}
      <div className={styles.list}>
        {controller.items.map((app) => (
          <article key={app.installation_id} className={styles.card}>
            <div className={styles.cardHeader}>
              <span className={styles.icon}>
                <AppIcon appID={app.app_id} />
              </span>
              <div className={styles.cardTitle}>
                <strong>{app.name}</strong>
                <span>
                  {appName(app.app_id, t)}
                  {app.config.url ? ` · ${app.config.url}` : ""}
                </span>
              </div>
              <span className={styles.status} data-status={app.enabled && !app.disconnected ? app.status : "disabled"}>
                {appStatus(app, t)}
              </span>
            </div>
            {app.last_error ? <p className={styles.cardError}>{appConnectionError(app, t)}</p> : null}
            {app.disconnected ? <p className={styles.hint}>{t("appDisconnectedHint")}</p> : null}
            <AppToolList tools={app.tools} t={t} />
            <div className={styles.actions}>
              <Button size="sm" disabled={Boolean(controller.busyID)} onClick={() => onSelect(app.installation_id)}>
                <Settings2 size={14} />
                {t("appSettings")}
              </Button>
              <Button
                size="sm"
                disabled={Boolean(controller.busyID) || !app.enabled}
                loading={controller.busyID === app.installation_id}
                onClick={() => {
                  void run(() => controller.connect(app.installation_id));
                }}
              >
                <RefreshCw size={14} />
                {app.status === "connected" ? t("appReconnect") : t("appConnect")}
              </Button>
              <Button
                size="sm"
                disabled={Boolean(controller.busyID)}
                onClick={() => {
                  void run(() => controller.update(app.installation_id, { enabled: !app.enabled }));
                }}
              >
                {app.enabled ? t("appDisable") : t("appEnable")}
              </Button>
              <Button
                size="sm"
                variant="tertiaryGray"
                disabled={Boolean(controller.busyID) || app.disconnected}
                onClick={() => {
                  setActionError("");
                  setPendingAction({ app, action: "disconnect" });
                }}
              >
                <Unplug size={14} />
                {t("appDisconnect")}
              </Button>
              <Button
                size="sm"
                variant="tertiaryDanger"
                disabled={Boolean(controller.busyID)}
                onClick={() => {
                  setActionError("");
                  setPendingAction({ app, action: "remove" });
                }}
              >
                {t("appRemove")}
              </Button>
            </div>
          </article>
        ))}
      </div>
      {selectedID && !editing && !controller.loading && !controller.error ? (
        <p className="form-error">{t("appNotFound")}</p>
      ) : null}
      <DialogRoot open={catalogOpen} onOpenChange={setCatalogOpen}>
        <DialogContent className={styles.catalogDialog} portalContainer={portalContainer}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("appAdd")}</DialogTitle>
              <DialogDescription>{t("appCatalogDescription")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogBody className={styles.catalog}>
            {controller.definitions.map((app) => (
              <button
                key={app.app_id}
                className={styles.catalogItem}
                onClick={() => {
                  setCatalogOpen(false);
                  onSelect(undefined);
                  setAdding(app);
                }}
              >
                <span className={styles.icon}>
                  <AppIcon appID={app.app_id} />
                </span>
                <div>
                  <strong>{appName(app.app_id, t, app.name)}</strong>
                  <p>{appDescription(app, t)}</p>
                </div>
                <Plus size={18} aria-hidden="true" />
              </button>
            ))}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
      {definition ? (
        <AppSettingsDialog
          key={`${agentID}/${editing?.installation_id || definition.app_id}`}
          definition={definition}
          existing={adding ? null : editing}
          hasFeishuChannel={hasFeishuChannel}
          t={t}
          portalContainer={portalContainer}
          onClose={() => {
            setAdding(null);
            onSelect(undefined);
          }}
          onProbe={(payload) =>
            probeAgentApp(agentID, {
              config: payload.config,
              credentials: payload.credentials,
              app_id: definition.app_id,
              installation_id: adding ? undefined : editing?.installation_id,
            })
          }
          onSave={async (payload, connect) => {
            if (editing && !adding) {
              await controller.update(editing.installation_id, payload);
              if (connect) await controller.connect(editing.installation_id);
            } else {
              await controller.create({ ...payload, app_id: definition.app_id, connect });
            }
          }}
        />
      ) : null}
      <DialogRoot
        open={Boolean(pendingAction)}
        onOpenChange={(open) => {
          if (!open && !controller.busyID) setPendingAction(null);
        }}
      >
        <DialogContent portalContainer={portalContainer}>
          <DialogHeader>
            <div>
              <DialogTitle>
                {t(pendingAction?.action === "remove" ? "appRemoveTitle" : "appDisconnectTitle", {
                  name: pendingAction?.app.name || "",
                })}
              </DialogTitle>
              <DialogDescription>
                {t(pendingAction?.action === "remove" ? "appRemoveDescription" : "appDisconnectDescription")}
              </DialogDescription>
            </div>
            <DialogCloseButton
              label={t("close")}
              disabled={Boolean(controller.busyID)}
              size="sm"
              variant="tertiaryGray"
            />
          </DialogHeader>
          {actionError ? (
            <DialogBody>
              <p className="form-error" role="alert">
                {actionError}
              </p>
            </DialogBody>
          ) : null}
          <DialogFooter>
            <Button disabled={Boolean(controller.busyID)} onClick={() => setPendingAction(null)}>
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              loading={Boolean(controller.busyID)}
              onClick={() => {
                if (!pendingAction) return;
                void run(() =>
                  pendingAction.action === "remove"
                    ? controller.remove(pendingAction.app.installation_id)
                    : controller.disconnect(pendingAction.app.installation_id),
                ).then((success) => {
                  if (success) setPendingAction(null);
                });
              }}
            >
              {t(pendingAction?.action === "remove" ? "appRemove" : "appDisconnect")}
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

function AppIcon({ appID }: { appID: string }) {
  if (appID === "gitlab") return <GitBranch size={22} aria-hidden="true" />;
  if (appID === "feishu") return <MessageCircle size={22} aria-hidden="true" />;
  if (appID === "llm-wiki") return <BookOpen size={22} aria-hidden="true" />;
  return <Boxes size={22} aria-hidden="true" />;
}
