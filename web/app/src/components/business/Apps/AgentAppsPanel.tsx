import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { useState, type ReactNode } from "react";
import { BookOpen, Boxes, Plus, RefreshCw, ChevronDown, Server } from "lucide-react";
import {
  Button,
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  Switch,
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
import { ConnectorGitLabIcon } from "@/components/ui/Icons";
import { appName, appStatus, appConnectionError } from "./appForm";
import { AppToolList } from "./AppToolList";
import type { AgentAppsController } from "./useAgentApps";
import styles from "./AgentAppsPanel.module.css";

type Props = {
  agentID: string;
  children?: ReactNode;
  onAddTools?: () => void;
  controller: AgentAppsController;
  t: TranslateFn;
  portalContainer?: HTMLElement | null;
  selectedID?: string;
  addAppID?: string;
  onSelect: (id: string | undefined) => void;
};

export function AgentAppsPanel({
  controller,
  t,
  portalContainer,
  selectedID,
  addAppID,
  onSelect,
  children,
  onAddTools,
}: Props) {
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<AppInstallation | null>(null);
  const [error, setError] = useState<unknown>(null);
  const selected = controller.items.find((app) => app.installation_id === selectedID);
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
        <DropdownMenuRoot>
          <DropdownMenuTrigger asChild>
            <Button variant="primary" size="sm">
              <Plus size={16} />
              {t("connectorsAdd")}
              <ChevronDown size={14} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent portalContainer={portalContainer}>
            <DropdownMenuItem onSelect={() => setAdding(true)}>
              <Boxes size={16} />
              {t("appAddFromResources")}
            </DropdownMenuItem>
            {onAddTools ? (
              <DropdownMenuItem onSelect={onAddTools}>
                <Server size={16} />
                {t("agentMCPAdd")}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenuRoot>
      </div>
      {error || controller.error ? (
        <div role="alert" className="form-error">
          {localizeAPIError(error || controller.error, t) || errorMessage(error || controller.error)}
        </div>
      ) : null}
      {controller.loading ? (
        <p role="status" className={styles.hint}>
          {t("loading")}
        </p>
      ) : null}
      <div className={styles.agentGrid}>
        {controller.items.map((app) => (
          <button
            key={app.installation_id}
            type="button"
            className={`${styles.connectorCard} ${styles.connectorOpen}`}
            onClick={() => onSelect(app.installation_id)}
          >
            <span className={styles.connectorIcon}>
              <AppIcon appID={app.app_id} />
            </span>
            <span className={styles.connectorCopy}>
              <span className={styles.connectorTitle} title={app.name}>
                {app.name}
              </span>
              <span className={styles.connectorDescription}>
                {appName(app.app_id, t)} · {app.config.url || app.config.command}
              </span>
            </span>
            <span className={styles.status} data-status={app.enabled ? app.status : "disabled"}>
              {appStatus(app, t)}
            </span>
          </button>
        ))}
      </div>
      {!controller.items.length && !controller.loading ? <p className={styles.hint}>{t("appEmptyTitle")}</p> : null}
      {children}
      <DialogRoot
        open={Boolean(selected)}
        onOpenChange={(open) => {
          if (!open) onSelect(undefined);
        }}
      >
        <DialogContent portalContainer={portalContainer} className={styles.settingsDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{selected?.name}</DialogTitle>
              <DialogDescription>{selected ? appStatus(selected, t) : ""}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
          </DialogHeader>
          <DialogBody className={styles.settingsBody}>
            {error ? (
              <p role="alert" className="form-error">
                {localizeAPIError(error, t) || errorMessage(error)}
              </p>
            ) : null}
            {selected ? (
              <>
                {selected.resource_enabled === false ? (
                  <p className={styles.hint}>{t("appGlobalDisabledHint")}</p>
                ) : null}
                {selected.last_error ? <p className={styles.cardError}>{appConnectionError(selected, t)}</p> : null}
                {selected.disconnected ? <p className={styles.hint}>{t("appDisconnectedHint")}</p> : null}
                <AppToolList tools={selected.tools} t={t} />
                <div className={styles.actions}>
                  <a
                    className="btn btn-secondary-gray btn-sm"
                    href={`#/connectors/${encodeURIComponent(selected.resource_id || "")}`}
                  >
                    {t("appManageResource")}
                  </a>
                  <Button
                    size="sm"
                    disabled={!!controller.busyID || !selected.enabled || selected.resource_enabled === false}
                    onClick={() => void run(() => controller.connect(selected.installation_id))}
                  >
                    <RefreshCw size={14} />
                    {selected.status === "connected" ? t("appReconnect") : t("appConnect")}
                  </Button>
                  <Button
                    size="sm"
                    disabled={!!controller.busyID}
                    onClick={() =>
                      void run(() => controller.update(selected.installation_id, { enabled: !selected.enabled }))
                    }
                  >
                    {selected.enabled ? t("appDisable") : t("appEnable")}
                  </Button>
                  <Button
                    size="sm"
                    disabled={!!controller.busyID || selected.disconnected}
                    onClick={() => void run(() => controller.disconnect(selected.installation_id))}
                  >
                    {t("appDisconnect")}
                  </Button>
                  <Button
                    size="sm"
                    variant="outlineDanger"
                    disabled={!!controller.busyID}
                    onClick={() => setRemoving(selected)}
                  >
                    {t("appRemove")}
                  </Button>
                </div>
              </>
            ) : null}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
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
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
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
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
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

export function AppManagedMCPRows({
  controller,
  t,
  onSelect,
  disabled = false,
  portalContainer,
}: Pick<Props, "controller" | "t" | "onSelect" | "portalContainer"> & { disabled?: boolean }) {
  const [detailID, setDetailID] = useState("");
  const [error, setError] = useState<unknown>(null);
  const selected = controller.items.find((app) => app.installation_id === detailID);
  async function toggle(app: AppInstallation) {
    setError(null);
    try {
      await controller.update(app.installation_id, { enabled: !app.enabled });
    } catch (failure) {
      setError(failure);
    }
  }
  const busy = disabled || Boolean(controller.busyID);
  const feedback = error ? (
    <p role="alert" className="form-error">
      {localizeAPIError(error, t, t("agentResourceApplyFailed"))}
    </p>
  ) : null;
  return controller.items.length ? (
    <>
      {selected ? null : feedback}
      <ResourceList>
        {controller.items.map((app) => (
          <ResourceListCard
            key={app.installation_id}
            title={app.name}
            description={`${t("appManagedMCP")} · ${appStatus(app, t)}`}
            icon={<AppIcon appID={app.app_id} />}
            onOpen={() => setDetailID(app.installation_id)}
            actions={
              <Switch
                aria-label={app.name}
                checked={app.enabled}
                disabled={busy || (!app.enabled && app.resource_enabled === false)}
                onCheckedChange={() => void toggle(app)}
              />
            }
          />
        ))}
      </ResourceList>
      <DialogRoot
        open={Boolean(selected)}
        onOpenChange={(open) => {
          if (!open) setDetailID("");
        }}
      >
        <DialogContent portalContainer={portalContainer}>
          <DialogHeader>
            <div>
              <DialogTitle>{selected?.name}</DialogTitle>
              <DialogDescription>{t("appManagedMCP")}</DialogDescription>
            </div>
            <div className="flex shrink-0 items-center gap-4">
              {selected ? (
                <Switch
                  aria-label={selected.name}
                  checked={selected.enabled}
                  disabled={busy || (!selected.enabled && selected.resource_enabled === false)}
                  onCheckedChange={() => void toggle(selected)}
                />
              ) : null}
              <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
            </div>
          </DialogHeader>
          <DialogBody>
            {feedback}
            {selected ? (
              <>
                <p>{appStatus(selected, t)}</p>
                <AppToolList tools={selected.tools} t={t} />
                {selected.resource_enabled === false ? <p>{t("appGlobalDisabledHint")}</p> : null}
              </>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <Button
              onClick={() => {
                onSelect(detailID);
                setDetailID("");
              }}
            >
              {t("appSettings")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  ) : null;
}

export function AppIcon({ appID }: { appID: string }) {
  if (appID === "gitlab") return <ConnectorGitLabIcon size={30} className={styles.gitlabIcon} aria-hidden="true" />;
  if (appID === "feishu") return <img src="icons/feishu.png" width={28} height={28} alt="" />;
  if (appID === "github") return <img src="icons/github.svg" width={28} height={28} alt="" />;
  if (appID === "llm-wiki") return <BookOpen size={22} aria-hidden="true" />;
  return <Boxes size={22} aria-hidden="true" />;
}
