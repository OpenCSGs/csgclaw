import {
  ResourceList,
  ResourceListCard,
  ResourceStatus,
} from "@/components/business/ResourceListCard";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ArrowUpRight, BookOpen, Plus, RefreshCw } from "lucide-react";
import {
  Button,
  Switch,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogCloseButton,
  useGlobalNotice,
} from "@/components/ui";
import { errorMessage } from "@/api/client";
import { localizeAPIError } from "@/shared/i18n";
import type { AppInstallation } from "@/api/apps";
import type { TranslateFn } from "@/models/conversations";
import { settingsDialogSearch } from "@/models/workspaceDialogs";
import { ConnectorBoxesIcon, ConnectorGitLabIcon } from "@/components/ui/Icons";
import { appName, appDescription } from "./appForm";
import { appStatus, appConnectionError } from "./appStatus";
import { AppToolList } from "./AppToolList";
import type { AgentAppsController } from "./useAgentApps";
import styles from "./AgentAppsPanel.module.css";
import {
  ResourcePicker,
  ResourcePickerList,
  ResourcePickerRow,
  ResourcePickerCopy,
} from "@/components/business/ResourcePicker";

type Props = {
  agentID: string;
  cliSupported?: boolean;
  children?: ReactNode;
  controller: AgentAppsController;
  t: TranslateFn;
  portalContainer?: HTMLElement | null;
  selectedID?: string;
  addAppID?: string;
  onSelect: (id: string | undefined) => void;
};

export function AgentAppsPanel({
  agentID,
  cliSupported = true,
  controller,
  t,
  portalContainer,
  selectedID,
  addAppID,
  onSelect,
  children,
}: Props) {
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<AppInstallation | null>(null);
  const [error, setError] = useState<unknown>(null);
  const pickerSession = useRef(0);
  const location = useLocation();
  const navigate = useNavigate();
  const { showNotice } = useGlobalNotice();
  useEffect(
    () => () => {
      pickerSession.current += 1;
    },
    [agentID],
  );
  const feishu = controller.items.find((app) => app.kind === "cli" && app.app_id === "feishu");
  const selected = controller.items.find((app) => app.installation_id === selectedID);
  const selectedError = selected ? appConnectionError(selected, t) : "";
  const selectedResource = selected?.resource_id
    ? controller.resources.find((resource) => resource.installation_id === selected.resource_id)
    : undefined;
  const selectedCLISkills = selected?.skills?.length ? selected.skills : (selectedResource?.skills ?? []);
  const selectedCLIRuntimeStatus = selected?.runtime_status || selectedResource?.runtime_status;
  const available = controller.resources.filter((resource) => !addAppID || resource.app_id === addAppID);
  const connectorDescription = (app: AppInstallation) => {
    const definition = controller.definitions.find((item) => item.app_id === app.app_id);
    if (app.kind === "cli") return definition ? appDescription(definition, t) : "";
    return [definition ? "" : app.app_id, app.tools?.length ? t("iaMCPServerTools", { count: app.tools.length }) : ""]
      .filter(Boolean)
      .join(" · ");
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
  async function toggleConnector(app: AppInstallation | undefined, enabled: boolean, name: string) {
    setError(null);
    try {
      const updated = app ? await controller.update(app.installation_id, { enabled }) : await controller.bindFeishu();
      showNotice({
        message: t(updated.enabled ? "resourceEnabledNotice" : "resourceDisabledNotice", { name }),
        closeLabel: t("close"),
      });
    } catch (failure) {
      setError(failure);
    }
  }
  const closePicker = () => {
    pickerSession.current += 1;
    setAdding(false);
    onSelect(undefined);
  };
  const manageResources = (resourceID?: string) => {
    const search = new URLSearchParams(location.search);
    search.delete("connector");
    search.delete("add_connector");
    search.delete("app_resource");
    if (resourceID) search.set("app_resource", resourceID);
    closePicker();
    void navigate({ search: settingsDialogSearch(search.toString(), "connectors") }, { replace: true });
  };
  async function bindResource(resourceID: string) {
    const session = pickerSession.current;
    setError(null);
    try {
      await controller.bind(resourceID);
      if (session === pickerSession.current) closePicker();
    } catch (failure) {
      if (session === pickerSession.current) setError(failure);
    }
  }
  return (
    <section id="agent-profile-apps" className={`profile-section ${styles.panel}`}>
      <div className={styles.heading}>
        <div>
          <div className="profile-section-title">{t("profileAppConnectors")}</div>
          <p className="profile-section-description">{t("profileAppConnectorHint")}</p>
        </div>
        <div className="flex items-center gap-3">
          <span className={styles.hint}>{t("profileAppConnectorCount", { count: controller.items.length })}</span>
          <Button
            variant="secondaryGray"
            size="sm"
            iconOnly
            aria-label={t("profileAppConnectorAdd")}
            onClick={() => setAdding(true)}
          >
            <Plus size={16} />
          </Button>
        </div>
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
      <ResourceList>
        <ResourceListCard
          disabled={!feishu}
          title={appName("feishu", t)}
          description={
            !cliSupported ? t("feishuCLIUnsupported") : feishu ? connectorDescription(feishu) : t("feishuCLIWorkerHint")
          }
          icon={<AppIcon appID="feishu" />}
          onOpen={() => feishu && onSelect(feishu.installation_id)}
          actions={
            <>
              {feishu ? <ResourceStatus status={resourceStatus(feishu)}>{appStatus(feishu, t)}</ResourceStatus> : null}
              <Switch
                aria-label={t("feishuCLIWorkerSwitch")}
                checked={Boolean(feishu?.enabled)}
                disabled={!cliSupported || Boolean(controller.busyID) || feishu?.status === "applying"}
                onCheckedChange={(enabled) => void toggleConnector(feishu, enabled, appName("feishu", t))}
              />
            </>
          }
        />
        {controller.items
          .filter((app) => app.kind !== "cli" || app.app_id !== "feishu")
          .map((app) => (
            <ResourceListCard
              key={app.installation_id}
              title={app.name}
              description={connectorDescription(app)}
              icon={<AppIcon appID={app.app_id} />}
              onOpen={() => onSelect(app.installation_id)}
              actions={
                <>
                  <ResourceStatus status={resourceStatus(app)}>{appStatus(app, t)}</ResourceStatus>
                  <Switch
                    aria-label={app.name}
                    checked={app.enabled}
                    disabled={Boolean(controller.busyID) || (!app.enabled && app.resource_enabled === false)}
                    onCheckedChange={(enabled) => void toggleConnector(app, enabled, app.name)}
                  />
                </>
              }
            />
          ))}
      </ResourceList>
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
              <DialogDescription>{selected ? appName(selected.app_id, t) : ""}</DialogDescription>
            </div>
            <div className="flex shrink-0 items-center gap-4">
              <Switch
                aria-label={selected?.name || t("appEnable")}
                checked={Boolean(selected?.enabled)}
                disabled={Boolean(controller.busyID) || (!selected?.enabled && selected?.resource_enabled === false)}
                onCheckedChange={(enabled) =>
                  selected ? void toggleConnector(selected, enabled, selected.name) : undefined
                }
              />
              <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
            </div>
          </DialogHeader>
          <DialogBody className={`${styles.settingsBody} ${styles.scrollableSettingsBody}`}>
            {error ? (
              <p role="alert" className="form-error">
                {localizeAPIError(error, t) || errorMessage(error)}
              </p>
            ) : null}
            {selected ? (
              <>
                <div>
                  <ResourceStatus status={resourceStatus(selected)}>{appStatus(selected, t)}</ResourceStatus>
                </div>
                {selected.resource_enabled === false ? (
                  <p className={styles.hint}>{t("appGlobalDisabledHint")}</p>
                ) : null}
                {selectedError ? <p className={styles.cardError}>{selectedError}</p> : null}
                {selected.disconnected ? <p className={styles.hint}>{t("appDisconnectedHint")}</p> : null}
                {selected.kind === "cli" ? (
                  <div>
                    {selectedCLIRuntimeStatus === "configured" ? (
                      <p>{t("feishuCLIConfiguredRuntime")}</p>
                    ) : selectedCLIRuntimeStatus === "loaded" ? (
                      <p>{t("feishuCLILoadedRuntime")}</p>
                    ) : null}
                    {selected.app_id === "feishu" ? <p className={styles.hint}>{t("feishuCLISkillsSource")}</p> : null}
                    <ul>
                      {selectedCLISkills.map((skill) => (
                        <li key={skill.skill_name}>
                          {skill.skill_name} · {t(skill.managed ? "feishuCLIManagedSkill" : "feishuCLIUserSkill")}
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : (
                  <AppToolList tools={selected.tools} t={t} defaultOpen={false} />
                )}
              </>
            ) : null}
          </DialogBody>
          {selected ? (
            <DialogFooter className={styles.settingsFooter}>
              <div className={styles.settingsActionGroup}>
                <Button
                  size="md"
                  variant="primary"
                  className={styles.manageResourceLink}
                  onClick={() => manageResources(selected.resource_id)}
                >
                  {t("appManageResource")}
                  <ArrowUpRight size={14} />
                </Button>
                {selected.kind !== "cli" ? (
                  <>
                    <Button
                      size="md"
                      disabled={!!controller.busyID || !selected.enabled || selected.resource_enabled === false}
                      onClick={() => void run(() => controller.connect(selected.installation_id))}
                    >
                      <RefreshCw size={14} />
                      {t("appConnect")}
                    </Button>
                    <Button
                      size="md"
                      disabled={!!controller.busyID || selected.disconnected}
                      onClick={() => void run(() => controller.disconnect(selected.installation_id))}
                    >
                      {t("appDisconnect")}
                    </Button>
                  </>
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
                  {t("profileRemoveResource")}
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
        <ResourcePicker
          title={t("profileAppConnectorAdd")}
          description={t("profileAppConnectorHint")}
          closeLabel={t("close")}
          portalContainer={portalContainer}
          actions={
            <>
              <Button size="sm" variant="linkColor" onClick={() => manageResources()}>
                {t("appManageResources")}
              </Button>
            </>
          }
          footer={
            <Button size="sm" onClick={closePicker}>
              {t("close")}
            </Button>
          }
        >
          {error || controller.error ? (
            <div className="form-error" role="alert">
              {localizeAPIError(error || controller.error, t) || errorMessage(error || controller.error)}
            </div>
          ) : null}
          <ResourcePickerList>
            {available.map((resource) => {
              const bound = controller.items.some((item) => item.resource_id === resource.installation_id);
              return (
                <ResourcePickerRow key={resource.installation_id}>
                  <span className={styles.catalogIcon}>
                    <AppIcon appID={resource.app_id} />
                  </span>
                  <ResourcePickerCopy
                    name={resource.name}
                    description={bound ? t("profileResourceEnabled") : appStatus(resource, t)}
                  />
                  <Button
                    size="sm"
                    variant="primary"
                    disabled={
                      bound || !resource.enabled || !!controller.busyID || (resource.kind === "cli" && !cliSupported)
                    }
                    onClick={() => void bindResource(resource.installation_id)}
                  >
                    {t(bound ? "profileResourceEnabled" : "appEnable")}
                  </Button>
                </ResourcePickerRow>
              );
            })}
          </ResourcePickerList>
          {!available.length ? (
            <div className={styles.catalogEmpty}>
              <span className={styles.catalogEmptyIcon} aria-hidden="true">
                <ConnectorBoxesIcon size={18} />
              </span>
              <div className={styles.catalogEmptyCopy}>
                <strong>{t("appNoAvailableResourcesTitle")}</strong>
                <p>{t("appNoAvailableResources")}</p>
              </div>
            </div>
          ) : null}
        </ResourcePicker>
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
  const { showNotice } = useGlobalNotice();
  const items = controller.items.filter((app) => app.kind !== "cli");
  const selected = items.find((app) => app.installation_id === detailID);
  async function toggle(app: AppInstallation) {
    setError(null);
    try {
      const updated = await controller.update(app.installation_id, { enabled: !app.enabled });
      showNotice({
        message: t(updated.enabled ? "resourceEnabledNotice" : "resourceDisabledNotice", { name: updated.name }),
        closeLabel: t("close"),
      });
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

function resourceStatus(app: AppInstallation): "enabled" | "disabled" | "error" | "pending" {
  if (!app.enabled || app.resource_enabled === false) return "disabled";
  if (app.status === "connected") return "enabled";
  if (app.status === "error") return "error";
  return "pending";
}
