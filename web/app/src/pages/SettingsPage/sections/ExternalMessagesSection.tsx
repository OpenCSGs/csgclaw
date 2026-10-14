import { useCallback, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
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
import { NotifierControls } from "@/components/business/ProfileControls/NotifierControls";
import {
  createNotificationSourceRequest,
  deleteNotificationSourceRequest,
  fetchNotificationParticipants,
  updateNotificationParticipantRequest,
} from "@/api/agents";
import { errorMessage } from "@/api/client";
import { useWorkspaceBootstrapConfigQuery, workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import {
  draftNotifierDetailsFromDraft,
  ensureNotifierPullSubscriptionDraft,
  normalizeNotifierDeliveryMode,
  resolvedNotifierWebhookOrigin,
  type AgentDraft,
} from "@/models/agents";
import type { ParticipantLike } from "@/api/agents";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function ExternalMessagesSection() {
  const controller = useWorkspaceControllerContext();
  const t = controller.t;
  const bootstrapConfig = useWorkspaceBootstrapConfigQuery();
  const queryClient = useQueryClient();

  const [sources, setSources] = useState<ParticipantLike[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyID, setBusyID] = useState("");
  const [error, setError] = useState("");
  // null closes the dialog; { item: null } opens a new source.
  const [editing, setEditing] = useState<{ item: ParticipantLike | null } | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const list = await fetchNotificationParticipants();
      setSources(Array.isArray(list) ? list : []);
      setError("");
    } catch (err) {
      setError(errorMessage(err, t("externalMessagesLoadFailed")));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void load();
    // Load once per mount, regardless of locale changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Room participants come from the cached workspace bootstrap.
  const refreshWorkspace = () => queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.bootstrap() });

  async function handleDelete(item: ParticipantLike) {
    if (!item.id || busyID) {
      return;
    }
    setBusyID(item.id);
    setError("");
    try {
      await deleteNotificationSourceRequest(item.id);
      await load();
      await refreshWorkspace();
    } catch (err) {
      setError(errorMessage(err, t("externalMessagesSaveFailed")));
    } finally {
      setBusyID("");
    }
  }

  return (
    <SettingsRow title={t("iaSettingsCardExternal")} description={t("externalMessagesDescription")}>
      {loading ? (
        <p className={styles.hintInline}>{t("loading")}</p>
      ) : (
        <>
          <ul className={styles.sourceList}>
            {sources.map((item) => {
              const metadata = (item.metadata || {}) as Record<string, unknown>;
              return (
                <li key={item.id}>
                  <div className={styles.sourceRow}>
                    <span className={styles.sourceRowMain}>
                      <strong>{item.name || item.id}</strong>
                      <small>
                        {deliveryLabel(metadata, t)} ·{" "}
                        {String(metadata.webhook_token ?? "").trim()
                          ? t("externalMessagesTokenSet")
                          : t("externalMessagesTokenUnset")}
                      </small>
                    </span>
                    <Button
                      size="sm"
                      variant="secondaryGray"
                      disabled={Boolean(busyID)}
                      onClick={() => setEditing({ item })}
                    >
                      <Pencil aria-hidden="true" size={14} />
                      {t("edit")}
                    </Button>
                    <Button
                      iconOnly
                      size="sm"
                      variant="tertiaryGray"
                      aria-label={`${t("delete")} ${item.name || ""}`}
                      title={t("delete")}
                      loading={busyID === item.id}
                      disabled={Boolean(busyID)}
                      onClick={() => void handleDelete(item)}
                    >
                      <Trash2 aria-hidden="true" size={15} />
                    </Button>
                  </div>
                </li>
              );
            })}
            {!sources.length ? (
              <li>
                <p className={styles.hintInline}>{t("externalMessagesEmpty")}</p>
              </li>
            ) : null}
          </ul>

          <div className={styles.formActions}>
            <Button
              variant="secondaryGray"
              size="md"
              disabled={Boolean(busyID)}
              onClick={() => setEditing({ item: null })}
            >
              <Plus aria-hidden="true" size={15} />
              {t("externalMessagesCreate")}
            </Button>
          </div>

          {error ? <p className="form-error">{error}</p> : null}
          <p className={styles.hintInline}>{t("externalMessagesWhereHint")}</p>

          {editing ? (
            <SourceDialog
              key={editing.item?.id || "new"}
              item={editing.item}
              webhookPublicOrigin={resolvedNotifierWebhookOrigin(bootstrapConfig.data ?? null)}
              t={t}
              onClose={() => setEditing(null)}
              onSaved={async () => {
                setEditing(null);
                await load();
                await refreshWorkspace();
              }}
            />
          ) : null}
        </>
      )}
    </SettingsRow>
  );
}

function SourceDialog({
  item,
  webhookPublicOrigin,
  t,
  onClose,
  onSaved,
}: {
  item: ParticipantLike | null;
  webhookPublicOrigin: string;
  t: (key: string, params?: Record<string, string | number>) => string;
  onClose: () => void;
  onSaved: () => void | Promise<void>;
}) {
  const creating = !item;
  const [name, setName] = useState(String(item?.name || ""));
  const [draft, setDraft] = useState<AgentDraft>(() => draftFromParticipant(item));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit() {
    const trimmed = name.trim();
    if (!trimmed || busy) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      if (creating) {
        const created = await createNotificationSourceRequest({ name: trimmed });
        if (created?.id) {
          await updateNotificationParticipantRequest(created.id, {
            metadata: draftNotifierDetailsFromDraft(draft) || {},
          });
        }
      } else if (item?.id) {
        await updateNotificationParticipantRequest(item.id, {
          name: trimmed,
          metadata: draftNotifierDetailsFromDraft(draft) || {},
        });
      }
      await onSaved();
    } catch (err) {
      setError(errorMessage(err, t("externalMessagesSaveFailed")));
    } finally {
      setBusy(false);
    }
  }

  return (
    <DialogRoot open onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent className={styles.sourceDialog}>
        <DialogHeader>
          <div>
            <DialogTitle>{creating ? t("externalMessagesCreate") : t("externalMessagesEditTitle")}</DialogTitle>
            <DialogDescription>{t("externalMessagesDialogHint")}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} size="md" variant="tertiaryGray" />
        </DialogHeader>
        <DialogBody>
          <label className="field">
            <span>{t("externalMessagesNameLabel")}</span>
            <input
              autoFocus
              value={name}
              placeholder={t("externalMessagesNamePlaceholder")}
              onChange={(event) => setName(event.currentTarget.value)}
            />
          </label>
          <NotifierControls
            agentID={item?.id || ""}
            draft={draft}
            hideHeading
            onPatch={(patch) =>
              setDraft((current) => ensureNotifierPullSubscriptionDraft({ ...current, ...patch }) as AgentDraft)
            }
            t={t}
            webhookPublicOrigin={webhookPublicOrigin}
          />
          {error ? <p className="form-error">{error}</p> : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={busy}
            disabled={!name.trim() || busy}
            onClick={() => void handleSubmit()}
          >
            {creating ? t("create") : t("save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

function deliveryLabel(metadata: Record<string, unknown>, t: (key: string) => string): string {
  return normalizeNotifierDeliveryMode(metadata.delivery_mode || "webhook") === "remote_pull"
    ? t("notifierDeliveryRemotePull")
    : t("notifierDeliveryWebhook");
}

function draftFromParticipant(item: ParticipantLike | null): AgentDraft {
  const metadata = (item?.metadata || {}) as Record<string, unknown>;
  return ensureNotifierPullSubscriptionDraft({
    notifier_delivery_mode: normalizeNotifierDeliveryMode(metadata.delivery_mode || "webhook"),
    notifier_webhook_token: String(metadata.webhook_token ?? ""),
    notifier_remote_url: String(metadata.remote_url ?? ""),
    notifier_remote_subscription_id: String(metadata.remote_subscription_id ?? ""),
    notifier_poll_interval: String(metadata.poll_interval ?? ""),
    notifier_remote_token: String(metadata.remote_token ?? ""),
    notifier_remote_messages_url: String(metadata.remote_messages_url ?? ""),
    notifier_remote_ack_url: String(metadata.remote_ack_url ?? ""),
  } as AgentDraft) as AgentDraft;
}
