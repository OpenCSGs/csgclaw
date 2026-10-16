import { useMCPProbe } from "./useMCPProbe";
import { useEffect, useMemo, useState } from "react";
import { Plus, RefreshCw, Trash2 } from "lucide-react";
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
  DismissibleAlert,
  Switch,
} from "@/components/ui";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { MCPProbeCards } from "./MCPDetailDialog";
import styles from "./MCPDetailDialog.module.css";
import pageStyles from "../SettingsPage.module.css";
import { knowledgeBaseMCPServer, type RemoteKnowledgeBase } from "@/models/knowledgeBases";
import type { TranslateFn } from "@/models/conversations";

export type KnowledgeBaseDetailTarget = {
  kb: RemoteKnowledgeBase;
  variant: "installed" | "market";
};

export function KnowledgeBaseDetailDialog({
  target,
  t,
  onOpenChange,
  onConnect,
  connecting = false,
  connectError = "",
  toggle,
  allowDelete = true,
  portalContainer,
  removeAction,
}: {
  target: KnowledgeBaseDetailTarget | null;
  t: TranslateFn;
  onOpenChange: (open: boolean) => void;
  onConnect?: (kb: RemoteKnowledgeBase) => void;
  connecting?: boolean;
  connectError?: string;
  /** Toggles the underlying MCP server for this agent. */
  toggle?: { enabled: boolean; busy: boolean; onToggle: () => void };

  allowDelete?: boolean;
  portalContainer?: HTMLElement | null;

  removeAction?: { busy: boolean; onRemove: () => void };
}) {
  const kb = target?.kb ?? null;
  const variant = target?.variant ?? "installed";

  const [preview, setPreview] = useState("");
  const [previewLoading, setPreviewLoading] = useState(false);
  const controller = useWorkspaceControllerContext();
  const hub = controller.hubViewProps?.hub;
  const detail = hub?.detailPaneProps;
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const { probe, probing, error: probeError, run: runProbe, reset: resetProbe } = useMCPProbe();

  const mcpName = String(kb?.configuredMCPName || "").trim();
  const mcpServer = useMemo(
    () => (kb ? knowledgeBaseMCPServer(kb, hub?.mcpServers ?? []) : null),
    [hub?.mcpServers, kb],
  );
  const connected = Boolean(String(kb?.configuredMCPName || "").trim());

  const targetKey = target ? `${target.variant}:${target.kb.id}` : "";
  const configDocument = useMemo(
    () => (mcpServer ? JSON.stringify({ mcpServers: { [mcpServer.name]: mcpServer.config } }, null, 2) : ""),
    [mcpServer],
  );

  useEffect(() => {
    setConfirmOpen(false);
    setDeleteError("");
    resetProbe();
    setPreview("");
    if (target?.variant !== "market") {
      return;
    }
    let cancelled = false;
    setPreviewLoading(true);
    void (async () => {
      try {
        const result = await fetchRemoteKnowledgeBaseMCPConfig(target.kb.id);
        if (!cancelled) {
          setPreview(JSON.stringify({ mcpServers: { [result.name]: result.config } }, null, 2));
        }
      } catch {
        // Preview failure must not block connecting the knowledge base.
      } finally {
        if (!cancelled) {
          setPreviewLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetKey]);

  async function handleDelete() {
    if (!mcpServer) {
      return;
    }
    setDeleteError("");
    const deleted = await detail?.onDeleteMCP?.(mcpServer);
    if (deleted === false) {
      setDeleteError(t("iaMCPDeleteFailed"));
      return;
    }
    setConfirmOpen(false);
    onOpenChange(false);
  }

  return (
    <>
      <DialogRoot open={Boolean(kb)} onOpenChange={onOpenChange}>
        <DialogContent className={styles.dialog} portalContainer={portalContainer}>
          {kb ? (
            <>
              <DialogHeader className={styles.header}>
                <div className={styles.headerCopy}>
                  <div className={styles.titleRow}>
                    <DialogTitle>{kb.name || kb.id}</DialogTitle>
                    <span className={pageStyles.tag}>
                      <span className={pageStyles.tagDot} aria-hidden="true" />
                      {t("iaKnowledgeBadgeAgenticHub")}
                    </span>
                  </div>
                  <DialogDescription className={styles.description}>
                    {kb.description || kb.name || kb.id}
                  </DialogDescription>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  {toggle && mcpServer ? (
                    <Switch
                      aria-label={kb.name || kb.id}
                      checked={toggle.enabled}
                      disabled={toggle.busy}
                      onCheckedChange={() => toggle.onToggle()}
                    />
                  ) : null}
                  <DialogCloseButton label={t("close")} size="md" variant="tertiaryGray" />
                </div>
              </DialogHeader>
              <DialogBody className={styles.body}>
                <MCPProbeCards probe={probe} probing={probing} error={probeError} onDismiss={resetProbe} t={t} />
                {connectError ? (
                  <DismissibleAlert className="form-error" messageKey={connectError} closeLabel={t("close")}>
                    <span role="alert">{connectError}</span>
                  </DismissibleAlert>
                ) : null}
                {variant === "market" ? (
                  previewLoading || preview ? (
                    <section className={styles.card}>
                      <h3 className={styles.cardTitle}>{t("iaMCPConfigTitle")}</h3>
                      {previewLoading ? (
                        <p className={styles.statusIdle}>{t("resourcesLoading")}</p>
                      ) : (
                        <pre className={styles.code}>{preview}</pre>
                      )}
                    </section>
                  ) : null
                ) : (
                  <section className={styles.card}>
                    <h3 className={styles.cardTitle}>{t("iaMCPConfigTitle")}</h3>
                    {mcpServer ? (
                      /* Edit the underlying connection in MCP settings. */
                      <pre className={styles.code}>{configDocument}</pre>
                    ) : (
                      <>
                        <p className={styles.statusIdle}>
                          {t(detail?.mcpStateLoading ? "resourcesLoading" : "iaKnowledgeMCPMissing")}
                        </p>
                        <Button
                          size="sm"
                          variant="secondaryGray"
                          loading={detail?.mcpStateLoading}
                          onClick={() => void hub?.refetchMCPServers?.()}
                        >
                          {t("retry")}
                        </Button>
                      </>
                    )}
                  </section>
                )}
              </DialogBody>
              <DialogFooter className={styles.footer}>
                {removeAction ? (
                  <Button
                    className={styles.footerDelete}
                    variant="danger"
                    size="sm"
                    loading={removeAction.busy}
                    onClick={() => removeAction.onRemove()}
                  >
                    <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                    {t("agentRemoveKnowledgeFromAgent")}
                  </Button>
                ) : null}
                {allowDelete && variant === "installed" && connected ? (
                  <Button
                    className={styles.footerDelete}
                    variant="danger"
                    size="sm"
                    disabled={!mcpServer || Boolean(detail?.mcpMutationBusy)}
                    onClick={() => setConfirmOpen(true)}
                  >
                    <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                    {t("iaKnowledgeDeleteAction")}
                  </Button>
                ) : null}
                {variant === "installed" && connected ? (
                  <Button
                    variant="secondaryGray"
                    size="sm"
                    disabled={!mcpServer}
                    loading={probing}
                    onClick={() => void runProbe(mcpServer)}
                  >
                    <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                    {probing ? t("resourcesMCPTesting") : t("iaMCPTestConnection")}
                  </Button>
                ) : null}
                {variant === "market" && kb ? (
                  <Button variant="primary" size="sm" loading={connecting} onClick={() => onConnect?.(kb)}>
                    {connected ? (
                      <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                    ) : (
                      <Plus size={15} strokeWidth={2.2} aria-hidden="true" />
                    )}
                    {connected ? t("iaKnowledgeUpdate") : t("iaKnowledgeConnect")}
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
              <DialogTitle>{t("iaKnowledgeDeleteTitle")}</DialogTitle>
              <DialogDescription>
                {t("iaKnowledgeDeleteConfirmMessage", { name: kb?.name || "", mcp: mcpName })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {deleteError ? (
            <p className={styles.confirmError} role="alert">
              {deleteError}
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="secondaryGray" size="sm" onClick={() => setConfirmOpen(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              loading={Boolean(detail?.mcpMutationBusy)}
              onClick={() => void handleDelete()}
            >
              {t("iaKnowledgeDeleteAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  );
}
