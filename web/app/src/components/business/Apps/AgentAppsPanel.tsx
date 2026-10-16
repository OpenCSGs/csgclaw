import { ResourceList, ResourceListCard } from "@/components/business/ResourceListCard";
import { useState, type ReactNode } from "react";
import { ArrowRight, ArrowUpRight, BookOpen, Plus, RefreshCw, ChevronDown, Server } from "lucide-react";
import {
  Button,
  LinkButton,
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
import { ConnectorBoxesIcon, ConnectorGitLabIcon } from "@/components/ui/Icons";
import { appName, appStatus, appConnectionError, appDescription } from "./appForm";
import { AppToolList } from "./AppToolList";
import type { AgentAppsController } from "./useAgentApps";
import styles from "./AgentAppsPanel.module.css";

type Props = {
  agentID: string;
  cliSupported?: boolean;
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
  cliSupported = true,
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
  const feishu = controller.items.find((app) => app.kind === "cli" && app.app_id === "feishu");
  const selected = controller.items.find((app) => app.installation_id === selectedID);
  const available = controller.resources.filter(
    (resource) =>
      !controller.items.some((item) => item.resource_id === resource.installation_id) &&
      (!addAppID || resource.app_id === addAppID),
  );
  const manageResourcesHref = addAppID ? `#/connectors?add_connector=${encodeURIComponent(addAppID)}` : "#/connectors";
  const connectorDescription = (app: AppInstallation) => {
    const definition = controller.definitions.find((item) => item.app_id === app.app_id);
    return definition ? appDescription(definition, t) : "";
  };
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
              <ConnectorBoxesIcon size={16} />
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
        <article
          className={`${styles.connectorCard} ${feishu ? styles.connectorOpen : ""}`}
          role={feishu ? "button" : undefined}
          tabIndex={feishu ? 0 : undefined}
          onClick={() => feishu && onSelect(feishu.installation_id)}
          onKeyDown={(event) => {
            if (!feishu || (event.key !== "Enter" && event.key !== " ")) return;
            event.preventDefault();
            onSelect(feishu.installation_id);
          }}
        >
          <span className={styles.connectorIcon}>
            <AppIcon appID="feishu" />
          </span>
          <span className={styles.connectorCopy}>
            <span className={styles.connectorTitle}>{appName("feishu", t)}</span>
            <span className={styles.connectorDescription}>
              {!cliSupported
                ? t("feishuCLIUnsupported")
                : feishu
                  ? connectorDescription(feishu)
                  : t("feishuCLIWorkerHint")}
            </span>
          </span>
          <span
            className={styles.connectorToggle}
            onClick={(event) => event.stopPropagation()}
            onKeyDown={(event) => event.stopPropagation()}
          >
            <Switch
              aria-label={t("feishuCLIWorkerSwitch")}
              checked={Boolean(feishu?.enabled)}
              size="sm"
              disabled={!cliSupported || Boolean(controller.busyID) || feishu?.status === "applying"}
              onCheckedChange={(enabled) =>
                void run(() =>
                  feishu ? controller.update(feishu.installation_id, { enabled }) : controller.bindFeishu(),
                )
              }
            />
          </span>
        </article>
        {controller.items
          .filter((app) => app.kind !== "cli" || app.app_id !== "feishu")
          .map((app) => (
            <article
              key={app.installation_id}
              className={`${styles.connectorCard} ${styles.connectorOpen}`}
              role="button"
              tabIndex={0}
              onClick={() => onSelect(app.installation_id)}
              onKeyDown={(event) => {
                if (event.key !== "Enter" && event.key !== " ") return;
                event.preventDefault();
                onSelect(app.installation_id);
              }}
            >
              <span className={styles.connectorIcon}>
                <AppIcon appID={app.app_id} />
              </span>
              <span className={styles.connectorCopy}>
                <span className={styles.connectorTitle} title={app.name}>
                  {app.name}
                </span>
                <span className={styles.connectorDescription}>{connectorDescription(app)}</span>
              </span>
              <span
                className={styles.connectorToggle}
                onClick={(event) => event.stopPropagation()}
                onKeyDown={(event) => event.stopPropagation()}
              >
                <Switch
                  aria-label={app.name}
                  checked={app.enabled}
                  size="sm"
                  disabled={Boolean(controller.busyID) || (!app.enabled && app.resource_enabled === false)}
                  onCheckedChange={(enabled) => void run(() => controller.update(app.installation_id, { enabled }))}
                />
              </span>
            </article>
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
            <div className="flex shrink-0 items-center gap-4">
              <Switch
                aria-label={selected?.name || t("appEnable")}
                checked={Boolean(selected?.enabled)}
                disabled={Boolean(controller.busyID) || (!selected?.enabled && selected?.resource_enabled === false)}
                onCheckedChange={(enabled) =>
                  selected ? void run(() => controller.update(selected.installation_id, { enabled })) : undefined
                }
              />
              <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
            </div>
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
                {selected.kind === "cli" ? (
                  <div>
                    {selected.runtime_status === "configured" ? (
                      <p>{t("feishuCLIConfiguredRuntime")}</p>
                    ) : selected.runtime_status === "loaded" ? (
                      <p>{t("feishuCLILoadedRuntime")}</p>
                    ) : null}
                    {selected.app_id === "feishu" ? <p className={styles.hint}>{t("feishuCLISkillsSource")}</p> : null}
                    <ul>
                      {selected.skills?.map((skill) => (
                        <li key={skill.skill_name}>
                          {skill.skill_name} · {t(skill.managed ? "feishuCLIManagedSkill" : "feishuCLIUserSkill")}
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : (
                  <AppToolList tools={selected.tools} t={t} fillAvailableHeight />
                )}
              </>
            ) : null}
          </DialogBody>
          {selected ? (
            <DialogFooter className={styles.settingsFooter}>
              <div className={styles.settingsActionGroup}>
                <LinkButton
                  size="md"
                  variant="primary"
                  className={styles.manageResourceLink}
                  href={`#/connectors/${encodeURIComponent(selected.resource_id || "")}`}
                >
                  {t("appManageResource")}
                  <ArrowUpRight size={14} />
                </LinkButton>
                {selected.kind !== "cli" ? (
                  <Button
                    size="md"
                    disabled={!!controller.busyID || !selected.enabled || selected.resource_enabled === false}
                    onClick={() => void run(() => controller.connect(selected.installation_id))}
                  >
                    <RefreshCw size={14} />
                    {selected.status === "connected" ? t("appReconnect") : t("appConnect")}
                  </Button>
                ) : null}
                {selected.kind !== "cli" ? (
                  <Button
                    size="md"
                    disabled={!!controller.busyID || selected.disconnected}
                    onClick={() => void run(() => controller.disconnect(selected.installation_id))}
                  >
                    {t("appDisconnect")}
                  </Button>
                ) : null}
              </div>
              {selected.kind !== "cli" ? (
                <Button
                  size="md"
                  variant="danger"
                  className={styles.settingsRemoveButton}
                  disabled={!!controller.busyID}
                  onClick={() => setRemoving(selected)}
                >
                  {t("appRemove")}
                </Button>
              ) : null}
            </DialogFooter>
          ) : null}
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
                className={styles.catalogItem}
                disabled={!resource.enabled || !!controller.busyID || (resource.kind === "cli" && !cliSupported)}
                onClick={() =>
                  void run(() => controller.bind(resource.installation_id)).then((ok) => {
                    if (ok) closePicker();
                  })
                }
              >
                <span className={styles.catalogIcon}>
                  <AppIcon appID={resource.app_id} />
                </span>
                <span className={styles.catalogCopy}>
                  <strong>{resource.name}</strong>
                  <span className={styles.catalogDescription}>
                    {appName(resource.app_id, t)} · {resource.config.url || resource.config.command}
                  </span>
                </span>
              </Button>
            ))}
            {!available.length ? (
              <div className={styles.catalogEmpty}>
                <span className={styles.catalogEmptyIcon} aria-hidden="true">
                  <ConnectorBoxesIcon size={18} />
                </span>
                <div className={styles.catalogEmptyCopy}>
                  <strong>{t("appNoAvailableResourcesTitle")}</strong>
                  <p>{t("appNoAvailableResources")}</p>
                </div>
                <LinkButton size="sm" className={styles.catalogManageAction} href={manageResourcesHref}>
                  <ConnectorBoxesIcon size={16} />
                  {t("appManageResources")}
                  <ArrowRight size={14} />
                </LinkButton>
              </div>
            ) : (
              <LinkButton size="sm" className={styles.catalogManageAction} href={manageResourcesHref}>
                <ConnectorBoxesIcon size={16} />
                {t("appManageResources")}
                <ArrowRight size={14} />
              </LinkButton>
            )}
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
  const items = controller.items.filter((app) => app.kind !== "cli");
  const selected = items.find((app) => app.installation_id === detailID);
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
  return items.length ? (
    <>
      {selected ? null : feedback}
      <ResourceList>
        {items.map((app) => (
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
  return <ConnectorBoxesIcon size={22} aria-hidden="true" />;
}
