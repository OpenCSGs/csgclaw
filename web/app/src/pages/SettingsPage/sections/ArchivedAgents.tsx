import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteArchivedAgent, fetchArchivedAgents, restoreAgent } from "@/api/agents";
import { errorMessage } from "@/api/client";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import {
  Button,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  TextInput,
} from "@/components/ui";
import { SettingsRow } from "./SettingsRow";

export function ArchivedAgents({ t }: { t: TranslateFn }) {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["archived-agents"], queryFn: fetchArchivedAgents, refetchInterval: 10000 });
  const [selection, setSelection] = useState<{ agent: AgentLike; deleting: boolean } | null>(null);
  const [name, setName] = useState("");
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const items = (query.data ?? []).filter((item) =>
    String(item.name || item.id)
      .toLowerCase()
      .includes(search.trim().toLowerCase()),
  );
  function select(agent: AgentLike, deleting: boolean) {
    setSelection({ agent, deleting });
    setName("");
    setError("");
  }
  async function confirm() {
    if (!selection?.agent.id || busy) return;
    setBusy(true);
    setError("");
    try {
      if (selection.deleting) {
        await deleteArchivedAgent(selection.agent.id, name);
      } else await restoreAgent(selection.agent.id);
      setSelection(null);
      await client.invalidateQueries();
    } catch (failure) {
      setError(errorMessage(failure, t("agentActionFailed")));
    } finally {
      setBusy(false);
    }
  }
  return (
    <SettingsRow title={t("archivedAgents")} description={t("archivedAgentsHint")}>
      <div className="flex flex-col gap-3">
        <TextInput
          aria-label={t("archivedAgentName")}
          placeholder={t("archivedAgentName")}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        {query.isPending && <p>{t("loading")}</p>}
        {query.error && <p role="alert">{errorMessage(query.error, t("agentActionFailed"))}</p>}
        {!query.isPending && items.length === 0 && <p>{t("archivedAgentsEmpty")}</p>}
        {items.map((item) => (
          <div key={item.id} className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <strong>{item.name || item.id}</strong>
              {item.archived_at && <p className="text-xs">{new Date(item.archived_at).toLocaleString()}</p>}
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="secondaryGray" onClick={() => select(item, false)}>
                {t("archivedAgentRestore")}
              </Button>
              <Button size="sm" variant="danger" onClick={() => select(item, true)}>
                {t("archivedAgentDelete")}
              </Button>
            </div>
          </div>
        ))}
      </div>
      <DialogRoot
        open={Boolean(selection)}
        onOpenChange={(open) => {
          if (!open && !busy) setSelection(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <div className="min-w-0">
              <DialogTitle>{t(selection?.deleting ? "archivedAgentDelete" : "archivedAgentRestore")}</DialogTitle>
              <DialogDescription>
                {t(selection?.deleting ? "archivedAgentDeleteHint" : "archivedAgentRestoreHint", {
                  name: String(selection?.agent.name || selection?.agent.id || ""),
                })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} disabled={busy} />
          </DialogHeader>
          <div className="flex flex-col gap-3 p-4">
            {selection?.deleting && (
              <TextInput
                aria-label={t("archivedAgentName")}
                value={name}
                disabled={busy}
                onChange={(event) => setName(event.target.value)}
                autoComplete="off"
              />
            )}
            {error && <p role="alert">{error}</p>}
            <div className="flex justify-end gap-2">
              <Button variant="secondaryGray" disabled={busy} onClick={() => setSelection(null)}>
                {t("cancel")}
              </Button>
              <Button
                variant={selection?.deleting ? "danger" : "primary"}
                disabled={busy || Boolean(selection?.deleting && name !== selection.agent.name)}
                onClick={() => void confirm()}
              >
                {t(busy ? "loading" : selection?.deleting ? "archivedAgentDelete" : "archivedAgentRestore")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </DialogRoot>
    </SettingsRow>
  );
}
