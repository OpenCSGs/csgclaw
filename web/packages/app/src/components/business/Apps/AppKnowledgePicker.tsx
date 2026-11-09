import { useState } from "react";
import { fetchRemoteKnowledgeBaseMCPConfig, fetchRemoteKnowledgeBases } from "@/api/knowledgeBases";
import { errorMessage } from "@/api/client";
import { Button, Field, Select, TextInput } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";
import { localizeAPIError } from "@/shared/i18n";
import styles from "./AgentAppsPanel.module.css";

function useAppKnowledgePicker(t: TranslateFn, onSelect: (url: string) => void) {
  const [items, setItems] = useState<RemoteKnowledgeBase[]>([]);
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState("");
  const [nextPage, setNextPage] = useState<number | undefined>();
  async function load(page = 1) {
    setBusy(true);
    setError("");
    try {
      const result = await fetchRemoteKnowledgeBases(search, page);
      setItems((previous) => (page === 1 ? result.items : [...previous, ...result.items]));
      setNextPage(result.nextPage);
    } catch (failure) {
      setError(localizeAPIError(failure, t) || errorMessage(failure));
    } finally {
      setBusy(false);
    }
  }
  async function select(id: string) {
    setBusy(true);
    setError("");
    try {
      const result = await fetchRemoteKnowledgeBaseMCPConfig(id);
      if (typeof result.config.url !== "string" || !result.config.url) throw new Error(t("appKnowledgeURLMissing"));
      onSelect(result.config.url);
      setSelected(id);
    } catch (failure) {
      setError(localizeAPIError(failure, t) || errorMessage(failure));
    } finally {
      setBusy(false);
    }
  }
  return { items, search, setSearch, busy, error, selected, nextPage, load, select };
}

export function AppKnowledgePicker({ t, onSelect }: { t: TranslateFn; onSelect: (url: string) => void }) {
  const picker = useAppKnowledgePicker(t, onSelect);
  return (
    <details className={styles.advanced}>
      <summary>{t("appChooseKnowledgeBase")}</summary>
      <div className={styles.fields}>
        <div className={styles.searchRow}>
          <TextInput
            aria-label={t("appKnowledgeSearch")}
            placeholder={t("appKnowledgeSearch")}
            value={picker.search}
            onChange={(event) => picker.setSearch(event.target.value)}
          />
          <Button loading={picker.busy} onClick={() => void picker.load()}>
            {t("search")}
          </Button>
        </div>
        {picker.items.length ? (
          <Field label={t("appKnowledgeBase")}>
            <Select
              value={picker.selected}
              disabled={picker.busy}
              triggerProps={{ "aria-label": t("appKnowledgeBase") }}
              options={[
                { value: "", label: t("appSelectKnowledgeBase") },
                ...picker.items.map((item) => ({
                  value: item.id,
                  label: item.name,
                  disabled: item.availability !== "available",
                })),
              ]}
              onValueChange={(id) => {
                if (id) void picker.select(id);
              }}
            />
          </Field>
        ) : null}
        {picker.nextPage ? (
          <Button size="sm" disabled={picker.busy} onClick={() => void picker.load(picker.nextPage)}>
            {t("appLoadMore")}
          </Button>
        ) : null}
        {picker.error ? (
          <p className="form-error" role="alert">
            {picker.error}
          </p>
        ) : null}
      </div>
    </details>
  );
}
