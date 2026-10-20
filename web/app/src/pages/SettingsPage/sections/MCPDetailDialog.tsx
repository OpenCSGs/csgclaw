import { useMCPProbe } from "./useMCPProbe";
import { mcpOriginLabel as originLabel } from "./resourceOrigins";
import { useEffect, useState } from "react";
import { Plug, Plus, RefreshCw, Trash2, X } from "lucide-react";
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
  Switch,
} from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import {
  isBuiltinMCPServer,
  mcpToolParameters,
  type MCPProbeResult,
  type MCPServer,
  type RemoteMCPServer,
} from "@/models/mcp";
import { JSONConfigEditor } from "@/components/business/HubDetailPane";
import { parseMCPServerDocument } from "@/models/mcpServerDocument";
import styles from "./MCPDetailDialog.module.css";
import pageStyles from "../SettingsPage.module.css";
import type { TranslateFn } from "@/models/conversations";

export type MCPDetailTarget =
  /** Resolve installed servers by name to reflect refreshed configuration. */
  { variant: "installed"; name: string } | { variant: "market"; server: RemoteMCPServer };

export function MCPDetailDialog({
  target,
  t,
  onOpenChange,
  market,
  toggle,
  allowDelete = true,
  portalContainer,
  removeAction,
}: {
  target: MCPDetailTarget | null;
  t: TranslateFn;
  onOpenChange: (open: boolean) => void;
  market?: {
    installed: boolean;
    busy: boolean;
    error?: string;
    onAction: () => void;
  };
  /** Enables or disables this resource for the current agent. */
  toggle?: { enabled: boolean; busy: boolean; onToggle: () => void };

  allowDelete?: boolean;
  portalContainer?: HTMLElement | null;
  /** Removes the agent binding without deleting the global resource. */
  removeAction?: { busy: boolean; onRemove: () => void };
}) {
  const controller = useWorkspaceControllerContext();
  const hub = controller.hubViewProps?.hub;
  const detail = hub?.detailPaneProps;
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleteError, setDeleteError] = useState("");

  const { probe, probing, error: probeError, run: runProbe, reset: resetProbe } = useMCPProbe();

  const [draft, setDraft] = useState("");
  // Compare against the saved draft because server serialization may reorder keys.
  const [baseline, setBaseline] = useState("");
  const [draftError, setDraftError] = useState("");
  const [saving, setSaving] = useState(false);

  const installed =
    target?.variant === "installed"
      ? ((hub?.mcpServers ?? []).find((server) => server.name === target.name) ?? null)
      : null;
  const marketItem = target?.variant === "market" ? target.server : null;

  const targetKey = target
    ? target.variant === "installed"
      ? `installed:${target.name}`
      : `market:${target.server.id}`
    : "";
  const name = installed?.name ?? marketItem?.name ?? "";
  const description = installed?.description ?? marketItem?.description ?? "";

  useEffect(() => {
    setConfirmOpen(false);
    setDeleteError("");
    resetProbe();
    setDraftError("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetKey]);

  // Reset drafts only when switching targets; background refreshes must preserve edits.
  useEffect(() => {
    if (target?.variant !== "installed") {
      setDraft("");
      setBaseline("");
      return;
    }
    const doc = serverDocument(target.name, installed?.config ?? {});
    setDraft(doc);
    setBaseline(doc);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetKey]);

  const readOnlyConfig = !installed || isBuiltinMCPServer(installed);
  const dirty = Boolean(installed) && !readOnlyConfig && draft !== baseline;

  async function saveConfig() {
    if (!installed || saving) {
      return;
    }
    const parsed = parseMCPServerDocument(draft, t);
    if (parsed.kind !== "valid") {
      setDraftError(parsed.kind === "structure" ? parsed.message : t("resourcesMCPServerDocumentInvalid"));
      return;
    }
    setDraftError("");
    setSaving(true);
    try {
      // Update by persisted identity even when the document renames the server.
      const saved = await detail?.onUpdateMCP?.(installed.name, parsed.payload);
      if (saved === false) {
        setDraftError(t("resourcesMCPSaveFailed"));
        return;
      }
      setBaseline(draft);
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!installed) {
      return;
    }
    setDeleteError("");
    const deleted = await detail?.onDeleteMCP?.(installed);
    if (deleted === false) {
      setDeleteError(t("iaMCPDeleteFailed"));
      return;
    }
    setConfirmOpen(false);
    onOpenChange(false);
  }

  const canDelete = allowDelete && Boolean(installed) && !isBuiltinMCPServer(installed);

  return (
    <>
      <DialogRoot open={Boolean(target)} onOpenChange={onOpenChange}>
        <DialogContent className={styles.dialog} portalContainer={portalContainer}>
          {target ? (
            <>
              <DialogHeader>
                <div>
                  <div className={styles.titleRow}>
                    <DialogTitle>{name}</DialogTitle>
                    <span className={pageStyles.tag}>
                      <span className={pageStyles.tagDot} aria-hidden="true" />
                      {installed ? originLabel(installed, t) : t("iaMCPBadgeMarket")}
                    </span>
                  </div>
                  <DialogDescription>{description || name}</DialogDescription>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  {toggle && installed ? (
                    <Switch
                      aria-label={name}
                      checked={toggle.enabled}
                      disabled={toggle.busy}
                      onCheckedChange={() => toggle.onToggle()}
                    />
                  ) : null}
                  <DialogCloseButton label={t("close")} size="md" variant="tertiaryGray" />
                </div>
              </DialogHeader>
              <DialogBody className={styles.body}>
                {marketItem && market?.busy ? (
                  <p role="status" className={styles.statusIdle}>
                    {t("resourcesMCPRemoteInstalling")}
                  </p>
                ) : marketItem && market?.error ? (
                  <p role="alert" className={styles.confirmError}>
                    {market.error}
                  </p>
                ) : null}
                <MCPProbeCards probe={probe} probing={probing} error={probeError} onDismiss={resetProbe} t={t} />
                <section className={styles.card}>
                  <h3 className={styles.cardTitle}>{t("iaMCPConfigTitle")}</h3>
                  {installed && !readOnlyConfig ? (
                    <>
                      <JSONConfigEditor
                        hideLabel
                        label={t("iaMCPConfigTitle")}
                        value={draft}
                        invalid={Boolean(draftError)}
                        minRows={8}
                        t={t}
                        onChange={(value) => {
                          setDraft(value);
                          setDraftError("");
                        }}
                      />
                      {draftError ? (
                        <p className={styles.confirmError} role="alert">
                          {draftError}
                        </p>
                      ) : null}
                    </>
                  ) : (
                    <pre className={styles.code}>
                      {marketItem
                        ? JSON.stringify({ mcpServers: { [marketItem.id]: marketConfigPreview(marketItem) } }, null, 2)
                        : draft || JSON.stringify({ mcpServers: {} }, null, 2)}
                    </pre>
                  )}
                </section>
              </DialogBody>
              <DialogFooter className={styles.footer}>
                {removeAction ? (
                  <Button
                    className={styles.footerDelete}
                    variant="danger"
                    size="md"
                    loading={removeAction.busy}
                    onClick={() => removeAction.onRemove()}
                  >
                    <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                    {t("agentRemoveMCPFromAgent")}
                  </Button>
                ) : null}
                {canDelete && installed ? (
                  <Button
                    className={styles.footerDelete}
                    variant="danger"
                    size="md"
                    disabled={Boolean(detail?.mcpMutationBusy)}
                    onClick={() => setConfirmOpen(true)}
                  >
                    <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                    {t("resourcesMCPDelete")}
                  </Button>
                ) : null}
                {dirty ? (
                  <Button variant="primary" size="md" loading={saving} onClick={() => void saveConfig()}>
                    {t("save")}
                  </Button>
                ) : null}
                {installed ? (
                  <Button variant="secondaryGray" size="md" loading={probing} onClick={() => void runProbe(installed)}>
                    <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                    {probing ? t("resourcesMCPTesting") : t("iaMCPTestConnection")}
                  </Button>
                ) : null}
                {marketItem && market ? (
                  <Button variant="primary" size="md" loading={market.busy} onClick={market.onAction}>
                    {market.installed ? (
                      <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                    ) : (
                      <Plus size={15} strokeWidth={2.2} aria-hidden="true" />
                    )}
                    {market.busy
                      ? t("resourcesMCPRemoteInstalling")
                      : market.installed
                        ? t("resourcesMCPRemoteReplaceAction")
                        : t("resourcesMCPRemoteInstallAction")}
                  </Button>
                ) : null}
              </DialogFooter>
            </>
          ) : null}
        </DialogContent>
      </DialogRoot>

      <DialogRoot open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent className={styles.confirmDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("resourcesMCPDelete")}</DialogTitle>
              <DialogDescription>{t("iaMCPDeleteConfirmMessage", { name })}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {deleteError ? (
            <p className={styles.confirmError} role="alert">
              {deleteError}
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="secondaryGray" size="md" onClick={() => setConfirmOpen(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              size="md"
              loading={Boolean(detail?.mcpMutationBusy)}
              onClick={() => void handleDelete()}
            >
              {t("resourcesMCPDelete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  );
}

export function MCPProbeCards({
  probe,
  probing = false,
  error = "",
  onDismiss,
  t,
}: {
  probe: MCPProbeResult | null;
  probing?: boolean;
  error?: string;
  onDismiss?: () => void;
  t: TranslateFn;
}) {
  if (probing) {
    return (
      <p role="status" className={styles.statusIdle}>
        {t("resourcesMCPTesting")}
      </p>
    );
  }
  if (!probe) {
    return null;
  }
  return (
    <>
      <section className={styles.card} role={probe.connected ? "status" : "alert"}>
        <div className={styles.statusLine}>
          <h3 className={styles.cardTitle}>{t("iaMCPStatusTitle")}</h3>
          {onDismiss ? (
            <Button iconOnly variant="tertiaryGray" size="sm" aria-label={t("close")} onClick={onDismiss}>
              <X size={14} aria-hidden="true" />
            </Button>
          ) : null}
        </div>
        {probe.connected ? (
          <div className={styles.statusLine}>
            <p className={styles.statusOk}>
              <Plug size={14} strokeWidth={2} aria-hidden="true" />
              {t("iaMCPProbeConnected")}
            </p>
            <span className={styles.chips}>
              {probe.serverInfo?.name ? (
                <span className={styles.chip}>
                  {probe.serverInfo.name}
                  {probe.serverInfo.version ? ` ${probe.serverInfo.version}` : ""}
                </span>
              ) : null}
              {probe.protocolVersion ? (
                <span className={styles.chip}>{t("iaMCPProbeProtocol", { version: probe.protocolVersion })}</span>
              ) : null}
              {typeof probe.durationMs === "number" ? (
                <span className={styles.chip}>{t("iaMCPProbeDuration", { ms: probe.durationMs })}</span>
              ) : null}
            </span>
          </div>
        ) : (
          <p className={styles.statusBad}>
            <Plug size={14} strokeWidth={2} aria-hidden="true" />
            {t("iaMCPProbeFailed")}
          </p>
        )}
        {error ? <p className={styles.confirmError}>{error}</p> : null}
      </section>

      {probe.connected && probe.tools.length ? (
        <section className={styles.card}>
          <h3 className={styles.cardTitle}>
            {t("iaMCPToolsTitle")}
            <span className={styles.count}>{probe.tools.length}</span>
          </h3>
          <ul className={styles.toolList}>
            {probe.tools.map((tool) => (
              <li key={tool.name} className={styles.tool}>
                <div className={styles.toolHead}>
                  <code className={styles.toolName}>{tool.title || tool.name}</code>
                  {tool.title && tool.title !== tool.name ? <span className={styles.toolId}>{tool.name}</span> : null}
                </div>
                {tool.description ? <p className={styles.toolDesc}>{tool.description}</p> : null}
                {mcpToolParameters(tool).length ? (
                  <div className={styles.params}>
                    {mcpToolParameters(tool).map((param) => (
                      <span key={param.name} className={styles.param}>
                        <code>{param.name}</code>
                        <span className={styles.paramType}>{param.type || "any"}</span>
                        {param.required ? <em className={styles.paramRequired}>{t("iaMCPToolRequired")}</em> : null}
                      </span>
                    ))}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </>
  );
}

function serverDocument(name: string, config: MCPServer["config"]): string {
  return JSON.stringify({ mcpServers: { [name]: config } }, null, 2);
}

function marketConfigPreview(item: RemoteMCPServer) {
  return {
    url: item.url || "",
    ...(item.protocol ? { protocol: item.protocol } : {}),
    description: item.description || item.name,
  };
}
