import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArchiveRestore, Trash2 } from "lucide-react";
import { deleteArchivedAgent, fetchArchivedAgents, restoreAgent } from "@/api/agents";
import { errorMessage } from "@/api/client";
import { resolveAgentAvatarSource, type AgentLike } from "@/models/agents";
import type { TranslateFn, UsersById } from "@/models/conversations";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import {
  Button,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  DialogBody,
  DialogFooter,
  Field,
  TextInput,
} from "@/components/ui";
import styles from "../SettingsPage.module.css";

export function ArchivedAgents({
  t,
  usersById,
  onCountChange,
}: {
  t: TranslateFn;
  usersById?: UsersById;
  onCountChange?: (count: number) => void;
}) {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["archived-agents"], queryFn: fetchArchivedAgents, refetchInterval: 10000 });
  const [selection, setSelection] = useState<{ agent: AgentLike; deleting: boolean } | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const items = (query.data ?? []).map((item) => ({
    ...item,
    avatar: resolveAgentAvatarSource(item, usersById) || item.avatar,
  }));
  const selectedName = String(selection?.agent.name || selection?.agent.id || "");
  useEffect(() => {
    onCountChange?.(query.data?.length ?? 0);
  }, [query.data?.length, onCountChange]);
  function select(agent: AgentLike, deleting: boolean) {
    setSelection({ agent, deleting });
    setName("");
    setError("");
    setNotice("");
  }
  async function confirm() {
    if (!selection?.agent.id || busy || (selection.deleting && name !== selectedName)) return;
    setBusy(true);
    setError("");
    try {
      if (selection.deleting) await deleteArchivedAgent(selection.agent.id, name);
      else await restoreAgent(selection.agent.id);
      setNotice(t(selection.deleting ? "archivedAgentDeleted" : "archivedAgentRestored"));
      setSelection(null);
      await client.invalidateQueries();
    } catch (failure) {
      setError(errorMessage(failure, t("agentActionFailed")));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      {notice && (
        <p className={styles.listNotice} role="status">
          {notice}
        </p>
      )}
      {query.isPending && <p className={styles.listNotice}>{t("loading")}</p>}
      {query.error && <p role="alert">{errorMessage(query.error, t("agentActionFailed"))}</p>}
      {!query.isPending && !query.error && items.length === 0 && (
        <p className={styles.emptyState}>{t("archivedAgentsEmpty")}</p>
      )}
      <ul className={styles.sourceList}>
        {items.map((item) => (
          <li key={item.id} className={styles.sourceRow}>
            <div className={styles.sourceRowMain}>
              <span className={styles.archivedAgentName}>
                <span className={styles.archivedAgentAvatar} aria-hidden="true">
                  <AgentAvatarContent
                    avatar={item.avatar}
                    fallback={avatarFallbackText(item.avatar, item.name, item.id)}
                  />
                </span>
                <strong>{item.name || item.id}</strong>
              </span>
              <small>
                <span className={styles.kindTag}>{t("archivedAgentsTab")}</span>
                {item.archived_at && t("archivedAgentAt", { time: new Date(item.archived_at).toLocaleString() })}
              </small>
            </div>
            <Button size="sm" variant="secondaryGray" disabled={busy} onClick={() => select(item, false)}>
              <ArchiveRestore size={14} aria-hidden="true" />
              {t("archivedAgentRestore")}
            </Button>
            <Button
              iconOnly
              size="sm"
              variant="tertiaryGray"
              disabled={busy}
              aria-label={`${t("archivedAgentDelete")} ${item.name || item.id}`}
              title={t("archivedAgentDelete")}
              onClick={() => select(item, true)}
            >
              <Trash2 size={15} aria-hidden="true" />
            </Button>
          </li>
        ))}
      </ul>
      <DialogRoot
        open={Boolean(selection)}
        onOpenChange={(open) => {
          if (!open && !busy) setSelection(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <div className="min-w-0">
              <DialogTitle>
                {t(selection?.deleting ? "archivedAgentDeleteTitle" : "archivedAgentRestore", { name: selectedName })}
              </DialogTitle>
              <DialogDescription>
                {t(selection?.deleting ? "archivedAgentDeleteHint" : "archivedAgentRestoreHint", {
                  name: selectedName,
                })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} disabled={busy} />
          </DialogHeader>
          <DialogBody className="flex flex-col gap-3">
            {selection?.deleting && (
              <>
                <ul className={styles.archiveImpactList}>
                  {[
                    "archivedAgentDeleteData",
                    "archivedAgentDeleteChats",
                    "archivedAgentKeepSchedules",
                    "archivedAgentKeepHistory",
                    "archivedAgentKeepFiles",
                  ].map((key) => (
                    <li key={key}>{t(key)}</li>
                  ))}
                </ul>
                <Field label={t("archivedAgentConfirmName", { name: selectedName })}>
                  <TextInput
                    aria-label={t("archivedAgentName")}
                    value={name}
                    disabled={busy}
                    onChange={(event) => setName(event.target.value)}
                    autoComplete="off"
                    placeholder={selectedName}
                  />
                </Field>
              </>
            )}
            {error && <p role="alert">{error}</p>}
          </DialogBody>
          <DialogFooter>
            <Button variant="secondaryGray" disabled={busy} onClick={() => setSelection(null)}>
              {t("cancel")}
            </Button>
            <Button
              variant={selection?.deleting ? "danger" : "primary"}
              loading={busy}
              disabled={busy || Boolean(selection?.deleting && name !== selectedName)}
              onClick={() => void confirm()}
            >
              {t(selection?.deleting ? "archivedAgentDelete" : "archivedAgentRestore")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  );
}
